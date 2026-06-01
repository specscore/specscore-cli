package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// selfUpdateCommand returns the "self-update" command (aliased "update"),
// which updates the installed specscore binary in place.
//
// The canonical name and the "update" alias resolve to the same command, so
// `specscore self-update` and `specscore update` are interchangeable. The
// --check flag reports whether a newer release is available without applying
// it; --yes (-y) skips the interactive confirmation prompt.
//
// NOTE: RunE is a thin placeholder for now. The detection/update path logic
// (release lookup, download, atomic replace, confirmation) lands in later
// tasks. It prints a single deterministic line so the canonical-and-alias
// equivalence AC can be verified with identical output for both call shapes.
func selfUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "self-update",
		Aliases: []string{"update"},
		Short:   "Update the installed specscore binary in place",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Placeholder: path logic lands in later tasks.
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "self-update: not yet implemented")
			return nil
		},
	}
	cmd.Flags().Bool("check", false, "report whether a newer release is available without applying it")
	cmd.Flags().BoolP("yes", "y", false, "skip the interactive confirmation prompt")
	return cmd
}
