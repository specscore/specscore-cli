package cli

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/strongo/cli-helpers/cliinstall/cobracmd"
	"github.com/strongo/cli-helpers/selfupdate"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

const specscoreHomebrewUninstallCommand = "brew uninstall --cask specscore"

// uninstallCommand returns the "uninstall" command, built from
// github.com/strongo/cli-helpers/cliinstall/cobracmd against specscore's catalog entry.
func uninstallCommand() *cobra.Command {
	return cobracmd.NewUninstall(cobracmd.UninstallCommandOptions{
		Short:  "Uninstall installed fleet CLIs",
		Errors: newUninstallErrors(),
		HostID: specscoreCatalogID,
	})
}

type uninstallErrors struct {
	prefix        string
	remedyCommand string
}

func newUninstallErrors() uninstallErrors {
	return uninstallErrors{prefix: "uninstall", remedyCommand: specscoreHomebrewUninstallCommand}
}

func (e uninstallErrors) Failure(err error) error {
	if err == nil {
		return nil
	}

	var usage *cobracmd.UsageError
	if errors.As(err, &usage) {
		return exitcode.InvalidArgsErrorf("%s: %v", e.prefix, err)
	}

	switch selfupdate.KindOf(err) {
	case selfupdate.KindPermission:
		path := failurePath(err)
		if path == "" {
			path = "the " + e.prefix + " destination"
		}
		return exitcode.InvalidStateErrorf(
			"%s: permission denied writing %s: %v\n"+
				"Re-run with elevated permissions (sudo), or run %s.",
			e.prefix, path, err, e.remedyCommand)
	case selfupdate.KindUnknownTarget:
		return exitcode.InvalidArgsErrorf("%s: %v", e.prefix, err)
	case selfupdate.KindNoInstallDir, selfupdate.KindDestinationExists:
		return exitcode.InvalidStateErrorf("%s: %v", e.prefix, err)
	case selfupdate.KindAmbiguous, selfupdate.KindDowngrade,
		selfupdate.KindNonInteractive, selfupdate.KindChecksum, selfupdate.KindManagedVersion:
		return exitcode.InvalidStateErrorf("%s: %v", e.prefix, err)
	case selfupdate.KindReleaseLookup, selfupdate.KindDownload, selfupdate.KindUnknownTag, selfupdate.KindUnsupportedPlatform:
		return exitcode.NotFoundErrorf("%s: %v", e.prefix, err)
	case selfupdate.KindManagedCommand:
		return exitcode.New(selfUpdateUnexpectedCode, e.prefix+": "+err.Error())
	default: // selfupdate.KindUnexpected, and any kind a future library version adds.
		return exitcode.New(selfUpdateUnexpectedCode, e.prefix+": "+err.Error())
	}
}
