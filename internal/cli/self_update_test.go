package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/selfupdate"
)

// exitCoder is the convention the top-level CLI runner uses to translate an
// error into a process exit code.
type exitCoder interface{ ExitCode() int }

func runSelfUpdate(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := selfUpdateCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// withDetection overrides the package-level detection hook for the duration of
// the test and restores it afterward, so tests never depend on the real
// os.Executable path.
func withDetection(t *testing.T, d selfupdate.Detection) {
	t.Helper()
	prev := detectInstall
	detectInstall = func() (selfupdate.Detection, error) { return d, nil }
	t.Cleanup(func() { detectInstall = prev })
}

// withLatest overrides the package-level release-resolution hook for the
// duration of the test and restores it afterward, so --check tests never hit
// the network.
func withLatest(t *testing.T, tag string, err error) {
	t.Helper()
	prev := resolveLatest
	resolveLatest = func(context.Context) (string, error) { return tag, err }
	t.Cleanup(func() { resolveLatest = prev })
}

// withVersion overrides the build-time current-version var for the duration of
// the test so verdict comparison is deterministic regardless of how the test
// binary was built.
func withVersion(t *testing.T, v string) {
	t.Helper()
	prev := version
	version = v
	t.Cleanup(func() { version = prev })
}

// AC: cli/self-update#ac:canonical-and-alias — invoking `self-update --check`
// and `update --check` MUST resolve to the same command and produce identical
// stdout and the same error/exit result. We verify alias equivalence by
// confirming "update" is registered as an alias of the canonical command, then
// driving the canonical command with --check and asserting deterministic
// output and a nil error.
func TestSelfUpdate_CanonicalAndAlias(t *testing.T) {
	// Force a deterministic managed detection plus an up-to-date verdict so
	// --check produces stable, non-erroring stdout independent of the real
	// test-binary path and the network.
	withDetection(t, selfupdate.Detection{Method: selfupdate.Managed, Manager: selfupdate.Homebrew})
	withVersion(t, "1.2.3")
	withLatest(t, "v1.2.3", nil)

	cmd := selfUpdateCommand()
	if cmd.Name() != "self-update" {
		t.Errorf("canonical name = %q; want %q", cmd.Name(), "self-update")
	}
	if !cmd.HasAlias("update") {
		t.Errorf("command does not have alias %q; aliases=%v", "update", cmd.Aliases)
	}

	// Both invocations resolve to the same command object, so running the
	// canonical command with --check is representative of both call shapes.
	out, _, err := runSelfUpdate(t, "--check")
	if err != nil {
		t.Fatalf("self-update --check returned error: %v", err)
	}
	if out == "" {
		t.Error("expected deterministic placeholder output on stdout, got empty")
	}

	// Re-run to confirm the output is deterministic (identical across runs),
	// which is what guarantees the alias produces identical output.
	out2, _, err2 := runSelfUpdate(t, "--check")
	if err2 != nil {
		t.Fatalf("second run returned error: %v", err2)
	}
	if out != out2 {
		t.Errorf("output not deterministic: %q != %q", out, out2)
	}
}

// AC: cli/self-update#ac:managed-is-redirected — when the executable lives in a
// Homebrew/Scoop/WinGet managed location, self-update MUST print the detected
// manager and its exact upgrade command, exit 0, and leave the executable
// unchanged (no filesystem writes). We force each managed detection via the
// override hook and assert the manager name + exact command appear on stdout
// with a nil error. Returning before any download is sufficient to prove no
// write side-effect occurs.
func TestSelfUpdate_ManagedIsRedirected(t *testing.T) {
	cases := []struct {
		name        string
		manager     selfupdate.Manager
		wantName    string
		wantCommand string
	}{
		{"homebrew", selfupdate.Homebrew, "Homebrew", "brew upgrade specscore"},
		{"scoop", selfupdate.Scoop, "Scoop", "scoop update specscore"},
		{"winget", selfupdate.WinGet, "WinGet", "winget upgrade SpecScore.CLI"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			withDetection(t, selfupdate.Detection{Method: selfupdate.Managed, Manager: c.manager})

			out, _, err := runSelfUpdate(t)
			if err != nil {
				t.Fatalf("managed redirect returned error (want nil/exit 0): %v", err)
			}
			if !strings.Contains(out, c.wantName) {
				t.Errorf("stdout %q does not name detected manager %q", out, c.wantName)
			}
			if !strings.Contains(out, c.wantCommand) {
				t.Errorf("stdout %q does not contain exact upgrade command %q", out, c.wantCommand)
			}
		})
	}
}

