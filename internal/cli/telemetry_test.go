package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// withTempHomeForCLI is the cli-package analogue of telemetry/withTempHome.
// Reroutes HOME so the telemetry subcommand reads/writes a scratch path.
func withTempHomeForCLI(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	// Clear opt-out env vars so tests start from a known state.
	t.Setenv("SPECSCORE_TELEMETRY", "")
	t.Setenv("DO_NOT_TRACK", "")
	t.Setenv("CI", "")
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("GITLAB_CI", "")
	t.Setenv("BUILDKITE", "")
	t.Setenv("CIRCLECI", "")
	return dir
}

func TestTelemetryStatus_DefaultsReportEnabled(t *testing.T) {
	withTempHomeForCLI(t)
	var buf bytes.Buffer
	if err := writeStatus(&buf, "", false); err != nil {
		t.Fatalf("writeStatus: %v", err)
	}
	out := buf.String()
	// Both channels' init() ran (usage.go and errors.go).
	if !strings.Contains(out, "usage-stats: enabled") {
		t.Errorf("expected usage-stats: enabled line, got %q", out)
	}
	if !strings.Contains(out, "crash-reports: enabled") {
		t.Errorf("expected crash-reports: enabled line, got %q", out)
	}
	if !strings.Contains(out, "(source: default)") {
		t.Errorf("expected source: default, got %q", out)
	}
}

