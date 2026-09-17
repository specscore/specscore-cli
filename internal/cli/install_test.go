package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/strongo/cli-helpers/cliinstall/cobracmd"
	"github.com/strongo/cli-helpers/selfupdate"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// AC: cli/install#req:command-name, cli/install#req:specscore-host-identity
// — the command is registered under specscore's own catalog id and does not
// panic (cobracmd.New panics when the host id is absent from the compiled
// catalog).
func TestInstall_Registration(t *testing.T) {
	cmd := installCommand()
	if cmd.Name() != "install" {
		t.Errorf("Name() = %q, want %q", cmd.Name(), "install")
	}
}

// cli-install#req:host-identity-from-catalog — a host id absent from the
// compiled catalog is a programming error the command constructor panics
// on. This proves the mechanism cobracmd.New documents, using a bogus id
// rather than "specscore" (which always exists).
func TestInstall_PanicsForUnknownHostID(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected cobracmd.New to panic for an unregistered host id")
		}
	}()
	cobracmd.New(cobracmd.CommandOptions{HostID: "nosuchhost-specscore-test"})
}

// AC: cli-install#ac:direct-install-writes-only-verified-new-files,
// cli-install#req:unknown-target-refused — `specscore install nosuchcli`
// MUST fail before any confirmation, network request or write, with exit
// code 2 (InvalidArgs) and a message naming the unknown target, carrying no
// "self-update:" prefix. Plan() rejects every unknown name before probing
// anything, so this is offline-safe with no injected Env.
func TestInstall_UnknownTargetExits2(t *testing.T) {
	cmd := installCommand()
	cmd.SetArgs([]string{"nosuchcli"})
	var out, errOut strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error for an unknown install target")
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
		t.Errorf("message %q carries a self-update: prefix; install errors MUST NOT (cli-install#req:host-owned-exit-codes)", err.Error())
	}
	if !strings.HasPrefix(err.Error(), "install: ") {
		t.Errorf("message %q does not start with the exact \"install: \" prefix (S3 review fix)", err.Error())
	}
}

// AC: cli-install#req:host-owned-exit-codes — installErrors.Failure maps
// every kind per cli/install#req:exit-code-contract's table, and no message
// carries a "self-update:" prefix.
func TestInstallErrors_FailureExitCodes(t *testing.T) {
	cases := []struct {
		name string
		kind selfupdate.FailureKind
		want int
	}{
		{"unknown_target", selfupdate.KindUnknownTarget, exitcode.InvalidArgs},
		{"no_install_dir", selfupdate.KindNoInstallDir, exitcode.InvalidState},
		{"destination_exists", selfupdate.KindDestinationExists, exitcode.InvalidState},
		{"ambiguous", selfupdate.KindAmbiguous, exitcode.InvalidState},
		{"release_lookup", selfupdate.KindReleaseLookup, exitcode.NotFound},
		{"download", selfupdate.KindDownload, exitcode.NotFound},
		{"checksum", selfupdate.KindChecksum, exitcode.InvalidState},
		{"permission", selfupdate.KindPermission, exitcode.InvalidState},
		{"non_interactive", selfupdate.KindNonInteractive, exitcode.InvalidState},
		{"downgrade", selfupdate.KindDowngrade, exitcode.InvalidState},
		{"unknown_tag", selfupdate.KindUnknownTag, exitcode.NotFound},
		{"unsupported_platform", selfupdate.KindUnsupportedPlatform, exitcode.NotFound},
		{"managed_version", selfupdate.KindManagedVersion, exitcode.InvalidState},
		{"managed_command", selfupdate.KindManagedCommand, selfUpdateUnexpectedCode},
		{"unexpected", selfupdate.KindUnexpected, selfUpdateUnexpectedCode},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := installErrors{cmd: "install"}.Failure(&selfupdate.Failure{Kind: c.kind, Err: errors.New("boom")})
			ec, ok := err.(exitCoder)
			if !ok {
				t.Fatalf("error %T does not expose ExitCode()", err)
			}
			if got := ec.ExitCode(); got != c.want {
				t.Errorf("ExitCode() = %d, want %d", got, c.want)
			}
			if strings.Contains(err.Error(), "self-update:") {
				t.Errorf("message %q carries a self-update: prefix; install errors MUST NOT", err.Error())
			}
			if !strings.HasPrefix(err.Error(), "install: ") {
				t.Errorf("message %q does not start with the exact \"install: \" prefix (S3 review fix: the prefix names the command the user ran)", err.Error())
			}
		})
	}
}

