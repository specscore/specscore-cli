package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
	"github.com/specscore/specscore-cli/pkg/lint"
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
		Long: `Increments a lesson's **Recurred:** count and appends a dated entry
(with the optional --note) to its ## Recurrences section. It does NOT change
**Status:** — a recurrence is a signal that a lesson needs to graduate, not a
graduation itself. Run "specscore lesson change-status <slug> --to=<status>"
separately to act on the signal. A missing lesson exits 3.

A recurrence against a lesson already retired (Withdrawn or Superseded) is
evidence the retirement itself was wrong — it still exits 0 and records the
occurrence (the evidence is worth keeping), but prints a warning to stderr
rather than succeeding silently.`,
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
		o, err := lesson.AddOccurrence(lesson.AddOccurrenceOptions{LessonPath: path, Summary: note, Context: captureOccurrenceContext(root, path), Evidence: lesson.Evidence{Kind: "none"}})
		if err != nil {
			return exitcode.UnexpectedErrorf("recording occurrence: %v", err)
		}
		items, err := lesson.DiscoverOccurrences(path)
		if err != nil {
			return exitcode.UnexpectedErrorf("reading occurrences: %v", err)
		}
		_, _ = o, items // occurrence identifier stays internal for historical output compatibility.
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: recurred %d\n", slug, len(items))
		return nil
	}

	count, err := lessonRecurFn(path, note)
	if err != nil {
		return exitcode.UnexpectedErrorf("recording recurrence: %v", err)
	}

	// Keep the lessons index (Recurred column, when present) in sync, mirroring
	// every other lesson-mutating verb's post-write lint --fix pass.
	specSub := filepath.Join(root, "spec")
	if _, err := lintLintFn(lint.Options{SpecRoot: specSub, Fix: true}); err != nil {
		return exitcode.UnexpectedErrorf("running lint --fix: %v", err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: recurred %d\n", slug, count)
	return nil
}
