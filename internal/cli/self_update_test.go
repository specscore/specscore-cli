package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
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

// AC: cli/self-update#ac:already-current-noop — a manual install already on the
// latest stable release that runs `specscore self-update` (without --check) MUST
// report it is up to date and exit 0 without downloading or replacing anything.
// We force a manual detection plus a latest tag equal to the running version,
// then assert a nil error (exit 0) and an up-to-date message on stdout. Returning
// before the self-replace placeholder is sufficient proof that no download or
// replacement was attempted.
func TestSelfUpdate_AlreadyCurrentNoop(t *testing.T) {
	withDetection(t, selfupdate.Detection{Method: selfupdate.Manual, Manager: selfupdate.ManagerNone})
	withVersion(t, "1.2.3")
	withLatest(t, "v1.2.3", nil)

	out, _, err := runSelfUpdate(t)
	if err != nil {
		t.Fatalf("already-current self-update returned error (want nil/exit 0): %v", err)
	}
	if !strings.Contains(strings.ToLower(out), "up to date") {
		t.Errorf("stdout %q does not report the binary is up to date", out)
	}
}

// withInteractive overrides the package-level TTY-detection hook for the
// duration of the test and restores it afterward, so confirmation tests never
// depend on whether the test process is attached to a real terminal.
func withInteractive(t *testing.T, interactive bool) {
	t.Helper()
	prev := isInteractive
	isInteractive = func() bool { return interactive }
	t.Cleanup(func() { isInteractive = prev })
}

// withSelfReplace overrides the package-level self-replace hook with a spy so
// confirmation tests never download or replace anything. It returns a pointer
// to a bool that records whether the hook was invoked.
func withSelfReplace(t *testing.T, err error) *bool {
	t.Helper()
	called := false
	prev := doSelfReplace
	doSelfReplace = func(context.Context, string) error {
		called = true
		return err
	}
	t.Cleanup(func() { doSelfReplace = prev })
	return &called
}

// AC: cli/self-update#ac:confirm-prompt-and-yes — a manual install with a newer
// release available, run with --yes, MUST perform the replacement without
// prompting, after printing the current → latest transition. We stub a manual
// detection plus a newer latest and override the self-replace hook with a spy
// so no real download/replace happens.
func TestSelfUpdate_ConfirmPromptAndYes_WithYes(t *testing.T) {
	withDetection(t, selfupdate.Detection{Method: selfupdate.Manual, Manager: selfupdate.ManagerNone})
	withVersion(t, "1.0.0")
	withLatest(t, "v1.1.0", nil)
	withInteractive(t, false) // --yes must work regardless of TTY state
	called := withSelfReplace(t, nil)

	out, _, err := runSelfUpdate(t, "--yes")
	if err != nil {
		t.Fatalf("self-update --yes returned error (want nil): %v", err)
	}
	if !*called {
		t.Error("doSelfReplace was not called with --yes")
	}
	if !strings.Contains(out, "→") || !strings.Contains(out, "1.0.0") || !strings.Contains(out, "1.1.0") {
		t.Errorf("stdout %q does not contain the current → latest transition", out)
	}
}

// AC: cli/self-update#ac:confirm-prompt-and-yes — without --yes but attached to
// an interactive terminal, the command MUST print the transition, prompt for
// confirmation, and (on "y") proceed with the replacement. We drive stdin with
// "y\n" and assert the prompt text appears and the self-replace spy ran.
func TestSelfUpdate_ConfirmPromptAndYes_InteractiveConfirms(t *testing.T) {
	withDetection(t, selfupdate.Detection{Method: selfupdate.Manual, Manager: selfupdate.ManagerNone})
	withVersion(t, "1.0.0")
	withLatest(t, "v1.1.0", nil)
	withInteractive(t, true)
	called := withSelfReplace(t, nil)

	cmd := selfUpdateCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader("y\n"))
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("interactive confirm returned error (want nil): %v", err)
	}
	if !*called {
		t.Error("doSelfReplace was not called after interactive confirmation")
	}
	lower := strings.ToLower(out.String())
	if !strings.Contains(lower, "proceed") {
		t.Errorf("stdout %q does not contain a confirmation prompt", out.String())
	}
	if !strings.Contains(out.String(), "→") {
		t.Errorf("stdout %q does not contain the current → latest transition", out.String())
	}
}

