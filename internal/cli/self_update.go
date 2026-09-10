package cli

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/strongo/cli-helpers/selfupdate"
	"github.com/strongo/cli-helpers/selfupdate/cobracmd"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// Exit codes reserved for self-update's own operational-failure family.
// REQ: cli/self-update#req:exit-code-contract fixes 0 (success) and 10
// (--check reporting a verdict that is not up to date) globally; every
// operational error below MUST land on a code distinct from both.
//
// selfUpdateCheckPendingCode is returned by --check when the verdict is not
// up to date (update available or undetermined). It is numerically
// exitcode.Unexpected (10) — that was already this command's "update
// pending" code before this migration — but is named separately here so a
// reader doesn't mistake it for the *general* "unexpected runtime error"
// meaning that constant carries for every other specscore command.
const selfUpdateCheckPendingCode = exitcode.Unexpected

// selfUpdateUnexpectedCode covers selfupdate.KindUnexpected — a local
// staging/extraction/rename failure that isn't a permission error, or a
// failure resolving the running executable's own path. It is deliberately
// NOT selfUpdateCheckPendingCode (10): reusing that number for an unrelated
// internal failure would make the two indistinguishable to a script
// checking `$? == 10`. 9 is the one exitcode value no other specscore
// command has claimed.
//
// This is an intentional BEHAVIOR CHANGE, not a preserved one: the
// pre-migration internal/selfupdate code sometimes returned exitcode.
// Unexpected (10) here too (e.g. a tar/zip extraction failure), on the
// non-check path only, so it never collided with an actual --check
// invocation in practice — but it did collide with the documented meaning
// of exit 10 for this command. cli/self-update#req:exit-code-contract now
// states plainly that every operational error must avoid 10, so this closes
// that gap instead of reproducing it.
//
// The number lives in pkg/exitcode with every other code rather than as a
// literal here: exit codes are a CLI-wide contract, and a command that mints
// its own is how a vocabulary silently grows two meanings for one number —
// which is the very bug this constant exists to fix.
const selfUpdateUnexpectedCode = exitcode.UpdateFailed

// selfUpdateConfig returns specscore's own selfupdate.Config: its release
// identity, the package managers that publish it, and the version-probe
// arguments used to confirm a swap succeeded
// (cli/self-update#req:specscore-release-identity,
// cli/self-update#req:specscore-managers,
// cli/self-update#req:specscore-version-identity). Asset naming, the
// checksums filename, and the download URL are all left at the library's
// GoReleaser-shaped defaults — they already match .goreleaser.yml's own
// name_template ("specscore_<version>_<os>_<arch>" archives,
// "specscore_<version>_checksums.txt" checksums) exactly, so nothing here
// overrides them.
//
// Every manager is opted into EXECUTABLE upgrade mode via
// WithExecutableUpgrade: specscore MUST actually run the manager, not just
// print its command (this is the behavior change this migration exists for
// — see the founder's report that the previously-printed `brew upgrade
// specscore` fails against a cask install). UpgradeCommand stays the
// human-readable text shown in the availability preview and in --check's
// guidance; UpgradeExecutable/UpgradeArgs is the structured argv the
// library actually invokes (github.com/strongo/cli-helpers/selfupdate/
// cliui.ManagedCommandRunner — wired automatically by cobracmd.New, never
// through a shell).
//
//   - Homebrew: specscore ships as a CASK in tap specscore/tap (not a
//     formula), so both the display command and the executable argv MUST
//     be `brew upgrade --cask specscore`. `brew upgrade specscore` (the
//     pre-migration text) fails with "Treating specscore as a formula ...
//     specscore/tap/specscore not installed" — reproduced against the
//     founder's real Homebrew install before this fix.
//   - Scoop: `scoop update specscore`, argv ["update", "specscore"].
//   - WinGet: the display text stays `winget upgrade SpecScore.CLI` (the
//     pre-migration, human-typed form), but the executable argv uses
//     `--id SpecScore.CLI` instead of the bare package name. `winget
//     upgrade <name>` resolves by a fuzzy name/moniker match and can find
//     zero or multiple candidates depending on what else is installed;
//     `--id` selects the exact package deterministically, which matters
//     far more once this runs non-interactively than it does for a command
//     a human reads and can adjust. This manager's argv is UNVERIFIED in
//     this environment (no Windows host to run a real `winget` against);
//     flagged in the migration's report per the runbook-executed-before-
//     published rule.
func selfUpdateConfig() selfupdate.Config {
	return selfupdate.Config{
		BinaryName:           "specscore",
		Repository:           "specscore/specscore-cli",
		CurrentVersion:       buildInfo.Version,
		UndeterminedVersions: []string{"dev"},
		Managers: []selfupdate.Manager{
			selfupdate.Homebrew("brew upgrade --cask specscore").
				WithExecutableUpgrade("brew", "upgrade", "--cask", "specscore"),
			selfupdate.Scoop("scoop update specscore").
				WithExecutableUpgrade("scoop", "update", "specscore"),
			selfupdate.WinGet("winget upgrade SpecScore.CLI").
				WithExecutableUpgrade("winget", "upgrade", "--id", "SpecScore.CLI"),
		},
		VersionProbeArgs: []string{"--version"},
	}
}

