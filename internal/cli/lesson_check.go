package cli

import (
	"fmt"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/spf13/cobra"
)

// lesson check is the CI-shaped form of lesson list: it uses exactly the same
// filters/output but fails only when the reviewed allowance is exceeded.
func lessonCheckCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "check", Short: "Fail CI when matching Lesson gaps exceed an allowed baseline", Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true, RunE: runLessonCheck}
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("status", "", "comma-separated lifecycle status filter")
	cmd.Flags().Bool("not-enforced", false, "restrict to Recorded and Stated Lessons")
	cmd.Flags().Int("min-recurred", 0, "require at least this many derived occurrences")
	cmd.Flags().Int("max", 0, "maximum matching Lessons allowed before exit 1")
	cmd.Flags().String("format", "text", "output format: text, yaml, json")
	return cmd
}

func runLessonCheck(cmd *cobra.Command, _ []string) error {
	max, _ := cmd.Flags().GetInt("max")
	if max < 0 {
		return exitcode.InvalidArgsErrorf("--max must be >= 0, got %d", max)
	}
	// Reuse the list command, including its validation and deterministic output,
	// rather than duplicating filter interpretation in a CI-only code path.
	list := lessonListCommand()
	list.SetOut(cmd.OutOrStdout())
	list.SetErr(cmd.ErrOrStderr())
	for _, name := range []string{"project", "status", "not-enforced", "min-recurred", "format"} {
		if f := cmd.Flags().Lookup(name); f != nil && f.Changed {
			_ = list.Flags().Set(name, f.Value.String())
		}
	}
	project, _ := cmd.Flags().GetString("project")
	status, _ := cmd.Flags().GetString("status")
	notEnforced, _ := cmd.Flags().GetBool("not-enforced")
	min, _ := cmd.Flags().GetInt("min-recurred")
	format, _ := cmd.Flags().GetString("format")
	// Count separately from rendered output. The predicate is intentionally the
	// same as list's public parameters and derives directory counts.
	dir, err := resolveLessonsDir(project)
	if err != nil {
		return err
	}
	lessons, err := lessonDiscoverForCheck(dir)
	if err != nil {
		return err
	}
	count, err := countCheckedLessons(lessons, status, notEnforced, min)
	if err != nil {
		return err
	}
	list.SetArgs(listArgsForCheck(project, status, notEnforced, min, format))
	if err := list.Execute(); err != nil {
		return err
	}
	if count > max {
		return exitcode.ConflictErrorf("lesson check found %d matching Lessons; maximum is %d", count, max)
	}
	return nil
}

// Kept as helpers to make the checked predicate independently testable.
func lessonDiscoverForCheck(dir string) ([]*lesson.Lesson, error) { return lesson.Discover(dir) }
func countCheckedLessons(items []*lesson.Lesson, status string, notEnforced bool, min int) (int, error) {
	if notEnforced && status != "" {
		return 0, exitcode.InvalidArgsError("--status and --not-enforced are mutually exclusive")
	}
	var statuses map[string]bool
	var err error
	if notEnforced {
		statuses = lessonStatusSet(notEnforcedStatuses)
	} else if status != "" {
		statuses, err = parseLessonStatusFilter(status)
		if err != nil {
			return 0, err
		}
	}
	count := 0
	for _, l := range items {
		if statuses != nil && !statuses[strings.ToLower(l.Status)] {
			continue
		}
		n := l.Recurred
		if l.Canonical {
			occurrences, err := lesson.DiscoverOccurrences(l.Path)
			if err != nil {
				return 0, exitcode.UnexpectedErrorf("reading occurrences: %v", err)
			}
			n = len(occurrences)
		}
		if n >= min {
			count++
		}
	}
	return count, nil
}
func listArgsForCheck(project, status string, notEnforced bool, min int, format string) []string {
	args := []string{}
	if project != "" {
		args = append(args, "--project", project)
	}
	if status != "" {
		args = append(args, "--status", status)
	}
	if notEnforced {
		args = append(args, "--not-enforced")
	}
	if min != 0 {
		args = append(args, "--min-recurred", fmt.Sprint(min))
	}
	args = append(args, "--format", format)
	return args
}
