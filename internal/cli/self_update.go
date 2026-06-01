package cli

import (
	"context"
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

// resolveLatest resolves the latest stable release tag. It is a package-level
// variable so tests can stub the result (and an error case) without network
// access, mirroring the detectInstall hook.
var resolveLatest = func(ctx context.Context) (string, error) {
	return selfupdate.Resolver{}.LatestStableTag(ctx)
}

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

			if check, _ := cmd.Flags().GetBool("check"); check {
				return runCheck(cmd, detection)
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

// runCheck implements the read-only --check mode for any install method. It
// resolves the latest stable release, reports availability and the appropriate
// next step, and performs no download or filesystem write. Exit-code contract:
// up-to-date returns nil (exit 0); available/undetermined returns an exit-10
// error; a release-lookup failure returns a distinct exit-3 error so it never
// collides with the exit-10 "update available" code.
func runCheck(cmd *cobra.Command, detection selfupdate.Detection) error {
	latest, err := resolveLatest(cmd.Context())
	if err != nil {
		return exitcode.NotFoundErrorf("self-update --check: could not resolve the latest release: %v", err)
	}

	result := selfupdate.Compare(version, latest)
	out := cmd.OutOrStdout()

	switch result.Verdict {
	case selfupdate.UpToDate:
		_, _ = fmt.Fprintf(out, "specscore is up to date (%s).\n", result.Current)
		return nil
	case selfupdate.Undetermined:
		_, _ = fmt.Fprintf(out, "current specscore version is undetermined (%s); latest stable is %s.\n", result.Current, result.Latest)
	default:
		_, _ = fmt.Fprintf(out, "update available: %s → %s\n", result.Current, result.Latest)
	}

	// Print the appropriate next step for the detected install method. No
	// download or write happens on any of these paths.
	switch detection.Method {
	case selfupdate.Managed:
		if upgrade, ok := selfupdate.UpgradeCommand(detection.Manager); ok {
			_, _ = fmt.Fprintf(out,
				"specscore was installed via %s. Run the following to upgrade:\n\n    %s\n",
				selfupdate.ManagerName(detection.Manager), upgrade)
		}
	case selfupdate.Manual:
		_, _ = fmt.Fprintln(out, "To upgrade, run: specscore self-update")
	default:
		_, _ = fmt.Fprintln(out, ambiguousGuidance)
	}

	return exitcode.New(result.Verdict.ExitCode(), "")
}