// selfUpdateConfigFunc is a seam over selfUpdateConfig so tests can point a
// full command execution at an httptest.Server instead of the real GitHub
// API, mirroring the pattern the shared library's own reference CLI uses
// (github.com/strongo/cli-helpers/cmd/selfupdate's buildConfigFunc). Only
// tests override this.
var selfUpdateConfigFunc = selfUpdateConfig

// selfUpdateInteractiveFunc is a seam over cobracmd.CommandOptions.
// Interactive: nil in production, which lets cliui.Confirm fall back to its
// own real-TTY check (cliui.IsTerminal). Tests override this to drive the
// confirmation-prompt and non-interactive-refusal paths deterministically,
// instead of depending on whether `go test` itself happens to have a TTY
// attached.
var selfUpdateInteractiveFunc func() bool

// selfUpdateErrors maps github.com/strongo/cli-helpers/selfupdate's typed
// outcomes onto specscore's own pre-existing exit-code contract
// (cli/self-update#req:exit-code-contract), which this migration leaves
// unchanged: 0 for success, 10 reserved for --check reporting a verdict
// that is not up to date (update available or undetermined), and every
// operational error a code distinct from both. See self_update_test.go for
// the exhaustive kind → code table this type is required to hold.
type selfUpdateErrors struct{}

// Failure implements cobracmd.ErrorMapper. It classifies err via
// selfupdate.KindOf and returns the matching *exitcode.Error, preserving
// the exact codes specscore returned before this migration and extending
// the same two buckets to the two FailureKinds this migration's executable
// manager upgrades newly introduce:
//   - KindPermission: exitcode.InvalidState (4), with a remedy hint (sudo /
//     package manager) and the executable path, exactly as
//     classifySelfReplaceError reported it previously.
//   - KindAmbiguous, KindDowngrade, KindNonInteractive, KindChecksum,
//     KindManagedVersion: exitcode.InvalidState (4) — refusals and
//     state-guard failures. KindManagedVersion joins this group because it
//     is the same shape of failure — a `--version` pin the executable
//     manager path cannot honor (a redirect-only manager, an unreachable
//     managed-release lookup, or a pin that isn't the manager's latest) is
//     a refusal to proceed, not a lookup miss or a local write failure.
//   - KindReleaseLookup, KindDownload, KindUnknownTag,
//     KindUnsupportedPlatform: exitcode.NotFound (3) — the release or asset
//     could not be located.
//   - KindManagedCommand: selfUpdateUnexpectedCode (9) — the configured
//     manager runner/verifier is missing (a specscore wiring bug, since
//     cobracmd.New always supplies both) or the manager's own process
//     (e.g. `brew`) failed. Neither is a release-lookup miss nor a
//     state-guard refusal; it groups with the other "local operation
//     failed" causes below rather than getting its own bucket.
//   - Anything else (KindUnexpected, and any kind a future library version
//     adds): selfUpdateUnexpectedCode (9).
func (selfUpdateErrors) Failure(err error) error {
	switch selfupdate.KindOf(err) {
	case selfupdate.KindPermission:
		path := failurePath(err)
		if path == "" {
			path = "the specscore executable"
		}
		return exitcode.InvalidStateErrorf(
			"self-update: permission denied writing %s: %v\n"+
				"Re-run with elevated permissions (sudo), or update via your package manager.",
			path, err)
	case selfupdate.KindAmbiguous, selfupdate.KindDowngrade, selfupdate.KindNonInteractive, selfupdate.KindChecksum, selfupdate.KindManagedVersion:
		return exitcode.InvalidStateErrorf("self-update: %v", err)
	case selfupdate.KindReleaseLookup, selfupdate.KindDownload, selfupdate.KindUnknownTag, selfupdate.KindUnsupportedPlatform:
		return exitcode.NotFoundErrorf("self-update: %v", err)
	case selfupdate.KindManagedCommand:
		return exitcode.New(selfUpdateUnexpectedCode, "self-update: "+err.Error())
	default: // selfupdate.KindUnexpected, and any kind a future library version adds.
		return exitcode.New(selfUpdateUnexpectedCode, "self-update: "+err.Error())
	}
}

