package cli

import (
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
	return runLessonOccurrenceAddWithDeps(cmd, args, defaultLessonOccurrenceDeps())
}

func runLessonOccurrenceAddWithDeps(cmd *cobra.Command, args []string, deps lessonOccurrenceDeps) error {
	slug := args[0]
	if err := lesson.ValidateSlug(slug); err != nil {
		return exitcode.InvalidArgsErrorf("invalid slug %q: %v", slug, err)
	}
	project, _ := cmd.Flags().GetString("project")
	lessonsDir, err := resolveLessonsDir(project)
	if err != nil {
		return err
	}
	path, err := lesson.ResolveLessonFile(lessonsDir, slug)
	if err != nil {
		return err
	}
	l, err := lesson.Parse(path)
	if err != nil {
		return exitcode.UnexpectedErrorf("parsing lesson %s: %v", slug, err)
	}
	if !l.Canonical {
		return exitcode.InvalidStateErrorf("lesson %q is legacy flat form; `lesson recur` remains available until explicit migration", slug)
	}
	root := projectRootForOccurrence(project)
	indexSnapshot, err := snapshotOccurrenceIndexWithDeps(root, deps)
	if err != nil {
		return exitcode.UnexpectedErrorf("snapshotting lessons index: %v", err)
	}
	context, explicit, err := occurrenceContextInput(cmd)
	if err != nil {
		return err
	}
	capture, _ := cmd.Flags().GetBool("capture-context")
	if !explicit && capture {
		context = captureOccurrenceContext(projectRootForOccurrence(project), path)
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
	prepared, err := deps.prepareEvent(root, "lesson.occurrence-recorded", slug, map[string]any{"occurrence_id": id}, now)
	if err != nil {
		return exitcode.UnexpectedErrorf("preparing occurrence event: %v", err)
	}
	o, err := deps.addOccurrence(lesson.AddOccurrenceOptions{LessonPath: path, ID: id, Summary: summary, Context: context, Evidence: evidence, Now: now})
	if err != nil {
		if recovery, resolved := prepared.ResolveMutationFailure("recording occurrence", err); recovery {
			return exitcode.UnexpectedErrorf("%v", resolved)
		} else {
			return exitcode.InvalidArgsErrorf("invalid occurrence: %v", resolved)
		}
	}
	if err := deps.indexUpsert(filepath.Join(root, "spec"), l); err != nil {
		failure := lesson.CompensatePublication(func() error {
			if removeErr := lesson.RemoveOccurrence(o.Path); removeErr != nil {
				return removeErr
			}
			return indexSnapshot.restore()
		}, err)
		if recovery, resolved := prepared.ResolveMutationFailure("upserting occurrence index row", failure); recovery {
			return exitcode.UnexpectedErrorf("%v", resolved)
		} else {
			return exitcode.UnexpectedErrorf("upserting occurrence index row: %v", resolved)
		}
	}
	result, commitErr := prepared.Commit(cmd.Context())
	if commitErr != nil {
		return exitcode.UnexpectedErrorf("occurrence recorded but event publication is pending for event %s: %v", prepared.event.UUID, commitErr)
	}
	for _, failure := range result.Failed {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: subscriber %s remains pending: %v\n", failure.Name, failure.Err)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: occurrence %s\n", slug, o.ID)
	return nil
}

// occurrenceIndexSnapshot lets occurrence publication compensate both the
// immutable child and the derived index row if the narrow index upsert fails.
// It is deliberately scoped to spec/lessons/README.md: occurrence add never
// runs a repository-wide fixer.
type occurrenceIndexSnapshot struct {
	path string
	data []byte
	mode os.FileMode
}

func snapshotOccurrenceIndex(root string) (occurrenceIndexSnapshot, error) {
	return snapshotOccurrenceIndexWithDeps(root, defaultLessonOccurrenceDeps())
}

func snapshotOccurrenceIndexWithDeps(root string, deps lessonOccurrenceDeps) (occurrenceIndexSnapshot, error) {
	path := filepath.Join(root, "spec", "lessons", "README.md")
	b, err := deps.readFile(path)
	if err != nil {
		return occurrenceIndexSnapshot{}, err
	}
	info, err := deps.stat(path)
	if err != nil {
		return occurrenceIndexSnapshot{}, err
	}
	return occurrenceIndexSnapshot{path: path, data: b, mode: info.Mode().Perm()}, nil
}

func (s occurrenceIndexSnapshot) restore() error {
	return durableRestoreFile(s.path, s.data, s.mode)
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

func projectRootForOccurrence(project string) string {
	root, err := resolveSpecRoot(project)
	if err != nil {
		return ""
	}
	return root
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
