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
child under occurrences/, leaves the README byte-identical, and refreshes only its derived index row;
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
	return runLessonRecurWithDeps(cmd, args, defaultLessonCLIDeps())
}

func runLessonRecurWithDeps(cmd *cobra.Command, args []string, deps lessonCLIDeps) error {
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

	before, err := deps.parse(path)
	if err != nil {
		return exitcode.UnexpectedErrorf("parsing lesson %s: %v", slug, err)
	}
	if before.Canonical {
		id := uuid.NewString()
		now := time.Now().UTC()
		result, failure := publishCanonicalOccurrence(cmd.Context(), root, before, lesson.AddOccurrenceOptions{LessonPath: path, ID: id, Summary: note, Context: captureOccurrenceContext(root, path), Evidence: lesson.Evidence{Kind: "none"}, Now: now}, true, deps)
		if failure != nil {
			return failure.cliError(true)
		}
		warnIfLessonRetired(cmd.ErrOrStderr(), slug, result.status)
		for _, failure := range result.delivery.Failed {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: subscriber %s remains pending: %v\n", failure.Name, failure.Err)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: recurred %d\n", slug, len(result.items))
		return nil
	}

	if err := lesson.ValidateSafeContent("legacy recurrence note", note); err != nil {
		return exitcode.InvalidArgsErrorf("unsafe recurrence note: %v", err)
	}
	prepared, err := deps.prepareEvent(root, "lesson.occurrence-recorded", slug, map[string]any{"kind": "legacy-recurrence"}, time.Now().UTC())
	if err != nil {
		return exitcode.UnexpectedErrorf("preparing recurrence event: %v", err)
	}
	var updated *lesson.Lesson
	count, err := deps.recurWithPostMutation(path, note, func(int) error {
		var parseErr error
		updated, parseErr = deps.parse(path)
		if parseErr != nil {
			return parseErr
		}
		return reconcileLockedLessons(root, []string{slug}, nil, deps)
	})
	if err != nil {
		if recovery, resolved := prepared.ResolveMutationFailure("recording legacy recurrence", err); recovery {
			return exitcode.UnexpectedErrorf("%v", resolved)
		} else {
			return exitcode.UnexpectedErrorf("recording recurrence: %v", resolved)
		}
	}

	warnIfLessonRetired(cmd.ErrOrStderr(), slug, updated.Status)
	result, commitErr := prepared.Commit(cmd.Context())
	if commitErr != nil {
		return exitcode.UnexpectedErrorf("recurrence recorded but event publication is pending for event %s: %v", prepared.event.UUID, commitErr)
	}
	for _, failure := range result.Failed {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: subscriber %s remains pending: %v\n", failure.Name, failure.Err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: recurred %d\n", slug, count)
	return nil
}

type durableFile interface {
	Sync() error
	Close() error
}

type durableFileOps struct {
	open func(string) (durableFile, error)
}

func defaultDurableFileOps() durableFileOps {
	return durableFileOps{
		open: func(path string) (durableFile, error) { return os.Open(path) },
	}
}

func durableFencePathWithOps(path string, ops durableFileOps) error {
	for _, target := range []string{path, filepath.Dir(path)} {
		f, err := ops.open(target)
		if err != nil {
			return err
		}
		if err := f.Sync(); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}