// A nil err IS a real, reachable call on the success/dry-run path — see
// installErrors.Failure's own doc comment for why cliinstall/cobracmd
// v0.20.0 calls opts.Errors.Failure(nil) on every successful run.
func TestInstallErrors_FailureNilIsNil(t *testing.T) {
	err := installErrors{cmd: "install"}.Failure(nil)
	if err != nil {
		t.Errorf("Failure(nil) = %v, want nil", err)
	}
}

// *cobracmd.UsageError (an invalid --format, or --all combined with names)
// MUST map to exitcode.InvalidArgs (2), matching every other specscore
// command's convention for a malformed argument.
func TestInstallErrors_FailureUsageError(t *testing.T) {
	err := installErrors{cmd: "install"}.Failure(&cobracmd.UsageError{Err: errors.New("invalid --format")})
	ec, ok := err.(exitCoder)
	if !ok {
		t.Fatalf("error %T does not expose ExitCode()", err)
	}
	if ec.ExitCode() != exitcode.InvalidArgs {
		t.Errorf("ExitCode() = %d, want %d (InvalidArgs)", ec.ExitCode(), exitcode.InvalidArgs)
	}
	if strings.Contains(err.Error(), "self-update:") {
		t.Errorf("message %q carries a self-update: prefix; install errors MUST NOT", err.Error())
	}
}

// A non-*selfupdate.Failure error (selfupdate.KindOf returns KindUnexpected
// for anything that isn't one) still maps to the unexpected code rather than
// panicking or losing the underlying message.
func TestInstallErrors_FailureWrapsPlainError(t *testing.T) {
	err := installErrors{cmd: "install"}.Failure(errors.New("not a *selfupdate.Failure"))
	ec, ok := err.(exitCoder)
	if !ok {
		t.Fatalf("error %T does not expose ExitCode()", err)
	}
	if got := ec.ExitCode(); got != selfUpdateUnexpectedCode {
		t.Errorf("ExitCode() = %d, want %d", got, selfUpdateUnexpectedCode)
	}
	if !strings.Contains(err.Error(), "not a *selfupdate.Failure") {
		t.Errorf("error message %q lost the underlying error text", err.Error())
	}
}

// installErrors and selfUpdateErrors MUST agree on the exit code for every
// kind self-update itself already classifies
// (cli-install#req:host-owned-exit-codes: "a host maps it as its
// self-update maps UpdateAvailable" — extended here to every shared
// failure kind), so `install <name>` and `self-update` give the same code
// for the same underlying failure.
func TestInstallErrors_SharedKindsMatchSelfUpdateErrors(t *testing.T) {
	shared := []selfupdate.FailureKind{
		selfupdate.KindAmbiguous, selfupdate.KindReleaseLookup, selfupdate.KindDownload,
		selfupdate.KindChecksum, selfupdate.KindPermission, selfupdate.KindNonInteractive,
		selfupdate.KindDowngrade, selfupdate.KindUnknownTag, selfupdate.KindUnsupportedPlatform,
		selfupdate.KindManagedVersion, selfupdate.KindManagedCommand, selfupdate.KindUnexpected,
	}
	for _, kind := range shared {
		t.Run(kind.String(), func(t *testing.T) {
			installErr := installErrors{cmd: "install"}.Failure(&selfupdate.Failure{Kind: kind, Err: errors.New("boom")})
			selfUpdateErr := selfUpdateErrors{}.Failure(&selfupdate.Failure{Kind: kind, Err: errors.New("boom")})
			installEC, ok := installErr.(exitCoder)
			if !ok {
				t.Fatalf("install error %T does not expose ExitCode()", installErr)
			}
			selfUpdateEC, ok := selfUpdateErr.(exitCoder)
			if !ok {
				t.Fatalf("self-update error %T does not expose ExitCode()", selfUpdateErr)
			}
			if installEC.ExitCode() != selfUpdateEC.ExitCode() {
				t.Errorf("install ExitCode() = %d, self-update ExitCode() = %d; want equal for shared kind %v",
					installEC.ExitCode(), selfUpdateEC.ExitCode(), kind)
			}
		})
	}
}
