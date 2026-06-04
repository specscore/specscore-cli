package cli

import (
	"fmt"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/plan"
	"github.com/spf13/cobra"
)

// planListCommand returns the "plan list" subcommand.
func planListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all plan slugs, one per line",
		Long: `Lists single-file plans in a project as slugs, one per line, sorted
alphabetically. Use --status to filter by status (case-insensitive exact
match). Output is empty (exit 0) when no plans match.`,
		Args: cobra.NoArgs,
		RunE: runPlanList,
	}
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("status", "", "filter by status (case-insensitive exact match)")
	cmd.Flags().String("format", "text", "output format: text, yaml, json")
	cmd.Flags().String("fields", "", "comma-separated metadata fields: status, source-feature, mode, date, owner")
	return cmd
}

// validPlanFields lists the recognized --fields names, in canonical order.
var validPlanFields = []string{"status", "source-feature", "mode", "date", "owner"}

// parsePlanFields validates a comma-separated --fields value against the
// recognized plan field names, deduping while preserving order. An
// unrecognized name yields an exit-2 error naming the offending field.
func parsePlanFields(s string) ([]string, error) {
	if s == "" {
		return nil, nil
	}
	valid := make(map[string]bool, len(validPlanFields))
	for _, f := range validPlanFields {
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
				"unknown field %q (valid: %s)", f, strings.Join(validPlanFields, ", "))
		}
		if !seen[f] {
			seen[f] = true
			fields = append(fields, f)
		}
	}
	return fields, nil
}

// planListEntry is the structured representation emitted in yaml/json format.
type planListEntry struct {
	Slug   string `json:"slug" yaml:"slug"`
	Status string `json:"status" yaml:"status"`
}

func runPlanList(cmd *cobra.Command, _ []string) error {
	projectFlag, _ := cmd.Flags().GetString("project")
	statusFilter, _ := cmd.Flags().GetString("status")
	format, _ := cmd.Flags().GetString("format")
	fieldsFlag, _ := cmd.Flags().GetString("fields")

	// Validate --fields BEFORE discovery so a bad name fails fast (exit 2).
	fields, err := parsePlanFields(fieldsFlag)
	if err != nil {
		return err
	}

	if err := validateFormat(format); err != nil {
		return err
	}

	// When --fields is set, output must be structured; auto-upgrade a text
	// (or default) format to yaml rather than printing mixed text.
	effFormat := format
	if len(fields) > 0 && (format == "" || format == "text") {
		effFormat = "yaml"
	}

	plansDir, err := resolvePlansDir(projectFlag)
	if err != nil {
		return err
	}

	plans, err := plan.Discover(plansDir)
	if err != nil {
		return exitcode.UnexpectedErrorf("discovering plans: %v", err)
	}

	// Filter by status: case-insensitive EXACT match on the trimmed status.
	statusFilterLower := strings.ToLower(strings.TrimSpace(statusFilter))
	matched := func(p *plan.Plan) bool {
		status := strings.TrimSpace(p.Status)
		return statusFilter == "" || strings.ToLower(status) == statusFilterLower
	}

	w := cmd.OutOrStdout()

	// --fields mode: emit one map per plan with slug plus every requested
	// field, keys always present (no omitempty), encoded as yaml/json.
	if len(fields) > 0 {
		var entries []map[string]string
		for _, p := range plans {
			if !matched(p) {
				continue
			}
			entry := map[string]string{"slug": p.Slug}
			for _, f := range fields {
				entry[f] = planFieldValue(p, f)
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

	var entries []planListEntry
	for _, p := range plans {
		if !matched(p) {
			continue
		}
		entries = append(entries, planListEntry{Slug: p.Slug, Status: strings.TrimSpace(p.Status)})
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
			_, _ = fmt.Fprintln(w, e.Slug)
		}
	}
	return nil
}

// planFieldValue maps a requested --fields name to the plan's value.
func planFieldValue(p *plan.Plan, field string) string {
	switch field {
	case "status":
		return strings.TrimSpace(p.Status)
	case "source-feature":
		return p.SourceFeature
	case "mode":
		return string(p.Mode)
	case "date":
		return p.Date
	case "owner":
		return p.Owner
	}
	return ""
}
