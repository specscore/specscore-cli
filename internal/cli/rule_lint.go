package cli

import (
	"fmt"
	"path/filepath"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/spf13/cobra"
)

func ruleLintCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Run only the Rule lint family (R-001..R-009)",
		Long: `Runs the R-family rules over spec/rules/ and reports every violation.

This is a focused view of what ` + "`specscore spec lint`" + ` already runs: format and
required fields (R-001), status vocabulary (R-002), index completeness and row
sync (R-003/R-004), enforcement-needs-control (R-005), scope grammar (R-006),
source resolution (R-007), the strict lesson<->rule pair (R-008), and
supersession integrity (R-009).

--fix regenerates a missing or drifted rules index. It never edits a Rule
artifact: a Rule's fields are the author's, and a fixer that rewrote them would
be guessing at normative text.

Exits 0 when clean, 1 when any error-severity violation is reported.`,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runRuleLint,
	}
	cmd.Flags().Bool("fix", false, "regenerate a missing or drifted rules index")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("format", "text", "output format: text, yaml, json")
	return cmd
}

func runRuleLint(cmd *cobra.Command, _ []string) error {
	format, _ := cmd.Flags().GetString("format")
	if err := validateFormat(format); err != nil {
		return err
	}
	fix, _ := cmd.Flags().GetBool("fix")
	projectFlag, _ := cmd.Flags().GetString("project")
	root, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	violations, err := lintRunFn(lint.Options{
		SpecRoot:    filepath.Join(root, "spec"),
		ProjectRoot: root,
		Rules:       lint.RuleFamilyIDs(),
		Severity:    "error",
		Fix:         fix,
		FixTargets:  lint.RuleFamilyIDs(),
	})
	if err != nil {
		return exitcode.UnexpectedErrorf("running rule lint: %v", err)
	}

	w := cmd.OutOrStdout()
	switch format {
	case "json":
		if violations == nil {
			violations = []lint.Violation{}
		}
		if err := newJSONEnc(w).Encode(violations); err != nil {
			return err
		}
	case "yaml":
		enc := newYAMLEnc(w)
		if err := enc.Encode(violations); err != nil {
			return exitcode.UnexpectedErrorf("encoding yaml: %v", err)
		}
		if err := enc.Close(); err != nil {
			return err
		}
	default:
		for _, v := range violations {
			_, _ = fmt.Fprintf(w, "%s:%d [%s] %s\n", v.File, v.Line, v.Rule, v.Message)
		}
	}

	for _, v := range violations {
		if v.Severity == "error" {
			return exitcode.New(exitcode.Conflict, fmt.Sprintf("%d rule violation(s)", len(violations)))
		}
	}
	return nil
}
