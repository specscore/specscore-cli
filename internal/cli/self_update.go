package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

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

// isInteractive reports whether the process is attached to an interactive
// terminal. It is a package-level variable so tests can force a deterministic
// answer without depending on the test runner's stdin. The default inspects
// stdin's mode: a character device indicates a TTY rather than a pipe/file.
var isInteractive = func() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// doSelfReplace performs the actual download → verify → swap for a manual
// install, replacing the running executable with the release identified by
// latestTag. It is a package-level variable so tests can substitute a spy and
// never touch the network or filesystem. The default resolves the running
// executable's path, downloads and sha256-verifies the matching release asset,
// atomically swaps it in, then best-effort verifies the new binary's version.
var doSelfReplace = func(ctx context.Context, latestTag string) error {
	target, err := os.Executable()
	if err != nil {
		return exitcode.InvalidStateErrorf("self-update: could not resolve the running executable: %v", err)
	}
	tmp, err := selfupdate.Downloader{}.DownloadAndVerify(ctx, strings.TrimPrefix(latestTag, "v"), "", "")
	if err != nil {
		return err
	}
	if err := selfupdate.ReplaceExecutable(target, tmp); err != nil {
		return err
	}
	// Best-effort post-swap sanity check; a mismatch is surfaced but the swap
	// has already succeeded.
	_ = selfupdate.VerifyBinaryVersion(target, strings.TrimPrefix(latestTag, "v"))
	return nil
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
				if pinned, _ := cmd.Flags().GetString("version"); strings.TrimSpace(pinned) != "" {
					// Pinned target: install exactly this tag. Bypass the
					// stable-only resolveLatest/Compare path entirely so an
					// explicitly requested prerelease/draft installs as-is. The
					// tag is passed through (DownloadAndVerify strips a leading
					// "v" via AssetName); we only trim surrounding whitespace.
					pinnedTag := strings.TrimSpace(pinned)
					out := cmd.OutOrStdout()

					// Downgrade guard: when the running version is known (not the
					// "dev" placeholder) and the pinned target is strictly lower,
					// refuse unless --allow-downgrade is set. Direction can't be
					// determined for a dev build, so the guard does not trigger there.
					allowDowngrade, _ := cmd.Flags().GetBool("allow-downgrade")
					isDowngrade := version != selfupdate.DevVersion &&
						selfupdate.CompareVersions(pinnedTag, version) < 0
					if isDowngrade && !allowDowngrade {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
							"self-update: refusing to downgrade from %s to %s; pass --allow-downgrade to proceed\n",
							version, pinnedTag)
						return exitcode.InvalidStateErrorf(
							"self-update: refusing to downgrade from %s to %s; pass --allow-downgrade to proceed",
							version, pinnedTag)
					}

					if isDowngrade {
						_, _ = fmt.Fprintf(out, "downgrade: %s → %s\n", version, pinnedTag)
					} else {
						_, _ = fmt.Fprintf(out, "%s → %s\n", version, pinnedTag)
					}

					yes, _ := cmd.Flags().GetBool("yes")
					if !yes {
						if !isInteractive() {
							_, _ = fmt.Fprintln(cmd.ErrOrStderr(),
								"self-update: --yes is required for non-interactive use; refusing to replace the binary")
							return exitcode.InvalidStateError("self-update: --yes is required for non-interactive use")
						}
						_, _ = fmt.Fprint(out, "Proceed? [y/N] ")
						reader := bufio.NewReader(cmd.InOrStdin())
						line, _ := reader.ReadString('\n')
						if answer := strings.ToLower(strings.TrimSpace(line)); answer != "y" && answer != "yes" {
							_, _ = fmt.Fprintln(out, "self-update: aborted; binary left unchanged.")
							return nil
						}
					}

					if err := doSelfReplace(cmd.Context(), pinnedTag); err != nil {
						// Annotate with the pinned tag so an unknown-tag failure
						// (no matching published release or asset) names the tag
						// the user requested. The download/verify step runs before
						// any swap, so a failure here leaves the binary untouched.
						return classifySelfReplaceError(cmd, fmt.Errorf("release %s not found: %w", pinnedTag, err))
					}
					_, _ = fmt.Fprintf(out, "specscore updated to %s.\n", pinnedTag)
					return nil
				}
				latest, err := resolveLatest(cmd.Context())
				if err != nil {
					return exitcode.NotFoundErrorf("self-update: could not resolve the latest release: %v", err)
				}
				result := selfupdate.Compare(version, latest)
				if result.Verdict == selfupdate.UpToDate {
					// Already on the latest stable release. Report and exit 0
					// without downloading or replacing anything.
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "specscore is already up to date (%s).\n", result.Current)
					return nil
				}
				// Available/Undetermined: confirm (unless --yes), then swap.
				out := cmd.OutOrStdout()
				_, _ = fmt.Fprintf(out, "%s → %s\n", result.Current, result.Latest)

				yes, _ := cmd.Flags().GetBool("yes")
				if !yes {
					if !isInteractive() {
						// Refuse to block on input when there's no terminal and
						// no explicit consent. Leave the binary unchanged.
						_, _ = fmt.Fprintln(cmd.ErrOrStderr(),
							"self-update: --yes is required for non-interactive use; refusing to replace the binary")
						return exitcode.InvalidStateError("self-update: --yes is required for non-interactive use")
					}
					_, _ = fmt.Fprint(out, "Proceed? [y/N] ")
					reader := bufio.NewReader(cmd.InOrStdin())
					line, _ := reader.ReadString('\n')
					if answer := strings.ToLower(strings.TrimSpace(line)); answer != "y" && answer != "yes" {
						_, _ = fmt.Fprintln(out, "self-update: aborted; binary left unchanged.")
						return nil
					}
				}

				if err := doSelfReplace(cmd.Context(), latest); err != nil {
					return classifySelfReplaceError(cmd, err)
				}
				_, _ = fmt.Fprintf(out, "specscore updated to %s.\n", result.Latest)
				return nil
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
	// --version here is self-update-local: it pins the release tag to install
	// (leading "v" optional). It is distinct from the root `specscore --version`
	// flag, which prints the CLI's own build version.
	cmd.Flags().String("version", "", "install a specific release tag (leading \"v\" optional) instead of the latest")
	cmd.Flags().Bool("allow-downgrade", false, "permit installing a --version older than the running build")
	return cmd
}

