package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
	"github.com/spf13/cobra"
)

// lessonListCommand returns the "lesson list" subcommand.
func lessonListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all lesson slugs, one per line",
		Long: `Lists lessons in a project as slugs, one per line, sorted
alphabetically.

--not-enforced is the headline query — "what have we learned but not yet
enforced?" — matching every lesson in Recorded or Stated (Tier 0/1 of the
ladder; only Enforced binds). It is shorthand for --status=recorded,stated.

--status filters by one or more statuses, comma-separated and
case-insensitive (recorded, stated, enforced, withdrawn, superseded); an
unrecognized value exits 2 naming it rather than silently matching nothing.
--status and --not-enforced are mutually exclusive.

--min-recurred N additionally restricts to lessons whose recurrence count is
at least N. Canonical counts derive from validated child occurrences; legacy
flat counts use **Recurred:**. Thus "which lessons have recurred and are still
not enforced?" is one command: --not-enforced --min-recurred=1.

Output is empty (exit 0) when no lessons match.`,
		Args: cobra.NoArgs,
		RunE: runLessonList,
	}
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("status", "", "filter by one or more statuses, comma-separated, case-insensitive: recorded, stated, enforced, withdrawn, superseded")
	cmd.Flags().Bool("not-enforced", false, `the headline query: shorthand for --status=recorded,stated ("what have we learned but not yet enforced?")`)
	cmd.Flags().Int("min-recurred", 0, "restrict to lessons whose Recurred count is at least N (0 = no filter)")
	cmd.Flags().String("format", "text", "output format: text, yaml, json")
	cmd.Flags().String("fields", "", "comma-separated metadata fields: status, recurred, date, owner")
	return cmd
}

// notEnforcedStatuses is the status set --not-enforced expands to: every rung
// of the ladder below Enforced (Tier 2, the only tier that binds).
var notEnforcedStatuses = []lifecycle.Status{lifecycle.LessonRecorded, lifecycle.LessonStated}

// parseLessonStatusFilter validates a comma-separated --status value against
// the canonical Lesson status set, case-insensitively, and returns the
// matching set as lowercased canonical names. Empty parts (from stray commas)
// are skipped silently, mirroring parseLessonFields. An unrecognized status
// name yields an exit-2 error naming it — the filter must never silently
// resolve to "match nothing" because the caller mistyped a value.
func parseLessonStatusFilter(raw string) (map[string]bool, error) {
	set := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		v := strings.TrimSpace(part)
		if v == "" {
			continue
		}
		canonical, ok := lifecycle.ParseStatus(lifecycle.KindLesson, v)
		if !ok {
			return nil, exitcode.InvalidArgsErrorf(
				"unrecognized --status value %q; legal values: %s",
				v, strings.Join(lessonStatusNames(), ", "))
		}
		set[strings.ToLower(string(canonical))] = true
	}
	return set, nil
}

// lessonStatusNames returns every canonical Lesson status name, for
// rendering in --status error messages.
func lessonStatusNames() []string {
	statuses := lifecycle.LegalStatuses(lifecycle.KindLesson)
	out := make([]string, len(statuses))
	for i, s := range statuses {
		out[i] = string(s)
	}
	return out
}

// lessonStatusSet renders a slice of lifecycle.Status values as a lowercased
// name set, matching parseLessonStatusFilter's output shape.
func lessonStatusSet(statuses []lifecycle.Status) map[string]bool {
	set := make(map[string]bool, len(statuses))
	for _, s := range statuses {
		set[strings.ToLower(string(s))] = true
	}
	return set
}

// validLessonFields lists the recognized --fields names, in canonical order.
var validLessonFields = []string{"status", "recurred", "date", "owner"}

// parseLessonFields validates a comma-separated --fields value against the
// recognized lesson field names, deduping while preserving order. An
// unrecognized name yields an exit-2 error naming the offending field.
func parseLessonFields(s string) ([]string, error) {
	if s == "" {
		return nil, nil
	}
	valid := make(map[string]bool, len(validLessonFields))
	for _, f := range validLessonFields {
		valid[f] = true
	}
	seen := make(map[string]bool)
	var fields []string
	for _, part := range strings.Split(s, ",") {
		f := strings.TrimSpace(part)
		if f == "" {
			continue
		}
		if !valid[f] {
			return nil, exitcode.InvalidArgsErrorf(
				"unknown field %q (valid: %s)", f, strings.Join(validLessonFields, ", "))
		}
		if !seen[f] {
			seen[f] = true
			fields = append(fields, f)
		}
	}
	return fields, nil
}

// lessonListEntry is the structured representation emitted in yaml/json
// format.
type lessonListEntry struct {
	Slug     string `json:"slug" yaml:"slug"`
	Status   string `json:"status" yaml:"status"`
	Recurred int    `json:"recurred" yaml:"recurred"`
}

