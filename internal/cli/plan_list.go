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
	return cmd
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

	if err := validateFormat(format); err != nil {
		return err
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
	var entries []planListEntry
	for _, p := range plans {
		status := strings.TrimSpace(p.Status)
		if statusFilter != "" && strings.ToLower(status) != statusFilterLower {
			continue
		}
		entries = append(entries, planListEntry{Slug: p.Slug, Status: status})
	}

	w := cmd.OutOrStdout()
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