// classifySelfReplaceError converts a non-nil error from doSelfReplace into a
// clear, actionable message and a non-zero *exitcode.Error, distinguishing the
// two write-path failure modes. The atomic swap is gated behind download +
// verification, so on any of these errors the original binary is untouched.
//
// Permission-denied: the executable's directory is not writable. We report the
// failure with the executable path (best-effort via os.Executable) and a
// suggested remedy. Network/lookup/download: anything else (connection refused,
// rate limit, missing asset, checksum mismatch). An *exitcode.Error is passed
// through (its code/message are already meaningful); a bare error is wrapped.
func classifySelfReplaceError(cmd *cobra.Command, err error) error {
	errOut := cmd.ErrOrStderr()

	if errors.Is(err, fs.ErrPermission) {
		target, _ := os.Executable()
		if target == "" {
			target = "the specscore executable"
		}
		_, _ = fmt.Fprintf(errOut,
			"self-update: permission denied writing %s: %v\n"+
				"Re-run with elevated permissions (sudo), or update via your package manager.\n",
			target, err)
		return exitcode.InvalidStateErrorf(
			"self-update: permission denied writing %s; re-run with elevated permissions (sudo) or update via your package manager",
			target)
	}

	// Network / lookup / download failure (or any other non-permission error).
	// Pass through a meaningful exitcode error; otherwise wrap as a release/
	// download failure.
	var ec *exitcode.Error
	if errors.As(err, &ec) {
		_, _ = fmt.Fprintf(errOut, "self-update: %v\n", err)
		// Return the unwrapped *exitcode.Error so the top-level runner's
		// ExitCode() convention sees a non-zero code even when err is a wrapper
		// (e.g. the pinned branch annotates with the requested tag via %w).
		return ec
	}
	_, _ = fmt.Fprintf(errOut, "self-update: failed to download the release: %v\n", err)
	return exitcode.NotFoundErrorf("self-update: failed to download the release: %v", err)
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
