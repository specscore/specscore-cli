package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/plan"
	"github.com/spf13/cobra"
)

// planReadinessDoc is the stable, agent-facing execution-readiness response.
// A false ready value is normal query data, not a command error: callers need
// to inspect every unmet prerequisite before deciding what to do next.
type planReadinessDoc struct {
	Slug               string                   `yaml:"slug" json:"slug"`
	Ready              bool                     `yaml:"ready" json:"ready"`
	UnmetPrerequisites []plan.UnmetPrerequisite `yaml:"unmet_prerequisites" json:"unmet_prerequisites"`
}

func planReadinessCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "readiness <slug>",
		Short: "Report whether a Plan's prerequisites permit execution",
		Long: `Reports whether a Plan is ready to begin execution with respect to its
declared **Prerequisite Plans:**. A prerequisite is ready only when its own
task rollup derives Implemented; a hand-authored Plan Status alone does not
satisfy this check. The command exits 0 for both ready and not-ready Plans so
agents can consume the stable ready boolean and every unmet prerequisite.

Use --format yaml (default), json, or text.`,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runPlanReadiness,
	}
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("format", "yaml", "output format: yaml, json, text")
	return cmd
}

func runPlanReadiness(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return exitcode.InvalidArgsError("missing required positional argument: <slug>")
	}
	if len(args) > 1 {
		return exitcode.InvalidArgsErrorf(
			"too many positional arguments: readiness accepts exactly one <slug>, got %d", len(args))
	}
	if err := plan.ValidateSlug(args[0]); err != nil {
		return exitcode.InvalidArgsErrorf("invalid slug %q: %v", args[0], err)
	}
	format, _ := cmd.Flags().GetString("format")
	if err := validateFormat(format); err != nil {
		return err
	}
	projectFlag, _ := cmd.Flags().GetString("project")
	specRoot, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}
	readiness, err := plan.PlanReadiness(specRoot, args[0])
	if err != nil {
		return readinessCLIError(err)
	}
	doc := planReadinessDoc{
		Slug:               args[0],
		Ready:              readiness.Ready,
		UnmetPrerequisites: readiness.Unmet,
	}
	switch format {
	case "json":
		if err := newJSONEnc(cmd.OutOrStdout()).Encode(doc); err != nil {
			return exitcode.UnexpectedErrorf("encoding json: %v", err)
		}
		return nil
	case "text":
		if err := writePlanReadinessText(cmd.OutOrStdout(), doc); err != nil {
			return exitcode.UnexpectedErrorf("writing text: %v", err)
		}
		return nil
	default:
		enc := newYAMLEnc(cmd.OutOrStdout())
		if err := enc.Encode(doc); err != nil {
			return exitcode.UnexpectedErrorf("encoding yaml: %v", err)
		}
		if err := enc.Close(); err != nil {
			return exitcode.UnexpectedErrorf("closing yaml output: %v", err)
		}
		return nil
	}
}

// readinessCLIError keeps library errors that already carry a CLI exit code
// intact, and turns every other parse/I/O failure into the documented
// Unexpected (10) result rather than Cobra's generic exit 1.
func readinessCLIError(err error) error {
	var coded interface{ ExitCode() int }
	if errors.As(err, &coded) {
		return err
	}
	return exitcode.UnexpectedErrorf("checking plan readiness: %v", err)
}

func writePlanReadinessText(w io.Writer, doc planReadinessDoc) error {
	bw := bufio.NewWriter(w)
	_, _ = fmt.Fprintf(bw, "Slug:  %s\n", doc.Slug)
	_, _ = fmt.Fprintf(bw, "Ready: %t\n", doc.Ready)
	if len(doc.UnmetPrerequisites) == 0 {
		_, _ = fmt.Fprintln(bw, "Unmet prerequisites: none")
		return bw.Flush()
	}
	_, _ = fmt.Fprintln(bw, "Unmet prerequisites:")
	for _, unmet := range doc.UnmetPrerequisites {
		if unmet.Reason != "" {
			_, _ = fmt.Fprintf(bw, "  - %s (status %s; %s)\n", unmet.Slug, unmet.Status, unmet.Reason)
			continue
		}
		derived := unmet.DerivedStatus
		if derived == "" {
			derived = "indeterminate"
		}
		_, _ = fmt.Fprintf(bw, "  - %s (status %s; derived %s)\n", unmet.Slug, unmet.Status, strings.TrimSpace(derived))
	}
	return bw.Flush()
}