func TestTelemetryStatus_SingleChannel(t *testing.T) {
	withTempHomeForCLI(t)
	var buf bytes.Buffer
	if err := writeStatus(&buf, "usage-stats", true); err != nil {
		t.Fatalf("writeStatus: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "usage-stats:") {
		t.Errorf("expected line to start with usage-stats:, got %q", out)
	}
	if strings.Contains(out, "crash-reports") {
		t.Errorf("expected ONLY usage-stats row, got %q", out)
	}
}

func TestValidateChannelArg_AllSentinelMeansAllChannels(t *testing.T) {
	ch, hasArg, err := validateChannelArg([]string{"all"})
	if err != nil {
		t.Fatalf("validateChannelArg(all): unexpected error %v", err)
	}
	if hasArg {
		t.Errorf("`all` should be treated as no-arg (hasArg=false), got hasArg=true with channel=%q", ch)
	}
	if ch != "" {
		t.Errorf("`all` should not yield a real channel name, got %q", ch)
	}
}

func TestValidateChannelArg_StarIsRejected(t *testing.T) {
	// `*` is no longer the sentinel; it should fall through to the
	// unknown-channel error path (exit 2). This guards against
	// inadvertent re-introduction of `*`.
	_, _, err := validateChannelArg([]string{"*"})
	if err == nil {
		t.Fatalf("expected `*` to be rejected as unknown channel")
	}
}

func TestTelemetryDisable_AllSentinel_EquivalentToNoArg(t *testing.T) {
	withTempHomeForCLI(t)
	var buf bytes.Buffer
	// First, disable with no-arg.
	if err := mutateState(&buf, "", false, false); err != nil {
		t.Fatalf("disable no-arg: %v", err)
	}
	want := buf.String()
	buf.Reset()
	// Reset state and disable with `all`.
	withTempHomeForCLI(t)
	single, hasArg, err := validateChannelArg([]string{"all"})
	if err != nil {
		t.Fatalf("validate all: %v", err)
	}
	if err := mutateState(&buf, single, hasArg, false); err != nil {
		t.Fatalf("disable all: %v", err)
	}
	if buf.String() != want {
		t.Errorf("disable all confirmation should match no-arg: got %q want %q", buf.String(), want)
	}
}

func TestValidateChannelArg_UnknownExits2(t *testing.T) {
	_, _, err := validateChannelArg([]string{"unknown-typo"})
	if err == nil {
		t.Fatalf("expected error for unknown channel")
	}
	type exitCoder interface{ ExitCode() int }
	ec, ok := err.(exitCoder)
	if !ok {
		t.Fatalf("error does not implement exitCoder: %T", err)
	}
	if ec.ExitCode() != 2 {
		t.Errorf("exit code: got %d, want 2", ec.ExitCode())
	}
	if !strings.Contains(err.Error(), "known channels:") {
		t.Errorf("error should list known channels, got %q", err.Error())
	}
}

func TestTelemetryEnableDisable_RoundTrip(t *testing.T) {
	home := withTempHomeForCLI(t)

	// Disable globally.
	var buf bytes.Buffer
	if err := mutateState(&buf, "", false, false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if !strings.Contains(buf.String(), "all channels disabled") {
		t.Errorf("disable confirmation: got %q", buf.String())
	}
	// File should exist.
	path := filepath.Join(home, ".specscore", "telemetry.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read telemetry.yaml: %v", err)
	}
	if !strings.Contains(string(raw), "enabled: false") {
		t.Errorf("file should contain enabled: false, got %q", string(raw))
	}

	// Status should report both disabled.
	buf.Reset()
	if err := writeStatus(&buf, "", false); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(buf.String(), "usage-stats: disabled") {
		t.Errorf("status should report usage-stats disabled, got %q", buf.String())
	}

	// Re-enable just crash-reports (per-channel override).
	buf.Reset()
	if err := mutateState(&buf, "crash-reports", true, true); err != nil {
		t.Fatalf("enable crash-reports: %v", err)
	}
	if !strings.Contains(buf.String(), "crash-reports enabled") {
		t.Errorf("enable crash-reports confirmation: got %q", buf.String())
	}
	// Status reflects per-channel override beating global.
	buf.Reset()
	if err := writeStatus(&buf, "", false); err != nil {
		t.Fatalf("status: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "usage-stats: disabled") {
		t.Errorf("usage-stats should still be disabled: %q", out)
	}
	if !strings.Contains(out, "crash-reports: enabled") {
		t.Errorf("crash-reports should be enabled (per-channel override): %q", out)
	}
}

func TestPreRun_FirstRunNoticeShownOnce(t *testing.T) {
	home := withTempHomeForCLI(t)
	// Reset module state.
	invocation = runtimeState{}
	noTelemetryFlag = false
	var notice bytes.Buffer
	prevWriter := firstRunNoticeWriter
	firstRunNoticeWriter = &notice
	t.Cleanup(func() { firstRunNoticeWriter = prevWriter })

	cmd := newRootCommandForTest()
	preRun(cmd)
	first := notice.String()
	// Required literal strings per REQ:first-run-notice-content.
	for _, want := range []string{
		"SpecScore",
		"usage-stats",
		"crash-reports",
		"specscore telemetry disable [channel-id]",
		"all",
	} {
		if !strings.Contains(first, want) {
			t.Errorf("first-run notice missing required literal %q; got:\n%s", want, first)
		}
	}

	// Verify install_id was created.
	if _, err := os.Stat(filepath.Join(home, ".specscore", "install_id")); err != nil {
		t.Errorf("install_id should exist after preRun: %v", err)
	}

	// Second invocation: notice should NOT print again.
	notice.Reset()
	invocation = runtimeState{}
	preRun(cmd)
	if notice.Len() > 0 {
		t.Errorf("second preRun should not print first-run notice, got %q", notice.String())
	}
}

func TestPreRun_FirstRunNoticeSuppressedInCI(t *testing.T) {
	home := withTempHomeForCLI(t)
	t.Setenv("CI", "true")
	invocation = runtimeState{}
	noTelemetryFlag = false
	var notice bytes.Buffer
	prevWriter := firstRunNoticeWriter
	firstRunNoticeWriter = &notice
	t.Cleanup(func() { firstRunNoticeWriter = prevWriter })

	cmd := newRootCommandForTest()
	preRun(cmd)
	if notice.Len() > 0 {
		t.Errorf("CI run should NOT print first-run notice, got %q", notice.String())
	}
	// But install_id MUST still be created (so a later interactive run on the
	// same machine doesn't re-trigger).
	if _, err := os.Stat(filepath.Join(home, ".specscore", "install_id")); err != nil {
		t.Errorf("install_id should still be created in CI: %v", err)
	}
}

// newRootCommandForTest constructs a minimal cobra command tree sufficient for
// driving preRun in tests. We deliberately don't go through fang.Execute.
func newRootCommandForTest() *cobra.Command {
	return &cobra.Command{Use: "specscore"}
}

// versionCmdForTest builds a minimal stand-in for the real `version`
// subcommand fangcmd.Wire registers (Use "version", a bool --json flag),
// with --json set to asJSON. Good enough to drive isVersionJSONProbe and
// preRun without pulling in the real buildinfo/cobracmd.VersionCommand.
func versionCmdForTest(t *testing.T, asJSON bool) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "version"}
	cmd.Flags().Bool("json", false, "")
	if asJSON {
		if err := cmd.Flags().Set("json", "true"); err != nil {
			t.Fatalf("Flags().Set(json, true): %v", err)
		}
	}
	return cmd
}

// cli-install#req:version-json-side-effect-free, cli/version#req:json-output
func TestIsVersionJSONProbe(t *testing.T) {
	if !isVersionJSONProbe(versionCmdForTest(t, true)) {
		t.Error("expected true for `version --json`")
	}
	if isVersionJSONProbe(versionCmdForTest(t, false)) {
		t.Error("expected false for plain `version` (no --json)")
	}
	if isVersionJSONProbe(&cobra.Command{Use: "install"}) {
		t.Error("expected false for a non-version command")
	}
	noFlagVersion := &cobra.Command{Use: "version"}
	if isVersionJSONProbe(noFlagVersion) {
		t.Error("expected false when the command carries no --json flag at all")
	}
	if isVersionJSONProbe(nil) {
		t.Error("expected false for a nil command")
	}
}

