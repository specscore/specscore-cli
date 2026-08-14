package cli

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/idea"
	"github.com/spf13/cobra"
)

// ideaListCommand returns the "idea list" subcommand.
func ideaListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all idea slugs, one per line",
		Long: `Lists all ideas in a project as slugs, one per line, sorted
alphabetically. By default, archived ideas are excluded. Use --all to
include them. Use --status to filter by lifecycle status. Use --parked
to show only ideas carrying the **Parked:** true axis (deliberately
deferred, independent of their **Status:**) and exclude unparked ideas.`,
		Args: cobra.NoArgs,
		RunE: runIdeaList,
	}
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("status", "", "filter by status (case-insensitive, e.g. Draft, Approved)")
	cmd.Flags().Bool("all", false, "include archived ideas in the output")
	cmd.Flags().Bool("include-archived", false, "include archived ideas in the output (alias for --all)")
	cmd.Flags().Bool("parked", false, "show only parked ideas (excludes unparked ideas)")
	cmd.Flags().String("format", "text", "output format: text, yaml, json")
	return cmd
}

// ideaListEntry is the structured representation emitted in yaml/json format.
type ideaListEntry struct {
	Slug     string `json:"slug" yaml:"slug"`
	Status   string `json:"status" yaml:"status"`
	Path     string `json:"path" yaml:"path"`
	Archived bool   `json:"archived" yaml:"archived"`
	Parked   bool   `json:"parked" yaml:"parked"`
}

func runIdeaList(cmd *cobra.Command, _ []string) error {
	projectFlag, _ := cmd.Flags().GetString("project")
	statusFilter, _ := cmd.Flags().GetString("status")
	showAll, _ := cmd.Flags().GetBool("all")
	includeArchived, _ := cmd.Flags().GetBool("include-archived")
	showAll = showAll || includeArchived
	parkedOnly, _ := cmd.Flags().GetBool("parked")
	format, _ := cmd.Flags().GetString("format")

	if format != "text" && format != "yaml" && format != "json" {
		return exitcode.InvalidArgsErrorf("invalid --format: %s (valid: text, yaml, json)", format)
	}

	specRoot, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	// Discover expects the spec/ directory (it joins "ideas" internally).
	specDir := filepath.Join(specRoot, "spec")
	discovered, err := idea.Discover(specDir)
	if err != nil {
		return exitcode.UnexpectedErrorf("discovering ideas: %v", err)
	}

	// Filter: exclude archived unless --all.
	var filtered []idea.Discovered
	for _, d := range discovered {
		if !showAll && d.Archived {
			continue
		}
		filtered = append(filtered, d)
	}

	// If --status or --parked is set, parse each idea and filter accordingly.
	// Also needed for yaml/json output regardless of filter.
	needsParse := statusFilter != "" || parkedOnly || format == "yaml" || format == "json"

	type enriched struct {
		idea.Discovered
		status string
		parked bool
	}
	var entries []enriched

	if needsParse {
		statusFilterLower := strings.ToLower(statusFilter)
		for _, d := range filtered {
			parsed, parseErr := idea.Parse(d.Path)
			status := "Draft"
			parked := false
			if parseErr == nil {
				if parsed.Status() != "" {
					status = parsed.Status()
				}
				parked = parsed.Parked()
			}
			if statusFilter != "" && strings.ToLower(status) != statusFilterLower {
				continue
			}
			if parkedOnly && !parked {
				continue
			}
			entries = append(entries, enriched{Discovered: d, status: status, parked: parked})
		}
	} else {
		for _, d := range filtered {
			entries = append(entries, enriched{Discovered: d})
		}
	}

	// Sort alphabetically by slug.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Slug < entries[j].Slug
	})

	w := cmd.OutOrStdout()

	switch format {
	case "yaml", "json":
		out := make([]ideaListEntry, len(entries))
		for i, e := range entries {
			out[i] = ideaListEntry{
				Slug:     e.Slug,
				Status:   e.status,
				Path:     e.Path,
				Archived: e.Archived,
				Parked:   e.parked,
			}
		}
		if format == "yaml" {
			enc := newYAMLEnc(w)
			if err := enc.Encode(out); err != nil {
				return exitcode.UnexpectedErrorf("encoding yaml: %v", err)
			}
			return enc.Close()
		}
		return newJSONEnc(w).Encode(out)
	default:
		for _, e := range entries {
			_, _ = fmt.Fprintln(w, e.Slug)
		}
	}
	return nil
}
