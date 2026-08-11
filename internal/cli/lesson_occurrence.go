package cli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/specscore/specscore-cli/pkg/event"
	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/spf13/cobra"
)

func lessonOccurrenceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "occurrence", Short: "Record and query immutable Lesson occurrences"}
	cmd.AddCommand(lessonOccurrenceAddCommand(), lessonOccurrenceListCommand(), lessonOccurrenceInfoCommand())
	return cmd
}

func lessonOccurrenceAddCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "add <lesson>", Short: "Append one typed occurrence without rewriting the Lesson", Long: "Context is strict, bounded, recursively scanned, and secret/contact-free; original/user/agent prompts, raw logs/diffs, credentials, email addresses, absolute paths, traversal, unknown fields, and non-UTC timestamps are refused. Evidence paths and worktree path hints are normalized repository-relative forward-slash paths or redacted. Docs: docs/agent-lessons.md#create-and-record", Args: cobra.ExactArgs(1), SilenceUsage: true, SilenceErrors: true, RunE: runLessonOccurrenceAdd}
	cmd.Flags().String("summary", "", "bounded factual summary (defaults to a neutral observation)")
	cmd.Flags().String("context-json", "", "JSON object with safe generic context")
	cmd.Flags().String("context-file", "", "path to a JSON context object")
	cmd.Flags().Bool("context-stdin", false, "read a JSON context object from stdin")
	cmd.Flags().Bool("capture-context", true, "capture safe local git/worktree facts when no explicit context is given")
	cmd.Flags().String("evidence-kind", "none", "evidence kind: url, path, command, none")
	cmd.Flags().String("evidence-ref", "", "bounded stable evidence reference (required unless evidence-kind=none)")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	return cmd
}

func runLessonOccurrenceAdd(cmd *cobra.Command, args []string) error {
	return runLessonOccurrenceAddWithDeps(cmd, args, defaultLessonCLIDeps())
}

func runLessonOccurrenceAddWithDeps(cmd *cobra.Command, args []string, deps lessonCLIDeps) error {
	slug := args[0]
	if err := lesson.ValidateSlug(slug); err != nil {
		return exitcode.InvalidArgsErrorf("invalid slug %q: %v", slug, err)
	}
	project, _ := cmd.Flags().GetString("project")
	root, err := resolveSpecRoot(project)
	if err != nil {
		return err
	}
	lessonsDir := filepath.Join(root, "spec", "lessons")
	path, err := lesson.ResolveLessonFile(lessonsDir, slug)
	if err != nil {
		return err
	}
	l, err := deps.parse(path)
	if err != nil {
		return exitcode.UnexpectedErrorf("parsing lesson %s: %v", slug, err)
	}
	if !l.Canonical {
		return exitcode.InvalidStateErrorf("lesson %q is legacy flat form; `lesson recur` remains available until explicit migration", slug)
	}
	context, explicit, err := occurrenceContextInput(cmd)
	if err != nil {
		return err
	}
	capture, _ := cmd.Flags().GetBool("capture-context")
	if !explicit && capture {
		context = captureOccurrenceContext(root, path)
	}
	summary, _ := cmd.Flags().GetString("summary")
	kind, _ := cmd.Flags().GetString("evidence-kind")
	ref, _ := cmd.Flags().GetString("evidence-ref")
	evidence := lesson.Evidence{Kind: kind}
	if strings.TrimSpace(ref) != "" {
		value := strings.TrimSpace(ref)
		evidence.Ref = &value
	}
	id := uuid.NewString()
	now := time.Now().UTC()
	result, failure := publishCanonicalOccurrence(cmd.Context(), root, l, lesson.AddOccurrenceOptions{LessonPath: path, ID: id, Summary: summary, Context: context, Evidence: evidence, Now: now}, false, deps)
	if failure != nil {
		return failure.cliError(false)
	}
	for _, failure := range result.delivery.Failed {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: subscriber %s remains pending: %v\n", failure.Name, failure.Err)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: occurrence %s\n", slug, result.occurrence.ID)
	return nil
}

type canonicalOccurrenceResult struct {
	occurrence lesson.Occurrence
	items      []lesson.Occurrence
	delivery   event.ReplayResult
}

type canonicalOccurrenceFailure struct {
	phase            string
	recoveryRequired bool
	err              error
	eventID          string
}