// UpdateAvailable implements cobracmd.ErrorMapper: --check reporting
// anything other than up to date (update available OR undetermined,
// per cli/self-update#req:exit-code-contract) exits 10 with an EMPTY
// message. The human-readable verdict line was already printed by
// cobracmd's own --check output; an empty exitcode.Error message is
// silentSignalErrorHandler's signal (see telemetry_wiring.go) to suppress
// fang's own rendering of it, exactly as self-update's --check behaved
// before this migration.
func (selfUpdateErrors) UpdateAvailable(selfupdate.CheckResult) error {
	return exitcode.New(selfUpdateCheckPendingCode, "")
}

// failurePath extracts the executable Path from err when it is (or wraps) a
// *selfupdate.Failure, and "" otherwise.
func failurePath(err error) string {
	var f *selfupdate.Failure
	if errors.As(err, &f) {
		return f.Path
	}
	return ""
}

// selfUpdateCommand returns the "self-update" command (aliased "update"),
// which updates the installed specscore binary in place. All detection,
// release-resolution, download, verification, and replacement behavior
// comes from github.com/strongo/cli-helpers/selfupdate
// (cli/self-update#req:library-provided-behavior); this function supplies
// only specscore's own identity (selfUpdateConfig) and exit-code contract
// (selfUpdateErrors).
//
// The canonical name and the "update" alias resolve to the same command, so
// `specscore self-update` and `specscore update` are interchangeable
// (cli/self-update#req:command-and-alias). --check reports availability
// without applying it, and states the next step — the manager's upgrade
// command for a managed install that only redirects, "run this command
// again" for a managed install that executes its manager, "specscore
// self-update" itself for a manual one, or the ambiguous-install guidance.
// --yes (-y) skips the interactive confirmation prompt; --version pins a
// release tag (refused for a managed install whose manager cannot honor a
// pin — cli/self-update#req:specscore-managers); --allow-downgrade permits
// a pinned target older than the running build
// (cli/self-update#req:flag-surface — this Feature's documented floor).
// --dry-run (report what would happen without downloading, writing, or
// running the manager) comes from the library for free and is left enabled.
//
// Every configured Manager is opted into EXECUTABLE upgrade mode
// (selfUpdateConfig), so a managed install is actually upgraded by running
// that manager — cobracmd.New wires the process runner
// (cliui.ManagedCommandRunner, argv only, never a shell) and the post-
// upgrade version probe (cliui.VerifyManagedBinary, against
// VersionProbeArgs) automatically; specscore supplies no exec code of its
// own (cli/self-update#req:library-provided-behavior).
func selfUpdateCommand() *cobra.Command {
	return cobracmd.New(selfUpdateConfigFunc(), cobracmd.CommandOptions{
		Short:       "Update the installed specscore binary in place",
		Aliases:     []string{"update"},
		Errors:      selfUpdateErrors{},
		Interactive: selfUpdateInteractiveFunc,
		// JSONFormat left false: specscore's Feature spec's flag surface
		// (cli/self-update#req:flag-surface) does not include --format, so
		// cobracmd never registers it. --dry-run IS registered — cobracmd.New
		// always adds it, and specscore's flag surface is a floor, not a
		// ceiling: "report what would happen without downloading or writing
		// anything" is useful on its own and costs nothing to expose.
	})
}
