package cli

// Features implemented: cli/rules

import (
	"fmt"

	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/spf13/cobra"
)

// rulesCommand returns the "rules" command, which lists every registered lint
// rule with its id, family, and description in a deterministic order.
func rulesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "rules",
		Short: "List every registered lint rule",
		Long: `Lists every registered lint rule with its id, family, and description in a
deterministic order. This is a read-only command that prints the rule catalog
and exits 0.`,
		RunE: runRules,
	}
}

func runRules(cmd *cobra.Command, _ []string) error {
	w := cmd.OutOrStdout()

	rules := lint.AllRules()

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
