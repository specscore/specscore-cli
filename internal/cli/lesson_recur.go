package cli

import (
	"fmt"
	"path/filepath"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
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
separately to act on the signal. A missing lesson exits 3.`,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runLessonRecur,
	}
	cmd.Flags().String("note", "", "free-form note describing this occurrence")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	return cmd
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