// AC: cli/self-update#ac:noninteractive-without-yes-refuses — a manual install
// with a newer release, run without --yes and without an interactive terminal,
// MUST refuse to replace, state that --yes is required for non-interactive use,
// and exit non-zero, leaving the binary unchanged. We force isInteractive→false
// and assert the self-replace spy was NOT called plus a non-zero *exitcode.Error.
func TestSelfUpdate_NonInteractiveWithoutYesRefuses(t *testing.T) {
	withDetection(t, selfupdate.Detection{Method: selfupdate.Manual, Manager: selfupdate.ManagerNone})
	withVersion(t, "1.0.0")
	withLatest(t, "v1.1.0", nil)
	withInteractive(t, false)
	called := withSelfReplace(t, nil)

	out, errOut, err := runSelfUpdate(t)
	if err == nil {
		t.Fatal("expected non-nil error for non-interactive run without --yes")
	}
	if *called {
		t.Error("doSelfReplace was called; binary must be left unchanged")
	}
	ec, ok := err.(exitCoder)
	if !ok {
		t.Fatalf("error %T does not expose ExitCode(); want *exitcode.Error", err)
	}
	if code := ec.ExitCode(); code == 0 {
		t.Errorf("exit code = %d; want non-zero", code)
	}
	combined := strings.ToLower(out + errOut + err.Error())
	if !strings.Contains(combined, "--yes") {
		t.Errorf("output/error %q does not mention that --yes is required", combined)
	}
	if !strings.Contains(combined, "non-interactive") && !strings.Contains(combined, "noninteractive") {
		t.Errorf("output/error %q does not mention non-interactive use", combined)
	}
}

// AC: cli/self-update#ac:network-failure-is-safe — a manual install with a
// newer release available, run with --yes, where the release source is
// unreachable MUST print a clear error, exit non-zero, and NOT modify the
// binary. We model the unreachable source two ways: (1) resolveLatest failing
// the lookup, and (2) doSelfReplace failing mid-download with a non-permission
// error. Both must surface a clear, actionable error and a non-zero
// *exitcode.Error.
func TestSelfUpdate_NetworkFailureIsSafe(t *testing.T) {
	t.Run("release lookup fails", func(t *testing.T) {
		withDetection(t, selfupdate.Detection{Method: selfupdate.Manual, Manager: selfupdate.ManagerNone})
		withVersion(t, "1.0.0")
		withLatest(t, "", errors.New("dial tcp: connection refused"))
		called := withSelfReplace(t, nil)

		out, errOut, err := runSelfUpdate(t, "--yes")
		if err == nil {
			t.Fatal("expected non-nil error when the release source is unreachable")
		}
		if *called {
			t.Error("doSelfReplace was called; the binary must be left unchanged on a lookup failure")
		}
		ec, ok := err.(exitCoder)
		if !ok {
			t.Fatalf("error %T does not expose ExitCode(); want *exitcode.Error", err)
		}
		if code := ec.ExitCode(); code == 0 {
			t.Errorf("exit code = %d; want non-zero", code)
		}
		combined := strings.ToLower(out + errOut + err.Error())
		if !strings.Contains(combined, "release") {
			t.Errorf("output/error %q does not mention the release-lookup failure", combined)
		}
	})

	t.Run("download fails", func(t *testing.T) {
		withDetection(t, selfupdate.Detection{Method: selfupdate.Manual, Manager: selfupdate.ManagerNone})
		withVersion(t, "1.0.0")
		withLatest(t, "v1.1.0", nil)
		withInteractive(t, false)
		// A network-ish, non-permission error from the download/verify step.
		_ = withSelfReplace(t, errors.New("dial tcp: connection refused"))

		out, errOut, err := runSelfUpdate(t, "--yes")
		if err == nil {
			t.Fatal("expected non-nil error when the asset download fails")
		}
		ec, ok := err.(exitCoder)
		if !ok {
			t.Fatalf("error %T does not expose ExitCode(); want *exitcode.Error", err)
		}
		if code := ec.ExitCode(); code == 0 {
			t.Errorf("exit code = %d; want non-zero", code)
		}
		combined := strings.ToLower(out + errOut + err.Error())
		if !strings.Contains(combined, "download") && !strings.Contains(combined, "release") {
			t.Errorf("output/error %q does not mention the download/release failure", combined)
		}
		// The happy-path "updated to" line must NOT appear: nothing was replaced.
		if strings.Contains(strings.ToLower(out), "updated to") {
			t.Errorf("stdout %q claims an update succeeded after a download failure", out)
		}
	})
}

