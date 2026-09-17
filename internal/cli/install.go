package cli

// specscore: feature/cli/install

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/strongo/cli-helpers/cliinstall/cobracmd"
	"github.com/strongo/cli-helpers/selfupdate"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// installCommand returns the "install" command, built from
// github.com/strongo/cli-helpers/cliinstall/cobracmd against specscore's own
// catalog id (cli/install#req:specscore-host-identity,
// cli-install#req:host-identity-from-catalog). `specscore install` lists the
// fleet CLIs relevant to specscore (wb, ingitdb, synchestra, chatwright,
// codegrapher) with their live status, and `specscore install <name>...`
// installs them the same way specscore itself was installed.
// cobracmd.New panics when "specscore" is absent from the compiled catalog —
// a programming error TestInstall_Registration and the cobracmd package's
// own tests catch, never a runtime state a user sees.
func installCommand() *cobra.Command {
	return cobracmd.New(cobracmd.CommandOptions{
		Short:  "List and install fleet CLIs relevant to specscore",
		Errors: installErrors{cmd: "install"},
		HostID: specscoreCatalogID,
	})
}

// installErrors implements cobracmd.ErrorMapper for specscore's own install
// AND upgrade commands, keeping the exit-code contract
// cli/install#req:exit-code-contract documents:
//   - *cobracmd.UsageError (an invalid --format, or --all combined with
//     names): exitcode.InvalidArgs (2), the same bucket every other
//     specscore command uses for a malformed argument.
//   - selfupdate.KindUnknownTarget: exitcode.InvalidArgs (2) —
//     cli-install#req:unknown-target-refused's underlying error already
//     names the unknown target and lists valid catalog ids.
//   - selfupdate.KindNoInstallDir, selfupdate.KindDestinationExists:
//     exitcode.InvalidState (4) — the local filesystem/PATH state prevents
//     the requested operation.
//   - Every kind selfUpdateErrors.Failure already classifies (KindPermission,
//     KindAmbiguous, KindDowngrade, KindNonInteractive, KindChecksum,
//     KindManagedVersion, KindReleaseLookup, KindDownload, KindUnknownTag,
//     KindUnsupportedPlatform, KindManagedCommand, KindUnexpected) maps to
//     the exact SAME exit code self-update returns for that kind
//     (cli-install#req:host-owned-exit-codes: "a host maps it as its
//     self-update maps ..."), reimplemented here rather than delegated to
//     selfUpdateErrors{}.Failure because that method's own messages carry a
//     "self-update: " prefix this command's messages MUST NOT carry
//     ("MUST NOT let them fall into a self-update default branch or print a
//     self-update: message prefix").
//
// cmd names the command whose own messages this instance produces —
// "install" or "upgrade" (S3 review fix): the fleet-wide rule is that a
// message's prefix names the command the user actually ran, and
// upgradeErrors (upgrade.go) constructs its own installErrors{cmd:
// "upgrade"} rather than embedding a zero-value one, so `specscore upgrade
// nosuchcli` prints "upgrade: ..." and not "install: ...".
type installErrors struct{ cmd string }

// Failure maps every install/upgrade failure per the kinds table on
// installErrors, prefixing every message with e.cmd (cli-install#req:host-
// owned-exit-codes: "The upgrade command MUST use the same error mapper" —
// the SAME table applies to both commands' self-update-shared kinds; only
// the prefix differs per command).
//
// cliinstall/cobracmd v0.21.0's mapFailure short-circuits a nil err before
// ever calling opts.Errors.Failure (the fix for the known v0.20.0 bug this
// comment used to document), so Failure is never called with nil through
// that path anymore; the guard below stays only because it is trivially
// free and keeps this method nil-safe for any direct caller, including
// TestInstallErrors_FailureNilIsNil.
func (e installErrors) Failure(err error) error {
	if err == nil {
		return nil
	}

	var usage *cobracmd.UsageError
	if errors.As(err, &usage) {
		return exitcode.InvalidArgsErrorf("%s: %v", e.cmd, err)
	}

	switch selfupdate.KindOf(err) {
	case selfupdate.KindUnknownTarget:
		return exitcode.InvalidArgsErrorf("%s: %v", e.cmd, err)
	case selfupdate.KindNoInstallDir, selfupdate.KindDestinationExists:
		return exitcode.InvalidStateErrorf("%s: %v", e.cmd, err)
	case selfupdate.KindPermission, selfupdate.KindAmbiguous, selfupdate.KindDowngrade,
		selfupdate.KindNonInteractive, selfupdate.KindChecksum, selfupdate.KindManagedVersion:
		return exitcode.InvalidStateErrorf("%s: %v", e.cmd, err)
	case selfupdate.KindReleaseLookup, selfupdate.KindDownload, selfupdate.KindUnknownTag, selfupdate.KindUnsupportedPlatform:
		return exitcode.NotFoundErrorf("%s: %v", e.cmd, err)
	case selfupdate.KindManagedCommand:
		return exitcode.New(selfUpdateUnexpectedCode, e.cmd+": "+err.Error())
	default: // selfupdate.KindUnexpected, and any kind a future library version adds.
		return exitcode.New(selfUpdateUnexpectedCode, e.cmd+": "+err.Error())
	}
}