func (f *canonicalOccurrenceFailure) cliError(recur bool) error {
	if f.recoveryRequired {
		return exitcode.UnexpectedErrorf("%v", f.err)
	}
	switch f.phase {
	case "snapshot":
		return exitcode.UnexpectedErrorf("snapshotting lessons index: %v", f.err)
	case "prepare":
		return exitcode.UnexpectedErrorf("preparing occurrence event: %v", f.err)
	case "record":
		if !recur {
			return exitcode.InvalidArgsErrorf("invalid occurrence: %v", f.err)
		}
		return exitcode.UnexpectedErrorf("recording occurrence: %v", f.err)
	case "index":
		return exitcode.UnexpectedErrorf("upserting occurrence index row: %v", f.err)
	case "read":
		return exitcode.UnexpectedErrorf("reading occurrences: %v", f.err)
	default:
		return exitcode.UnexpectedErrorf("occurrence recorded but event publication is pending for event %s: %v", f.eventID, f.err)
	}
}

// publishCanonicalOccurrence owns the one canonical occurrence transaction:
// snapshot index, prepare event, publish child, refresh/read the derived row,
// compensate child+index together, then commit the event.
func publishCanonicalOccurrence(ctx context.Context, root string, l *lesson.Lesson, opts lesson.AddOccurrenceOptions, readBack bool, deps lessonCLIDeps) (canonicalOccurrenceResult, *canonicalOccurrenceFailure) {
	if err := preflightOccurrenceIndex(root); err != nil {
		return canonicalOccurrenceResult{}, &canonicalOccurrenceFailure{phase: "snapshot", err: err}
	}
	prepared, err := deps.prepareEvent(root, "lesson.occurrence-recorded", l.Slug, map[string]any{"occurrence_id": opts.ID}, opts.Now)
	if err != nil {
		return canonicalOccurrenceResult{}, &canonicalOccurrenceFailure{phase: "prepare", err: err}
	}
	o, err := deps.addOccurrence(opts)
	if err != nil {
		recovery, resolved := prepared.ResolveMutationFailure("recording occurrence", err)
		return canonicalOccurrenceResult{}, &canonicalOccurrenceFailure{phase: "record", recoveryRequired: recovery, err: resolved}
	}
	compensate := func(operation string, cause error) (bool, error) {
		failure := lesson.CompensatePublication(func() error {
			if err := deps.removeOccurrence(o.Path); err != nil {
				return err
			}
			// Recompute only this Lesson's derived row from the surviving
			// immutable children. Restoring a whole-file snapshot could erase a
			// concurrently committed row owned by another command.
			indexPath := filepath.Join(root, "spec", "lessons", "README.md")
			if err := deps.reconcileIndex(filepath.Join(root, "spec"), l); err != nil {
				return err
			}
			return durableFencePathWithOps(indexPath, deps.durable)
		}, cause)
		return prepared.ResolveMutationFailure(operation, failure)
	}
	if err := deps.indexUpsert(filepath.Join(root, "spec"), l); err != nil {
		recovery, resolved := compensate("upserting occurrence index row", err)
		return canonicalOccurrenceResult{}, &canonicalOccurrenceFailure{phase: "index", recoveryRequired: recovery, err: resolved}
	}
	var items []lesson.Occurrence
	if readBack {
		items, err = deps.discoverOccurrences(opts.LessonPath)
		if err != nil {
			recovery, resolved := compensate("reading occurrences after publication", err)
			return canonicalOccurrenceResult{}, &canonicalOccurrenceFailure{phase: "read", recoveryRequired: recovery, err: resolved}
		}
	}
	delivery, err := prepared.Commit(ctx)
	if err != nil {
		return canonicalOccurrenceResult{}, &canonicalOccurrenceFailure{phase: "commit", eventID: prepared.event.UUID, err: err}
	}
	return canonicalOccurrenceResult{occurrence: o, items: items, delivery: delivery}, nil
}

func preflightOccurrenceIndex(root string) error {
	path := filepath.Join(root, "spec", "lessons", "README.md")
	if _, err := os.Stat(path); err != nil {
		return err
	}
	_, err := os.ReadFile(path)
	return err
}

func occurrenceContextInput(cmd *cobra.Command) (map[string]any, bool, error) {
	raw, _ := cmd.Flags().GetString("context-json")
	file, _ := cmd.Flags().GetString("context-file")
	stdin, _ := cmd.Flags().GetBool("context-stdin")
	n := 0
	if raw != "" {
		n++
	}
	if file != "" {
		n++
	}
	if stdin {
		n++
	}
	if n > 1 {
		return nil, false, exitcode.InvalidArgsError("--context-json, --context-file, and --context-stdin are mutually exclusive")
	}
	if n == 0 {
		return map[string]any{}, false, nil
	}
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, false, exitcode.InvalidArgsErrorf("reading --context-file: %v", err)
		}
		raw = string(b)
	}
	if stdin {
		b, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return nil, false, exitcode.InvalidArgsErrorf("reading --context-stdin: %v", err)
		}
		raw = string(b)
	}
	var context map[string]any
	if err := json.Unmarshal([]byte(raw), &context); err != nil || context == nil {
		return nil, false, exitcode.InvalidArgsError("context must be a JSON object")
	}
	return context, true, nil
}

