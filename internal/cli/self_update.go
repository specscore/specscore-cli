package cli

import (
	"errors"
	"fmt"

	"github.com/specscore/specscore-cli/internal/selfupdate"
	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/spf13/cobra"
)

// ambiguousGuidance is shown when the install method cannot be confidently
// classified. It tells the user the situation is ambiguous and how to update
// manually instead of letting self-update guess.
const ambiguousGuidance = `specscore could not determine how this binary was installed, so the install method is ambiguous.
To avoid replacing a binary that may be managed by a package manager, self-update will not modify it.

To update manually, either:
  - re-download the latest release from https://github.com/specscore/specscore-cli/releases, or
  - upgrade via the package manager you used to install specscore.`

// detectInstall resolves how the running binary was installed. It is a
// package-level variable so tests can override it to force a specific
// detection without touching the real os.Executable path.
var detectInstall = selfupdate.DetectSelf

// selfUpdateCommand returns the "self-update" command (aliased "update"),
// which updates the installed specscore binary in place.
//
// The canonical name and the "update" alias resolve to the same command, so
// `specscore self-update` and `specscore update` are interchangeable. The
// --check flag reports whether a newer release is available without applying
// it; --yes (-y) skips the interactive confirmation prompt.
//
// RunE dispatches on the detected install method. Package-managed installs are
// redirected to the owning package manager's upgrade command and never touch
// the filesystem. Manual and ambiguous paths are placeholders that land in
// later tasks.
func selfUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "self-update",
		Aliases: []string{"update"},
		Short:   "Update the installed specscore binary in place",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			detection, err := detectInstall()
			if err != nil {
				return err
			}

			switch detection.Method {
			case selfupdate.Managed:
				// Redirect to the owning package manager. No filesystem
				// writes/downloads happen on this path; we print and exit 0.
				upgrade, ok := selfupdate.UpgradeCommand(detection.Manager)
				if !ok {
					return errors.New("self-update: managed install detected but no upgrade command is known")
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(),
					"specscore was installed via %s. Run the following to upgrade:\n\n    %s\n",
					selfupdate.ManagerName(detection.Manager), upgrade)
				return nil
			case selfupdate.Manual:
				// TODO(task 6/7/8/9/10): manual self-replace path
				return errors.New("self-update: manual install path not yet implemented")
			default:
				// Ambiguous: the install method cannot be confidently
				// classified. Refuse to self-replace, print manual-update
				// guidance, and exit non-zero. Ambiguity must never resolve
				// to the self-replace path, so we return before any
				// download/write.
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), ambiguousGuidance)
				return exitcode.InvalidStateError("self-update: install method is ambiguous; refusing to self-replace")
			}
		},
	}
	cmd.Flags().Bool("check", false, "report whether a newer release is available without applying it")
	cmd.Flags().BoolP("yes", "y", false, "skip the interactive confirmation prompt")
	return cmd
}
