package cli

// specscore: feature/cli/install

import (
	"github.com/spf13/cobra"
	"github.com/strongo/cli-helpers/cliinstall"
	"github.com/strongo/cli-helpers/cliinstall/cobracmd"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// upgradeCommand returns the "upgrade" command, built from
// github.com/strongo/cli-helpers/cliinstall/cobracmd against specscore's own
// catalog id (cli-install#req:host-identity-from-catalog). `specscore
// upgrade` (no arguments) reports every installed catalog CLI plus specscore
// itself; `specscore upgrade --all`/`specscore upgrade <name>...` upgrade
// what the report showed. specscore is always upgraded last, classified and
// versioned from its OWN self-update Config (never a PATH probe of its own
// binary), so `specscore self-update` and `specscore upgrade specscore`
// reach the exact same library call
// (cli-install#req:self-update-equals-upgrade-self,
// cli-install#req:host-target-is-running-binary). specscore has no
// after-update hook, so HostAfterUpdate is left nil.
func upgradeCommand() *cobra.Command {
	return cobracmd.NewUpgrade(cobracmd.UpgradeCommandOptions{
		Short:       "Upgrade installed fleet CLIs, including specscore itself",
		Errors:      upgradeErrors{},
		HostID:      specscoreCatalogID,
		HostConfig:  selfUpdateConfigFunc(),
		Interactive: selfUpdateInteractiveFunc,
	})
}

// upgradeErrors extends installErrors (see install.go) with the
// upgrades-available method cli-install#req:upgrade-check requires:
// "the Cobra adapter MUST call the host's error mapper's upgrades-available
// method ... a host maps it as its self-update maps UpdateAvailable."
// Failure itself is inherited unchanged from installErrors — the SAME
// exit-code table applies to `upgrade`'s failures as to `install`'s
// (cli-install#req:host-owned-exit-codes: "The upgrade command MUST use the
// same error mapper").
type upgradeErrors struct{ installErrors }

// UpgradesAvailable maps `upgrade --check`'s (or the bare report's, though
// that path never calls this — cli-install#req:upgrade-no-args-reports) "at
// least one target has an update available or an undetermined verdict"
// signal to the SAME exit code and empty-message shape
// selfUpdateErrors.UpdateAvailable already uses for `self-update --check`:
// exit 10, empty message, so silentSignalErrorHandler suppresses fang's own
// rendering of it and the human-readable verdict lines cobracmd already
// printed are all the output there is.
func (upgradeErrors) UpgradesAvailable([]cliinstall.UpgradeResult) error {
	return exitcode.New(selfUpdateCheckPendingCode, "")
}