func runLessonList(cmd *cobra.Command, _ []string) error {
	projectFlag, _ := cmd.Flags().GetString("project")
	statusFlag, _ := cmd.Flags().GetString("status")
	notEnforced, _ := cmd.Flags().GetBool("not-enforced")
	minRecurred, _ := cmd.Flags().GetInt("min-recurred")
	format, _ := cmd.Flags().GetString("format")
	fieldsFlag, _ := cmd.Flags().GetString("fields")

	fields, err := parseLessonFields(fieldsFlag)
	if err != nil {
		return err
	}

	if err := validateFormat(format); err != nil {
		return err
	}

	if notEnforced && cmd.Flags().Changed("status") {
		return exitcode.InvalidArgsError("--status and --not-enforced are mutually exclusive")
	}
	if minRecurred < 0 {
		return exitcode.InvalidArgsErrorf("--min-recurred must be >= 0, got %d", minRecurred)
	}

	// A nil statusSet means "no status filter" (list every status), matching
	// the pre-existing default. --not-enforced and a non-empty --status each
	// populate it; an explicitly empty/unset --status leaves it nil.
	var statusSet map[string]bool
	switch {
	case notEnforced:
		statusSet = lessonStatusSet(notEnforcedStatuses)
	case strings.TrimSpace(statusFlag) != "":
		statusSet, err = parseLessonStatusFilter(statusFlag)
		if err != nil {
			return err
		}
	}

	effFormat := format
	if len(fields) > 0 && (format == "" || format == "text") {
		effFormat = "yaml"
	}

	lessonsDir, err := resolveLessonsDir(projectFlag)
	if err != nil {
		return err
	}

	lessons, err := lesson.Discover(lessonsDir)
	if err != nil {
		return exitcode.UnexpectedErrorf("discovering lessons: %v", err)
	}

	recurrenceCount := func(l *lesson.Lesson) (int, error) {
		if !l.Canonical {
			return l.Recurred, nil
		}
		items, err := lesson.DiscoverOccurrences(l.Path)
		return len(items), err
	}
	matched := func(l *lesson.Lesson) (bool, int, error) {
		recurred, err := recurrenceCount(l)
		if err != nil {
			return false, 0, err
		}
		if statusSet != nil && !statusSet[strings.ToLower(strings.TrimSpace(l.Status))] {
			return false, recurred, nil
		}
		if minRecurred > 0 && recurred < minRecurred {
			return false, recurred, nil
		}
		return true, recurred, nil
	}

	w := cmd.OutOrStdout()

	if len(fields) > 0 {
		var entries []map[string]string
		for _, l := range lessons {
			ok, recurred, err := matched(l)
			if err != nil {
				return exitcode.UnexpectedErrorf("reading occurrences: %v", err)
			}
			if !ok {
				continue
			}
			entry := map[string]string{"slug": l.Slug}
			for _, f := range fields {
				if f == "recurred" {
					entry[f] = strconv.Itoa(recurred)
				} else {
					entry[f] = lessonFieldValue(l, f)
				}
			}
			entries = append(entries, entry)
		}
		if effFormat == "json" {
			return newJSONEnc(w).Encode(entries)
		}
		enc := newYAMLEnc(w)
		if err := enc.Encode(entries); err != nil {
			return exitcode.UnexpectedErrorf("encoding yaml: %v", err)
		}
		return enc.Close()
	}

	var entries []lessonListEntry
	for _, l := range lessons {
		ok, recurred, err := matched(l)
		if err != nil {
			return exitcode.UnexpectedErrorf("reading occurrences: %v", err)
		}
		if !ok {
			continue
		}
		entries = append(entries, lessonListEntry{Slug: l.Slug, Status: strings.TrimSpace(l.Status), Recurred: recurred})
	}

	switch format {
	case "yaml":
		enc := newYAMLEnc(w)
		if err := enc.Encode(entries); err != nil {
			return exitcode.UnexpectedErrorf("encoding yaml: %v", err)
		}
		return enc.Close()
	case "json":
		return newJSONEnc(w).Encode(entries)
	default:
		for _, e := range entries {
			if e.Recurred > 0 {
				_, _ = fmt.Fprintf(w, "%s (recurred %d)\n", e.Slug, e.Recurred)
			} else {
				_, _ = fmt.Fprintln(w, e.Slug)
			}
		}
	}
	return nil
}

// lessonFieldValue maps a requested --fields name to the lesson's value.
func lessonFieldValue(l *lesson.Lesson, field string) string {
	switch field {
	case "status":
		return strings.TrimSpace(l.Status)
	case "recurred":
		return strconv.Itoa(l.Recurred)
	case "date":
		return l.Date
	case "owner":
		return l.Owner
	}
	return ""
}
