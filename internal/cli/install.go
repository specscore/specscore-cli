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
		Errors: installErrors{},
		HostID: specscoreCatalogID,
	})
}

// installErrors implements cobracmd.ErrorMapper for specscore's own install
// command, keeping the exit-code contract
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
type installErrors struct{}

// Failure maps every install failure per the kinds table on installErrors.
//
// A nil err IS a real, reachable call on the ordinary success and dry-run
// path, not just a defensive guard: cliinstall/cobracmd v0.20.0's
// runInstall calls mapFailure(opts, plan.Failure()) and mapFailure(opts,
// result.Failure()) unconditionally, and both return nil for a fully
// successful batch, so opts.Errors.Failure(nil) is called on every
// successful `specscore install` and `specscore install <name> --dry-run`
// run. Feedback for cli-helpers (known bug, unchanged as of v0.20.0):
// mapFailure itself should short-circuit nil before calling
// opts.Errors.Failure, matching what ErrorMapper.Failure's own doc comment
// already promises ("maps a non-nil command error") — see ingitdb-cli's
// identical nil-guard note on its own install command for the same
// observation against the same library version.
func (installErrors) Failure(err error) error {
	if err == nil {
		return nil
	}

	var usage *cobracmd.UsageError
	if errors.As(err, &usage) {
		return exitcode.InvalidArgsErrorf("install: %v", err)
	}

	switch selfupdate.KindOf(err) {
	case selfupdate.KindUnknownTarget:
		return exitcode.InvalidArgsErrorf("install: %v", err)
	case selfupdate.KindNoInstallDir, selfupdate.KindDestinationExists:
		return exitcode.InvalidStateErrorf("install: %v", err)
	case selfupdate.KindPermission, selfupdate.KindAmbiguous, selfupdate.KindDowngrade,
		selfupdate.KindNonInteractive, selfupdate.KindChecksum, selfupdate.KindManagedVersion:
		return exitcode.InvalidStateErrorf("install: %v", err)
	case selfupdate.KindReleaseLookup, selfupdate.KindDownload, selfupdate.KindUnknownTag, selfupdate.KindUnsupportedPlatform:
		return exitcode.NotFoundErrorf("install: %v", err)
	case selfupdate.KindManagedCommand:
		return exitcode.New(selfUpdateUnexpectedCode, "install: "+err.Error())
	default: // selfupdate.KindUnexpected, and any kind a future library version adds.
		return exitcode.New(selfUpdateUnexpectedCode, "install: "+err.Error())
	}
}