// AC: cli/self-update#ac:ambiguous-falls-back-safe — when the install method
// cannot be confidently classified, self-update MUST NOT replace the binary. It
// states the install method is ambiguous, prints manual-update guidance, and
// exits non-zero. We force an ambiguous detection via the override hook and
// assert a non-zero, non-10 *exitcode.Error plus the required messaging.
// Returning before any download/replace is sufficient to prove no replacement
// occurred (the only filesystem-touching code lives on the manual path, which
// this branch never reaches).
func TestSelfUpdate_AmbiguousFallsBackSafe(t *testing.T) {
	withDetection(t, selfupdate.Detection{Method: selfupdate.Ambiguous, Manager: selfupdate.ManagerNone})

	out, errOut, err := runSelfUpdate(t)
	if err == nil {
		t.Fatal("expected non-nil error for ambiguous detection (must exit non-zero)")
	}

	ec, ok := err.(exitCoder)
	if !ok {
		t.Fatalf("error %T does not expose ExitCode(); want *exitcode.Error", err)
	}
	if code := ec.ExitCode(); code == 0 || code == 10 {
		t.Errorf("exit code = %d; want non-zero and not 10", code)
	}

	combined := strings.ToLower(out + errOut + err.Error())
	if !strings.Contains(combined, "ambiguous") {
		t.Errorf("output/error %q does not state the install method is ambiguous", combined)
	}
	if !strings.Contains(combined, "github.com") {
		t.Errorf("output/error %q does not contain manual-update guidance", combined)
	}
}

// Extra positional args must be rejected to keep the call shape stable.
func TestSelfUpdate_RejectsExtraArgs(t *testing.T) {
	_, _, err := runSelfUpdate(t, "extra-positional")
	if err == nil {
		t.Fatal("expected error for extra positional argument")
	}
}

// AC: cli/self-update#ac:check-is-readonly — for any install method, running
// self-update --check with a newer release available MUST print availability
// and the appropriate next step, and MUST NOT download or replace the binary.
// We force a manual detection (the only method whose action path touches the
// filesystem) plus a newer latest tag, then assert the availability line and
// the manual self-update next-step note appear. The --check branch returns
// before reaching any download/replace code, so no temp files or writes occur.
func TestSelfUpdate_CheckIsReadonly(t *testing.T) {
	withDetection(t, selfupdate.Detection{Method: selfupdate.Manual, Manager: selfupdate.ManagerNone})
	withVersion(t, "1.0.0")
	withLatest(t, "v1.1.0", nil)

	out, _, err := runSelfUpdate(t, "--check")
	if err == nil {
		t.Fatal("expected non-nil error (exit 10) when an update is available")
	}

	lower := strings.ToLower(out)
	if !strings.Contains(lower, "1.0.0") || !strings.Contains(lower, "1.1.0") {
		t.Errorf("stdout %q does not report availability (current → latest)", out)
	}
	if !strings.Contains(lower, "self-update") {
		t.Errorf("stdout %q does not name the manual self-update next step", out)
	}
}

// AC: cli/self-update#ac:check-exit-code-contract — --check exit codes MUST be
// 0 (up to date), 10 (update available), and a third distinct code for a
// release-lookup error. We drive all three scenarios via the detection and
// release-resolution hooks and assert the exact codes, with the error case
// distinct from both 0 and 10.
func TestSelfUpdate_CheckExitCodeContract(t *testing.T) {
	withDetection(t, selfupdate.Detection{Method: selfupdate.Manual, Manager: selfupdate.ManagerNone})

	t.Run("up to date → exit 0", func(t *testing.T) {
		withVersion(t, "2.0.0")
		withLatest(t, "v2.0.0", nil)

		_, _, err := runSelfUpdate(t, "--check")
		if err != nil {
			t.Fatalf("up-to-date --check returned error (want nil/exit 0): %v", err)
		}
	})

	t.Run("update available → exit 10", func(t *testing.T) {
		withVersion(t, "2.0.0")
		withLatest(t, "v2.1.0", nil)

		_, _, err := runSelfUpdate(t, "--check")
		if err == nil {
			t.Fatal("update-available --check returned nil (want exit 10)")
		}
		ec, ok := err.(exitCoder)
		if !ok {
			t.Fatalf("error %T does not expose ExitCode(); want *exitcode.Error", err)
		}
		if code := ec.ExitCode(); code != 10 {
			t.Errorf("exit code = %d; want 10", code)
		}
	})

	t.Run("release-lookup error → distinct code", func(t *testing.T) {
		withVersion(t, "2.0.0")
		withLatest(t, "", errors.New("github releases request failed"))

		_, _, err := runSelfUpdate(t, "--check")
		if err == nil {
			t.Fatal("release-lookup error --check returned nil (want non-zero)")
		}
		ec, ok := err.(exitCoder)
		if !ok {
			t.Fatalf("error %T does not expose ExitCode(); want *exitcode.Error", err)
		}
		if code := ec.ExitCode(); code == 0 || code == 10 {
			t.Errorf("exit code = %d; want distinct from 0 and 10", code)
		} else if code != 3 {
			t.Errorf("exit code = %d; want 3 (NotFound)", code)
		}
	})
}

// The --yes flag has a -y shorthand and both --check and --yes default false.
func TestSelfUpdate_Flags(t *testing.T) {
	cmd := selfUpdateCommand()
	check := cmd.Flags().Lookup("check")
	if check == nil {
		t.Fatal("missing --check flag")
	}
	if check.DefValue != "false" {
		t.Errorf("--check default = %q; want false", check.DefValue)
	}
	yes := cmd.Flags().Lookup("yes")
	if yes == nil {
		t.Fatal("missing --yes flag")
	}
	if yes.Shorthand != "y" {
		t.Errorf("--yes shorthand = %q; want y", yes.Shorthand)
	}
	if yes.DefValue != "false" {
		t.Errorf("--yes default = %q; want false", yes.DefValue)
	}
}