// captureOccurrenceContext only reads local git state and has no connection to
// a server, browser, credentials, or runtime-specific session.
func captureOccurrenceContext(root, lessonPath string) map[string]any {
	ctx := map[string]any{"execution": map[string]any{"kind": "interactive"}}
	if actor := strings.TrimSpace(os.Getenv("SPECSCORE_ACTOR")); actor != "" {
		ctx["execution"] = map[string]any{"kind": "interactive", "id": actor}
	}
	if root == "" {
		return ctx
	}
	if root != "" {
		s := sha256.Sum256([]byte(root))
		// The worktree is the repository root, not the Lesson directory. The
		// opaque digest lets a server correlate a local claim without exposing
		// the machine path in a committed occurrence.
		ctx["worktree"] = map[string]any{"path_hint": "redacted", "id": fmt.Sprintf("local-%x", s[:6])}
	}
	git := map[string]any{}
	if out, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output(); err == nil {
		if commit := strings.TrimSpace(string(out)); commit != "" {
			git["commit"] = commit
		}
	}
	if out, err := exec.Command("git", "-C", root, "branch", "--show-current").Output(); err == nil {
		if branch := strings.TrimSpace(string(out)); branch != "" {
			git["branch"] = branch
		}
	}
	if len(git) > 0 {
		ctx["git"] = git
	}
	return ctx
}

func lessonOccurrenceListCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "list <lesson>", Short: "List occurrences in deterministic chronological order", Long: "List immutable safe occurrence facts. Docs: docs/agent-lessons.md#create-and-record", Args: cobra.ExactArgs(1), SilenceUsage: true, SilenceErrors: true, RunE: runLessonOccurrenceList}
	cmd.Flags().String("format", "text", "output format: text, yaml, json")
	cmd.Flags().String("project", "", "project root")
	return cmd
}

func lessonOccurrenceInfoCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "info <lesson> <occurrence-id>", Short: "Show one immutable occurrence", Long: "Reads and validates filename=id, strict UTC ordering data, recursive content policy, and the published occurrence contract. Docs: docs/agent-lessons.md#create-and-record", Args: cobra.ExactArgs(2), SilenceUsage: true, SilenceErrors: true, RunE: runLessonOccurrenceInfo}
	cmd.Flags().String("format", "yaml", "output format: text, yaml, json")
	cmd.Flags().String("project", "", "project root")
	return cmd
}

func occurrencePath(cmd *cobra.Command, slug string) (string, error) {
	project, _ := cmd.Flags().GetString("project")
	dir, err := resolveLessonsDir(project)
	if err != nil {
		return "", err
	}
	return lesson.ResolveLessonFile(dir, slug)
}

func runLessonOccurrenceList(cmd *cobra.Command, args []string) error {
	format, _ := cmd.Flags().GetString("format")
	if err := validateFormat(format); err != nil {
		return err
	}
	path, err := occurrencePath(cmd, args[0])
	if err != nil {
		return err
	}
	items, err := lesson.DiscoverOccurrences(path)
	if err != nil {
		return exitcode.UnexpectedErrorf("reading occurrences: %v", err)
	}
	if format == "json" {
		return newJSONEnc(cmd.OutOrStdout()).Encode(items)
	}
	if format == "yaml" {
		enc := newYAMLEnc(cmd.OutOrStdout())
		if err := enc.Encode(items); err != nil {
			return err
		}
		return enc.Close()
	}
	for _, o := range items {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", o.OccurredAt.Format("2006-01-02T15:04:05Z"), o.ID, o.Summary)
	}
	return nil
}

func runLessonOccurrenceInfo(cmd *cobra.Command, args []string) error {
	format, _ := cmd.Flags().GetString("format")
	if err := validateFormat(format); err != nil {
		return err
	}
	path, err := occurrencePath(cmd, args[0])
	if err != nil {
		return err
	}
	o, err := lesson.FindOccurrence(path, args[1])
	if os.IsNotExist(err) {
		return exitcode.NotFoundErrorf("occurrence not found: %s", args[1])
	}
	if err != nil {
		return exitcode.UnexpectedErrorf("reading occurrence: %v", err)
	}
	if format == "json" {
		return newJSONEnc(cmd.OutOrStdout()).Encode(o)
	}
	if format == "yaml" {
		enc := newYAMLEnc(cmd.OutOrStdout())
		if err := enc.Encode(o); err != nil {
			return err
		}
		return enc.Close()
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "ID: %s\nOccurred At: %s\nSummary: %s\n", o.ID, o.OccurredAt.Format("2006-01-02T15:04:05Z"), o.Summary)
	return nil
}
