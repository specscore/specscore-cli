package cli

import (
	"strings"
	"testing"

	"github.com/strongo/cli-helpers/cliinstall"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// AC: cli-install#req:core-framework-neutral, cli-install#req:update-alias-
// policy — the command is named "upgrade", registers the shared flag
// surface, and carries no "update" alias (that alias stays on self-update
// only).
func TestUpgrade_Registration(t *testing.T) {
	cmd := upgradeCommand()
	if cmd.Name() != "upgrade" {
		t.Errorf("Name() = %q, want %q", cmd.Name(), "upgrade")
	}
	for _, name := range []string{"all", "check", "yes", "dry-run", "format"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("flag %q is not registered", name)
		}
	}
	// upgrade has no --dir flag (cli-install#req: it always acts on the
	// copy status-probing already located).
	if cmd.Flags().Lookup("dir") != nil {
		t.Error("upgrade must not register --dir")
	}
	for _, alias := range cmd.Aliases {
		if alias == "update" {
			t.Errorf("upgrade command carries an %q alias; REQ: update-alias-policy forbids it", alias)
		}
	}
}

// AC: cli-install#ac:direct-install-writes-only-verified-new-files,
// cli-install#req:unknown-target-refused — `specscore upgrade nosuchcli`
// MUST fail before any confirmation, network request or write, with exit
// code 2 (InvalidArgs), the same code `install nosuchcli` returns, carrying
// no "self-update:" prefix.
func TestUpgrade_UnknownTargetExits2(t *testing.T) {
	cmd := upgradeCommand()
	cmd.SetArgs([]string{"nosuchcli"})
	var out, errOut strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error for an unknown upgrade target")
	}
	ec, ok := err.(exitCoder)
	if !ok {
		t.Fatalf("error %T does not expose ExitCode()", err)
	}
	if ec.ExitCode() != exitcode.InvalidArgs {
		t.Errorf("ExitCode() = %d, want %d (InvalidArgs)", ec.ExitCode(), exitcode.InvalidArgs)
	}
	if !strings.Contains(err.Error(), "nosuchcli") {
		t.Errorf("message %q does not name the unknown target", err.Error())
	}
	if strings.Contains(err.Error(), "self-update:") {
		t.Errorf("message %q carries a self-update: prefix; upgrade errors MUST NOT", err.Error())
	}
}

// cli-install#req:upgrade-check — UpgradesAvailable MUST map to the exact
// same exit code and empty-message shape self-update's own UpdateAvailable
// already uses (selfUpdateErrors.UpdateAvailable), since specscore's
// self-update maps "update available" to exit 10 with an empty message.
func TestUpgradeErrors_UpgradesAvailableMapsLikeSelfUpdateCheck(t *testing.T) {
	err := upgradeErrors{}.UpgradesAvailable([]cliinstall.UpgradeResult{{Target: "wb"}})
	ec, ok := err.(exitCoder)
	if !ok {
		t.Fatalf("error %T does not expose ExitCode()", err)
	}
	if got := ec.ExitCode(); got != 10 {
		t.Errorf("ExitCode() = %d, want 10", got)
	}
	if err.Error() != "" {
		t.Errorf("Error() = %q, want empty (silentSignalErrorHandler relies on this)", err.Error())
	}
}

// upgradeErrors.Failure is inherited unchanged from installErrors — this
// proves the promotion actually happens and a nil error still maps to nil
// (cli-install#req:host-owned-exit-codes: "The upgrade command MUST use the
// same error mapper").
func TestUpgradeErrors_FailureIsInstallErrorsFailure(t *testing.T) {
	if got := (upgradeErrors{}).Failure(nil); got != nil {
		t.Errorf("Failure(nil) = %v, want nil", got)
	}
}

// --- end-to-end wiring: cobracmd.NewUpgrade + selfupdate.Config.Check
// --- against a fake GitHub releases endpoint, proving `self-update` and
// --- `upgrade specscore` reach the same verdict from the SAME HostConfig
// --- (cli-install#req:self-update-equals-upgrade-self,
// --- cli-install#req:host-target-is-running-binary). Both commands resolve
// --- their Config through the shared selfUpdateConfigFunc seam
// --- (upgradeCommand calls selfUpdateConfigFunc() for HostConfig, exactly
// --- as selfUpdateCommand does for its own Config), so overriding that one
// --- seam points both at the same fixture server — no network involved.

func TestUpgrade_SelfUpdateEqualsUpgradeSelf(t *testing.T) {
	t.Run("update available: both exit 10 with the current/latest verdict", func(t *testing.T) {
		withVersion(t, "2.0.0")
		withFakeReleases(t, releaseServer(t, `[{"tag_name":"v2.1.0","prerelease":false,"draft":false}]`))

		selfCmd := selfUpdateCommand()
		var selfOut strings.Builder
		selfCmd.SetOut(&selfOut)
		selfCmd.SetArgs([]string{"--check"})
		selfErr := selfCmd.Execute()
		selfEC, ok := selfErr.(exitCoder)
		if !ok {
			t.Fatalf("self-update --check error %T does not expose ExitCode()", selfErr)
		}

		upCmd := upgradeCommand()
		var upOut strings.Builder
		upCmd.SetOut(&upOut)
		upCmd.SetArgs([]string{"specscore", "--check"})
		upErr := upCmd.Execute()
		upEC, ok := upErr.(exitCoder)
		if !ok {
			t.Fatalf("upgrade specscore --check error %T does not expose ExitCode()", upErr)
		}

		if selfEC.ExitCode() != upEC.ExitCode() {
			t.Errorf("self-update --check exit = %d, upgrade specscore --check exit = %d; want equal", selfEC.ExitCode(), upEC.ExitCode())
		}
		if selfEC.ExitCode() != 10 {
			t.Errorf("self-update --check exit = %d, want 10", selfEC.ExitCode())
		}
		for _, out := range []string{selfOut.String(), upOut.String()} {
			if !strings.Contains(out, "2.0.0") || !strings.Contains(out, "2.1.0") {
				t.Errorf("output %q does not report current -> latest", out)
			}
		}
	})

	t.Run("up to date: both exit 0", func(t *testing.T) {
		withVersion(t, "3.0.0")
		withFakeReleases(t, releaseServer(t, `[{"tag_name":"v3.0.0","prerelease":false,"draft":false}]`))

		selfCmd := selfUpdateCommand()
		selfCmd.SetOut(&strings.Builder{})
		selfCmd.SetArgs([]string{"--check"})
		if err := selfCmd.Execute(); err != nil {
			t.Fatalf("self-update --check returned error (want exit 0): %v", err)
		}

		upCmd := upgradeCommand()
		upCmd.SetOut(&strings.Builder{})
		upCmd.SetArgs([]string{"specscore", "--check"})
		if err := upCmd.Execute(); err != nil {
			t.Fatalf("upgrade specscore --check returned error (want exit 0): %v", err)
		}
	})
}

// cli-install#req:upgrade-no-args-reports — the bare report (no names, no
// --all) MUST exit successfully whether or not an upgrade is available: it
// never consults UpgradesAvailable at all, unlike an explicit --check.
func TestUpgrade_NoArgsReportsNeverSignalsAvailable(t *testing.T) {
	withVersion(t, "1.0.0")
	withFakeReleases(t, releaseServer(t, `[{"tag_name":"v1.1.0","prerelease":false,"draft":false}]`))

	cmd := upgradeCommand()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("bare `upgrade` returned error (want exit 0 regardless of verdict): %v", err)
	}
	if out.Len() == 0 {
		t.Error("bare upgrade report produced no output")
	}
}
