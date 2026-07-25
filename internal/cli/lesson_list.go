package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/spf13/cobra"
)

// lessonListCommand returns the "lesson list" subcommand.
func lessonListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all lesson slugs, one per line",
		Long: `Lists lessons in a project as slugs, one per line, sorted
alphabetically. Use --status to filter by status (case-insensitive exact
match) — --status=recorded answers "what have we learned but not yet
enforced?" in one command. Output is empty (exit 0) when no lessons match.`,
		Args: cobra.NoArgs,
		RunE: runLessonList,
	}
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("status", "", "filter by status (case-insensitive exact match): recorded, stated, enforced, withdrawn, superseded")
	cmd.Flags().String("format", "text", "output format: text, yaml, json")
	cmd.Flags().String("fields", "", "comma-separated metadata fields: status, recurred, date, owner")
	return cmd
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
	statusFilter, _ := cmd.Flags().GetString("status")
	format, _ := cmd.Flags().GetString("format")
	fieldsFlag, _ := cmd.Flags().GetString("fields")

	fields, err := parseLessonFields(fieldsFlag)
	if err != nil {
		return err
	}

	if err := validateFormat(format); err != nil {
		return err
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

	statusFilterLower := strings.ToLower(strings.TrimSpace(statusFilter))
	matched := func(l *lesson.Lesson) bool {
		status := strings.TrimSpace(l.Status)
		return statusFilter == "" || strings.ToLower(status) == statusFilterLower
	}

	w := cmd.OutOrStdout()

	if len(fields) > 0 {
		var entries []map[string]string
		for _, l := range lessons {
			if !matched(l) {
				continue
			}
			entry := map[string]string{"slug": l.Slug}
			for _, f := range fields {
				entry[f] = lessonFieldValue(l, f)
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
		if !matched(l) {
			continue
		}
		entries = append(entries, lessonListEntry{Slug: l.Slug, Status: strings.TrimSpace(l.Status), Recurred: l.Recurred})
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