// AC: cli/self-update#ac:permission-denied-is-safe — a manual install whose
// executable directory is not writable, run with --yes, MUST report the
// permission failure with the executable path and a suggested remedy, exit
// non-zero, and leave the original binary intact. We override doSelfReplace to
// return an fs.ErrPermission-wrapped error and assert the message includes a
// path and a remedy hint.
func TestSelfUpdate_PermissionDeniedIsSafe(t *testing.T) {
	withDetection(t, selfupdate.Detection{Method: selfupdate.Manual, Manager: selfupdate.ManagerNone})
	withVersion(t, "1.0.0")
	withLatest(t, "v1.1.0", nil)
	withInteractive(t, false)
	_ = withSelfReplace(t, fmt.Errorf("rename: %w", fs.ErrPermission))

	out, errOut, err := runSelfUpdate(t, "--yes")
	if err == nil {
		t.Fatal("expected non-nil error on a permission-denied replacement")
	}
	ec, ok := err.(exitCoder)
	if !ok {
		t.Fatalf("error %T does not expose ExitCode(); want *exitcode.Error", err)
	}
	if code := ec.ExitCode(); code == 0 {
		t.Errorf("exit code = %d; want non-zero", code)
	}
	combined := strings.ToLower(out + errOut + err.Error())
	if !strings.Contains(combined, "permission") {
		t.Errorf("output/error %q does not report a permission failure", combined)
	}
	// A remedy hint: elevated permissions (sudo) or the package manager.
	if !strings.Contains(combined, "sudo") && !strings.Contains(combined, "package manager") {
		t.Errorf("output/error %q does not suggest a remedy (sudo / package manager)", combined)
	}
	// The error must reference a path. os.Executable() should resolve in the
	// test process; assert a path separator is present as a proxy.
	if !strings.Contains(combined, "/") && !strings.Contains(combined, `\`) {
		t.Errorf("output/error %q does not include the executable path", combined)
	}
	if strings.Contains(strings.ToLower(out), "updated to") {
		t.Errorf("stdout %q claims an update succeeded after a permission failure", out)
	}
}

// withSelfReplaceTag overrides the package-level self-replace hook with a spy
// that captures the tag it was called with, so pinned-version tests can assert
// the exact target routed to the swap machinery without any download/replace.
func withSelfReplaceTag(t *testing.T, err error) *string {
	t.Helper()
	var gotTag string
	prev := doSelfReplace
	doSelfReplace = func(_ context.Context, tag string) error {
		gotTag = tag
		return err
	}
	t.Cleanup(func() { doSelfReplace = prev })
	return &gotTag
}

// AC: cli/self-update#ac:version-flag-selects-tag — a manual install run with
// `--version 0.0.3 --yes` MUST install exactly the v0.0.3 release (tag accepted
// with or without the leading v), routed through the same swap machinery as an
// unpinned update, and MUST NOT consult the stable-only latest resolver for the
// target. We stub a different "latest" tag as a sentinel: if resolveLatest were
// used for the target, the spy would capture the sentinel instead of the pin.
func TestSelfUpdate_VersionFlagSelectsTag(t *testing.T) {
	withDetection(t, selfupdate.Detection{Method: selfupdate.Manual, Manager: selfupdate.ManagerNone})
	withVersion(t, "0.0.1")
	withLatest(t, "v9.9.9", nil) // sentinel: must NOT become the install target
	withInteractive(t, false)    // --yes must work regardless of TTY
	gotTag := withSelfReplaceTag(t, nil)

	out, _, err := runSelfUpdate(t, "--version", "0.0.3", "--yes")
	if err != nil {
		t.Fatalf("pinned self-update returned error (want nil): %v", err)
	}
	if *gotTag != "0.0.3" {
		t.Errorf("doSelfReplace tag = %q; want pinned %q (not the sentinel latest)", *gotTag, "0.0.3")
	}
	if !strings.Contains(out, "→") || !strings.Contains(out, "0.0.1") || !strings.Contains(out, "0.0.3") {
		t.Errorf("stdout %q does not contain the current → pinned transition", out)
	}
	if strings.Contains(out, "9.9.9") {
		t.Errorf("stdout %q references the sentinel latest; pinned path must bypass resolveLatest", out)
	}
}

// AC: cli/self-update#ac:pinned-tag-allows-prerelease — a manual install run
// with `--version v0.1.0-rc.1 --yes` MUST install that prerelease exactly, even
// though the unpinned latest path filters prereleases out. We assert the spy
// captured the prerelease tag verbatim and the sentinel "latest" was not used.
func TestSelfUpdate_PinnedTagAllowsPrerelease(t *testing.T) {
	withDetection(t, selfupdate.Detection{Method: selfupdate.Manual, Manager: selfupdate.ManagerNone})
	withVersion(t, "0.0.1")
	withLatest(t, "v0.0.2", nil) // sentinel stable latest; prerelease would be skipped here
	withInteractive(t, false)
	gotTag := withSelfReplaceTag(t, nil)

	out, _, err := runSelfUpdate(t, "--version", "v0.1.0-rc.1", "--yes")
	if err != nil {
		t.Fatalf("pinned prerelease self-update returned error (want nil): %v", err)
	}
	if *gotTag != "v0.1.0-rc.1" {
		t.Errorf("doSelfReplace tag = %q; want pinned prerelease %q", *gotTag, "v0.1.0-rc.1")
	}
	if !strings.Contains(out, "→") || !strings.Contains(out, "v0.1.0-rc.1") {
		t.Errorf("stdout %q does not contain the current → pinned-prerelease transition", out)
	}
}

// The --version flag exists as a self-update-local string flag and defaults to
// empty (distinct from the root --version).
func TestSelfUpdate_VersionFlag(t *testing.T) {
	cmd := selfUpdateCommand()
	v := cmd.Flags().Lookup("version")
	if v == nil {
		t.Fatal("missing --version flag")
	}
	if v.DefValue != "" {
		t.Errorf("--version default = %q; want empty", v.DefValue)
	}
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
