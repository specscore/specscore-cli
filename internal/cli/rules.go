package cli

// Features implemented: cli/rules

import (
	"fmt"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/spf13/cobra"
)

// rulesCommand returns the "rules" command, which lists every registered lint
// rule with its id, family, and description in a deterministic order.
func rulesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rules",
		Short: "List every registered lint rule",
		Long: `Lists every registered lint rule with its id, family, and description in a
deterministic order. This is a read-only command that prints the rule catalog
and exits 0.`,
		RunE: runRules,
	}
	cmd.Flags().String("family", "", "restrict output to a single rule family")
	cmd.Flags().String("format", "text", "output format: text or json")
	cmd.Flags().Bool("write", false, "regenerate docs/lint-rules.md from the rule registry")
	return cmd
}

func runRules(cmd *cobra.Command, _ []string) error {
	// --write is an action flag: when set, regenerate the canonical catalog and
	// return without emitting the text/json listing.
	if write, _ := cmd.Flags().GetBool("write"); write {
		cwd, err := osGetwdFn()
		if err != nil {
			return exitcode.UnexpectedErrorf("cannot determine working directory: %v", err)
		}
		root, err := findRepoConfigRoot(cwd)
		if err != nil {
			return err
		}
		if err := lint.WriteCatalog(root); err != nil {
			return exitcode.UnexpectedErrorf("writing catalog: %v", err)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", lint.CatalogPath)
		return nil
	}

	family, _ := cmd.Flags().GetString("family")
	format, _ := cmd.Flags().GetString("format")

	if format != "text" && format != "json" {
		return exitcode.InvalidArgsErrorf("invalid --format value: %s (must be text or json)", format)
	}

	rules := lint.AllRules()
	if family != "" {
		filtered := rules[:0:0]
		for _, r := range rules {
			if r.Family == family {
				filtered = append(filtered, r)
			}
		}
		rules = filtered
	}

	w := cmd.OutOrStdout()

	if format == "json" {
		return newJSONEnc(w).Encode(rules)
	}

	// Pad the id column to the widest id so columns stay aligned and stable.
	idWidth := 0
	for _, r := range rules {
		if len(r.ID) > idWidth {
			idWidth = len(r.ID)
		}
	}

	for _, r := range rules {
		_, _ = fmt.Fprintf(w, "%-*s  [%s]  %s\n", idWidth, r.ID, r.Family, r.Description)
	}

	return nil
}
