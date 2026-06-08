package cli

import (
	"bufio"
	"fmt"
	"io"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/plan"
	"github.com/spf13/cobra"
)

// planInfoDoc is the structured output of `plan info`. Metadata fields are
// always present (no omitempty) so an absent source field renders as an empty
// but present key.
type planInfoDoc struct {
	Slug          string      `yaml:"slug" json:"slug"`
	Status        string      `yaml:"status" json:"status"`
	SourceFeature string      `yaml:"source_feature" json:"source_feature"`
	Source        string      `yaml:"source" json:"source"` // `idea:<slug>` or `none`; empty for Feature-sourced plans
	Mode          string      `yaml:"mode" json:"mode"`
	Date          string      `yaml:"date" json:"date"`
	Owner         string      `yaml:"owner" json:"owner"`
	Tasks         plan.Rollup `yaml:"tasks" json:"tasks"`
}

func planInfoCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info <slug>",
		Short: "Show plan metadata and a task-status rollup",
		Long: `Returns a plan's metadata (slug, status, source feature, mode, date,
owner) together with a rollup of its tasks by status. Default output is YAML;
use --format for JSON or text. A missing plan exits 3 with no stdout output.`,
		// Argument validation is performed manually inside RunE so a missing
		// or extra positional yields a typed exit-2 error, and SilenceUsage/
		// SilenceErrors keep stdout empty on any non-zero exit.
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runPlanInfo,
	}
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("format", "yaml", "output format: yaml, json, text")
	return cmd
}

func runPlanInfo(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return exitcode.InvalidArgsError("missing required positional argument: <slug>")
	}
	if len(args) > 1 {
		return exitcode.InvalidArgsErrorf(
			"too many positional arguments: info accepts exactly one <slug>, got %d", len(args),
		)
	}
	slug := args[0]

	format, _ := cmd.Flags().GetString("format")
	if err := validateFormat(format); err != nil {
		return err
	}

	projectFlag, _ := cmd.Flags().GetString("project")
	plansDir, err := resolvePlansDir(projectFlag)
	if err != nil {
		return err
	}

	path, err := resolvePlanPath(plansDir, slug)
	if err != nil {
		return err
	}

	p, err := plan.Parse(path)
	if err != nil {
		return exitcode.UnexpectedErrorf("parsing plan %s: %v", slug, err)
	}

	doc := planInfoDoc{
		Slug:          p.Slug,
		Status:        p.Status,
		SourceFeature: p.SourceFeature,
		Source:        p.SourceRaw,
		Mode:          string(p.Mode),
		Date:          p.Date,
		Owner:         p.Owner,
		Tasks:         p.TaskRollup(),
	}

	w := cmd.OutOrStdout()
	switch format {
	case "json":
		return newJSONEnc(w).Encode(doc)
	case "text":
		return writePlanInfoText(w, doc)
	default:
		enc := newYAMLEnc(w)
		if err := enc.Encode(doc); err != nil {
			return exitcode.UnexpectedErrorf("encoding yaml: %v", err)
		}
		return enc.Close()
	}
}

func writePlanInfoText(w io.Writer, doc planInfoDoc) error {
	bw := bufio.NewWriter(w)
	_, _ = fmt.Fprintf(bw, "Slug:           %s\n", doc.Slug)
	_, _ = fmt.Fprintf(bw, "Status:         %s\n", doc.Status)
	_, _ = fmt.Fprintf(bw, "Source Feature: %s\n", doc.SourceFeature)
	if doc.Source != "" {
		_, _ = fmt.Fprintf(bw, "Source:         %s\n", doc.Source)
	}
	_, _ = fmt.Fprintf(bw, "Mode:           %s\n", doc.Mode)
	_, _ = fmt.Fprintf(bw, "Date:           %s\n", doc.Date)
	_, _ = fmt.Fprintf(bw, "Owner:          %s\n", doc.Owner)
	r := doc.Tasks
	_, _ = fmt.Fprintf(bw, "Tasks: %d total (%d done, %d in-progress, %d pending, %d blocked)\n",
		r.Total, r.Done, r.InProgress, r.Pending, r.Blocked)
	return bw.Flush()
}