// AC: version-json-is-uniform-and-quiet — `version --json` must perform no
// file write (no install_id creation, no first-run notice) and must leave
// invocation.StartTime zero, so emitInvocationEvent's own "PreRun never
// fired" guard skips telemetry emission entirely
// (cli-install#req:version-json-side-effect-free).
func TestPreRun_VersionJSONProbeSkipsAllSideEffects(t *testing.T) {
	home := withTempHomeForCLI(t)
	invocation = runtimeState{}
	noTelemetryFlag = false
	var notice bytes.Buffer
	prevWriter := firstRunNoticeWriter
	firstRunNoticeWriter = &notice
	t.Cleanup(func() { firstRunNoticeWriter = prevWriter })

	preRun(versionCmdForTest(t, true))

	if !invocation.StartTime.IsZero() {
		t.Error("invocation.StartTime must stay zero for a version --json probe")
	}
	if notice.Len() > 0 {
		t.Errorf("first-run notice must not print for version --json, got %q", notice.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".specscore", "install_id")); err == nil {
		t.Error("install_id must not be created by a version --json probe")
	}

	// The full chain, not just the StartTime guard in isolation: no channel
	// transmission is attempted either.
	emitInvocationEvent(nil)
}

// AC: version-json-is-uniform-and-quiet — driven through the real,
// production command tree (Run, not a hand-built stand-in), `specscore
// version --json` creates no install_id file.
func TestRun_VersionJSON_CreatesNoInstallID(t *testing.T) {
	home := withTempHomeForCLI(t)
	invocation = runtimeState{}

	if err := Run([]string{"specscore", "version", "--json"}); err != nil {
		t.Fatalf("Run([version --json]) = %v, want nil", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".specscore", "install_id")); err == nil {
		t.Error("install_id must not be created by `specscore version --json`")
	}
}

// Contrast: plain `specscore version` (no --json) is an ordinary command and
// DOES create install_id like any other invocation — proving the exemption
// above is scoped to --json, not to the version subcommand as a whole.
func TestRun_PlainVersion_StillCreatesInstallID(t *testing.T) {
	home := withTempHomeForCLI(t)
	invocation = runtimeState{}

	if err := Run([]string{"specscore", "version"}); err != nil {
		t.Fatalf("Run([version]) = %v, want nil", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".specscore", "install_id")); err != nil {
		t.Error("install_id SHOULD be created by a plain `specscore version` invocation")
	}
}

// ---------------------------------------------------------------------------
// Cobra-level tests for telemetry subcommands (status, enable, disable).
// These exercise the command constructors via cmd.Execute(), covering the
// wiring that unit tests of writeStatus/mutateState/validateChannelArg miss.
// ---------------------------------------------------------------------------

