package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
	"github.com/spf13/cobra"
)

// lessonRecurCommand records that a lesson's gap manifested again — the
// strongest possible signal a lesson needs to graduate up the enforcement
// ladder. It does NOT change **Status:**; promoting the lesson stays a
// deliberate `change-status` call.
func lessonRecurCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recur <slug>",
		Short: "Record that a lesson's gap manifested again",
		Long: `For a canonical Lesson, appends exactly one immutable typed JSON
child under occurrences/ and leaves the README and index byte-identical;
recurrence metadata is derived from valid children. The compatibility path for
a legacy flat Lesson increments **Recurred:** and appends its old prose entry.
Neither path changes **Status:**. Run
"specscore lesson change-status <slug> --to=<status>" separately to act on the
signal. A missing lesson exits 3.

A recurrence against a lesson already retired (Withdrawn or Superseded) is
evidence the retirement itself was wrong — it still exits 0 and records the
occurrence (the evidence is worth keeping), but prints a warning to stderr
rather than succeeding silently.

Docs: docs/agent-lessons.md#create-and-record`,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runLessonRecur,
	}
	cmd.Flags().String("note", "", "free-form note describing this occurrence")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	return cmd
}

// warnIfLessonRetired prints a stderr warning when l's status is terminal
// (Withdrawn or Superseded — recognized statuses with no legal outgoing
// arc). An unrecognized or missing status is not this verb's concern (L-002
// already governs status validity), so it is left silent.
func warnIfLessonRetired(w io.Writer, slug string, status string) {
	canonical, ok := lifecycle.ParseStatus(lifecycle.KindLesson, strings.TrimSpace(status))
	if !ok || len(lifecycle.LegalTargets(lifecycle.KindLesson, canonical)) > 0 {
		return
	}
	_, _ = fmt.Fprintf(w,
		"warning: %s is %s — recording a recurrence against a retired lesson suggests the retirement should be revisited (consider recording a fresh lesson referencing this one) rather than continuing to log against it\n",
		slug, string(canonical))
}

func runLessonRecur(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return exitcode.InvalidArgsError("missing required positional argument: <slug>")
	}
	if len(args) > 1 {
		return exitcode.InvalidArgsErrorf(
			"too many positional arguments: recur accepts exactly one <slug>, got %d", len(args))
	}
	slug := args[0]
	if err := lesson.ValidateSlug(slug); err != nil {
		return exitcode.InvalidArgsErrorf("invalid slug %q: %v", slug, err)
	}

	note, _ := cmd.Flags().GetString("note")
	projectFlag, _ := cmd.Flags().GetString("project")

	root, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	path, err := lesson.ResolveLessonFile(filepath.Join(root, "spec", "lessons"), slug)
	if err != nil {
		return err
	}

	before, err := lessonParseFn(path)
	if err != nil {
		return exitcode.UnexpectedErrorf("parsing lesson %s: %v", slug, err)
	}
	warnIfLessonRetired(cmd.ErrOrStderr(), slug, before.Status)
	if before.Canonical {
		id := uuid.NewString()
		now := time.Now().UTC()
		prepared, err := prepareLessonEvent(root, "lesson.occurrence-recorded", slug, map[string]any{"occurrence_id": id}, now)
		if err != nil {
			return exitcode.UnexpectedErrorf("preparing occurrence event: %v", err)
		}
		o, err := lesson.AddOccurrence(lesson.AddOccurrenceOptions{LessonPath: path, ID: id, Summary: note, Context: captureOccurrenceContext(root, path), Evidence: lesson.Evidence{Kind: "none"}, Now: now})
		if err != nil {
			_ = prepared.Abort()
			return exitcode.UnexpectedErrorf("recording occurrence: %v", err)
		}
		items, err := lesson.DiscoverOccurrences(path)
		if err != nil {
			_ = os.Remove(o.Path)
			_ = prepared.Abort()
			return exitcode.UnexpectedErrorf("reading occurrences: %v", err)
		}
		result, commitErr := prepared.Commit(cmd.Context())
		if commitErr != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: occurrence recorded; durable event delivery is pending: %v\n", commitErr)
		}
		for _, failure := range result.Failed {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: subscriber %s remains pending: %v\n", failure.Name, failure.Err)
		}
		_ = items // occurrence identifier stays internal for historical output compatibility.
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: recurred %d\n", slug, len(items))
		return nil
	}

	if err := lesson.ValidateSafeContent("legacy recurrence note", note); err != nil {
		return exitcode.InvalidArgsErrorf("unsafe recurrence note: %v", err)
	}
	indexPath := filepath.Join(root, "spec", "lessons", "README.md")
	beforeBody, err := os.ReadFile(path)
	if err != nil {
		return exitcode.UnexpectedErrorf("snapshotting legacy Lesson: %v", err)
	}
	beforeIndex, indexErr := os.ReadFile(indexPath)
	if indexErr != nil && !os.IsNotExist(indexErr) {
		return exitcode.UnexpectedErrorf("snapshotting lessons index: %v", indexErr)
	}
	prepared, err := prepareLessonEvent(root, "lesson.occurrence-recorded", slug, map[string]any{"kind": "legacy-recurrence"}, time.Now().UTC())
	if err != nil {
		return exitcode.UnexpectedErrorf("preparing recurrence event: %v", err)
	}
	count, err := lessonRecurFn(path, note)
	if err != nil {
		_ = prepared.Abort()
		return exitcode.UnexpectedErrorf("recording recurrence: %v", err)
	}

	specSub := filepath.Join(root, "spec")
	updated, parseErr := lesson.Parse(path)
	if parseErr != nil {
		_ = os.WriteFile(path, beforeBody, 0o644)
		_ = prepared.Abort()
		return exitcode.UnexpectedErrorf("parsing updated legacy Lesson: %v", parseErr)
	}
	if err := lessonIndexUpsertFn(specSub, updated); err != nil {
		_ = os.WriteFile(path, beforeBody, 0o644)
		if indexErr == nil {
			_ = os.WriteFile(indexPath, beforeIndex, 0o644)
		}
		_ = prepared.Abort()
		return exitcode.UnexpectedErrorf("upserting legacy Lesson index row: %v", err)
	}
	result, commitErr := prepared.Commit(cmd.Context())
	if commitErr != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: recurrence recorded; durable event delivery is pending: %v\n", commitErr)
	}
	for _, failure := range result.Failed {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: subscriber %s remains pending: %v\n", failure.Name, failure.Err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: recurred %d\n", slug, count)
	return nil
}
