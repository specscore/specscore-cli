package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/strongo/cli-helpers/cliinstall/cobracmd"
	"github.com/strongo/cli-helpers/selfupdate"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

func TestUninstallCmd_Registration(t *testing.T) {
	cmd := uninstallCommand()
	if cmd.Name() != "uninstall" {
		t.Errorf("Name() = %q, want %q", cmd.Name(), "uninstall")
	}
}

func TestUninstallCmd_RegisteredAtRoot(t *testing.T) {
	rootCmd, fangOpts := newRootCommand()
	var stdout, stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs([]string{"uninstall", "--help"})

	err := executeWithPanicRecovery(rootCmd, fangOpts...)
	if err != nil {
		t.Fatalf("specscore uninstall --help failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "uninstall") {
		t.Errorf("help output does not mention uninstall:\n%s", stdout.String())
	}
}

func TestUninstallCmd_PanicsForUnknownHostID(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected cobracmd.NewUninstall to panic for an unregistered host id")
		}
	}()
	cobracmd.NewUninstall(cobracmd.UninstallCommandOptions{HostID: "nosuchhost-specscore-test"})
}

func TestUninstallErrors_FailureExitCodes(t *testing.T) {
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
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			mapped := (newUninstallErrors()).Failure(&selfupdate.Failure{Kind: testCase.kind, Err: errors.New("boom")})
			var coded *exitcode.Error
			if !errors.As(mapped, &coded) {
				t.Fatalf("Failure(%v) did not return an *exitcode.Error: %v", testCase.kind, mapped)
			}
			if coded.ExitCode() != testCase.want {
				t.Errorf("code = %d, want %d", coded.ExitCode(), testCase.want)
			}
			if !strings.HasPrefix(coded.Error(), "uninstall: ") {
				t.Errorf("message %q does not carry the exact uninstall: prefix", coded.Error())
			}
		})
	}
}

func TestUninstallErrors_FailureUsageError(t *testing.T) {
	mapped := (newUninstallErrors()).Failure(&cobracmd.UsageError{Err: errors.New("invalid --format")})
	var coded *exitcode.Error
	if !errors.As(mapped, &coded) {
		t.Fatalf("Failure(usage error) did not return an *exitcode.Error: %v", mapped)
	}
	if coded.ExitCode() != exitcode.InvalidArgs {
		t.Errorf("code = %d, want InvalidArgs (%d)", coded.ExitCode(), exitcode.InvalidArgs)
	}
	if !strings.HasPrefix(coded.Error(), "uninstall: ") {
		t.Errorf("message %q does not carry the exact uninstall: prefix", coded.Error())
	}
}

func TestUninstallErrors_FailurePermissionNamesPathAndBrew(t *testing.T) {
	mapped := (newUninstallErrors()).Failure(&selfupdate.Failure{
		Kind: selfupdate.KindPermission,
		Path: "/usr/local/bin/specscore",
		Err:  errors.New("permission denied"),
	})
	var coded *exitcode.Error
	if !errors.As(mapped, &coded) {
		t.Fatalf("Failure(permission) did not return an *exitcode.Error: %v", mapped)
	}
	if !strings.HasPrefix(coded.Error(), "uninstall: ") {
		t.Errorf("message %q does not carry the exact uninstall: prefix", coded.Error())
	}
	for _, want := range []string{"/usr/local/bin/specscore", "elevated permissions", specscoreHomebrewUninstallCommand} {
		if !strings.Contains(coded.Error(), want) {
			t.Errorf("message %q missing %q", coded.Error(), want)
		}
	}
}

func TestUninstallErrors_FailurePermissionWithoutPath(t *testing.T) {
	mapped := (newUninstallErrors()).Failure(&selfupdate.Failure{Kind: selfupdate.KindPermission, Err: errors.New("permission denied")})
	var coded *exitcode.Error
	if !errors.As(mapped, &coded) {
		t.Fatalf("Failure(permission) did not return an *exitcode.Error: %v", mapped)
	}
	if !strings.Contains(coded.Error(), "the uninstall destination") {
		t.Errorf("message %q does not fall back to a generic destination phrase", coded.Error())
	}
}

func TestUninstallErrors_FailureNil(t *testing.T) {
	if err := (newUninstallErrors()).Failure(nil); err != nil {
		t.Fatalf("Failure(nil) = %v, want nil", err)
	}
}