// runTelemetry builds the telemetry cobra command tree and executes it with
// the given args (e.g. "status", "enable", "disable usage-stats").
func runTelemetry(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := telemetryCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// --- telemetry status -------------------------------------------------------

func TestTelemetry_Status(t *testing.T) {
	withTempHomeForCLI(t)
	out, _, err := runTelemetry(t, "status")
	if err != nil {
		t.Fatalf("telemetry status: unexpected error: %v", err)
	}
	if !strings.Contains(out, "usage-stats:") {
		t.Errorf("expected usage-stats line in output, got %q", out)
	}
	if !strings.Contains(out, "crash-reports:") {
		t.Errorf("expected crash-reports line in output, got %q", out)
	}
}

func TestTelemetry_Status_SingleChannel(t *testing.T) {
	withTempHomeForCLI(t)
	out, _, err := runTelemetry(t, "status", "usage-stats")
	if err != nil {
		t.Fatalf("telemetry status usage-stats: unexpected error: %v", err)
	}
	if !strings.Contains(out, "usage-stats:") {
		t.Errorf("expected usage-stats line, got %q", out)
	}
	if strings.Contains(out, "crash-reports:") {
		t.Errorf("should only show usage-stats, got %q", out)
	}
}

func TestTelemetry_Status_InvalidChannel(t *testing.T) {
	withTempHomeForCLI(t)
	_, _, err := runTelemetry(t, "status", "banana")
	if err == nil {
		t.Fatal("expected error for invalid channel, got nil")
	}
	if !strings.Contains(err.Error(), "unknown channel") {
		t.Errorf("error should mention unknown channel, got %q", err.Error())
	}
}

// --- telemetry enable -------------------------------------------------------

func TestTelemetry_Enable(t *testing.T) {
	withTempHomeForCLI(t)
	out, _, err := runTelemetry(t, "enable")
	if err != nil {
		t.Fatalf("telemetry enable: unexpected error: %v", err)
	}
	if !strings.Contains(out, "all channels enabled") {
		t.Errorf("expected confirmation of all channels enabled, got %q", out)
	}
}

func TestTelemetry_Enable_SpecificChannel(t *testing.T) {
	withTempHomeForCLI(t)
	out, _, err := runTelemetry(t, "enable", "usage-stats")
	if err != nil {
		t.Fatalf("telemetry enable usage-stats: unexpected error: %v", err)
	}
	if !strings.Contains(out, "usage-stats enabled") {
		t.Errorf("expected confirmation of usage-stats enabled, got %q", out)
	}
}

func TestTelemetry_Enable_InvalidChannel(t *testing.T) {
	withTempHomeForCLI(t)
	_, _, err := runTelemetry(t, "enable", "banana")
	if err == nil {
		t.Fatal("expected error for invalid channel, got nil")
	}
	if !strings.Contains(err.Error(), "unknown channel") {
		t.Errorf("error should mention unknown channel, got %q", err.Error())
	}
}

// --- telemetry disable ------------------------------------------------------

func TestTelemetry_Disable(t *testing.T) {
	withTempHomeForCLI(t)
	out, _, err := runTelemetry(t, "disable")
	if err != nil {
		t.Fatalf("telemetry disable: unexpected error: %v", err)
	}
	if !strings.Contains(out, "all channels disabled") {
		t.Errorf("expected confirmation of all channels disabled, got %q", out)
	}
}

func TestTelemetry_Disable_SpecificChannel(t *testing.T) {
	withTempHomeForCLI(t)
	out, _, err := runTelemetry(t, "disable", "crash-reports")
	if err != nil {
		t.Fatalf("telemetry disable crash-reports: unexpected error: %v", err)
	}
	if !strings.Contains(out, "crash-reports disabled") {
		t.Errorf("expected confirmation of crash-reports disabled, got %q", out)
	}
}

func TestTelemetry_Disable_InvalidChannel(t *testing.T) {
	withTempHomeForCLI(t)
	_, _, err := runTelemetry(t, "disable", "banana")
	if err == nil {
		t.Fatal("expected error for invalid channel, got nil")
	}
	if !strings.Contains(err.Error(), "unknown channel") {
		t.Errorf("error should mention unknown channel, got %q", err.Error())
	}
}

// --- telemetry (bare) prints help -------------------------------------------

func TestTelemetry_BareCommand_PrintsHelp(t *testing.T) {
	withTempHomeForCLI(t)
	out, _, err := runTelemetry(t)
	if err != nil {
		t.Fatalf("bare telemetry command: unexpected error: %v", err)
	}
	if !strings.Contains(out, "status") || !strings.Contains(out, "enable") || !strings.Contains(out, "disable") {
		t.Errorf("expected help text listing subcommands, got %q", out)
	}
}

// --- telemetry enable/disable round-trip via cobra --------------------------

func TestTelemetry_EnableDisable_RoundTrip_ViaCobra(t *testing.T) {
	withTempHomeForCLI(t)

	// Disable all channels.
	out, _, err := runTelemetry(t, "disable")
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	if !strings.Contains(out, "all channels disabled") {
		t.Errorf("disable confirmation: got %q", out)
	}

	// Status should report disabled.
	out, _, err = runTelemetry(t, "status")
	if err != nil {
		t.Fatalf("status after disable: %v", err)
	}
	if !strings.Contains(out, "usage-stats: disabled") {
		t.Errorf("expected usage-stats disabled, got %q", out)
	}
	if !strings.Contains(out, "crash-reports: disabled") {
		t.Errorf("expected crash-reports disabled, got %q", out)
	}

	// Re-enable usage-stats only.
	out, _, err = runTelemetry(t, "enable", "usage-stats")
	if err != nil {
		t.Fatalf("enable usage-stats: %v", err)
	}
	if !strings.Contains(out, "usage-stats enabled") {
		t.Errorf("enable confirmation: got %q", out)
	}

	// Status: usage-stats enabled, crash-reports still disabled.
	out, _, err = runTelemetry(t, "status")
	if err != nil {
		t.Fatalf("status after partial enable: %v", err)
	}
	if !strings.Contains(out, "usage-stats: enabled") {
		t.Errorf("expected usage-stats enabled, got %q", out)
	}
	if !strings.Contains(out, "crash-reports: disabled") {
		t.Errorf("expected crash-reports still disabled, got %q", out)
	}
}
