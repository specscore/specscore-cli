package cli

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/strongo/selfupdate"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// exitCoder is the convention the top-level CLI runner uses to translate an
// error into a process exit code.
type exitCoder interface{ ExitCode() int }

// withVersion overrides the build-time current-version var for the duration
// of the test so verdict comparison is deterministic regardless of how the
// test binary was built.
func withVersion(t *testing.T, v string) {
	t.Helper()
	prev := version
	version = v
	t.Cleanup(func() { version = prev })
}

// --- command shape: name, alias, flag surface --------------------------

// AC: cli/self-update#ac:canonical-and-alias — the command MUST be named
// "self-update" with "update" registered as an alias resolving to the same
// command object, so both invocations share identical behavior by
// construction.
func TestSelfUpdate_CanonicalAndAlias(t *testing.T) {
	cmd := selfUpdateCommand()
	if cmd.Name() != "self-update" {
		t.Errorf("Name() = %q, want %q", cmd.Name(), "self-update")
	}
	if !cmd.HasAlias("update") {
		t.Errorf("command does not have alias %q; aliases=%v", "update", cmd.Aliases)
	}
	if cmd.Short != "Update the installed specscore binary in place" {
		t.Errorf("Short = %q; want the pre-migration text preserved", cmd.Short)
	}
}

// AC: cli/self-update#ac:canonical-and-alias — the full flag surface
// (--check, --yes/-y, --version, --allow-downgrade) MUST be registered with
// the same defaults/shorthand specscore exposed before this migration.
func TestSelfUpdate_FlagSurface(t *testing.T) {
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

	v := cmd.Flags().Lookup("version")
	if v == nil {
		t.Fatal("missing --version flag")
	}
	if v.DefValue != "" {
		t.Errorf("--version default = %q; want empty", v.DefValue)
	}

	allowDowngrade := cmd.Flags().Lookup("allow-downgrade")
	if allowDowngrade == nil {
		t.Fatal("missing --allow-downgrade flag")
	}
	if allowDowngrade.DefValue != "false" {
		t.Errorf("--allow-downgrade default = %q; want false", allowDowngrade.DefValue)
	}
}

// --format stays absent: specscore's Feature spec (req:flag-surface) does
// not include it, so JSONFormat is left false and cobracmd never registers
// a --format flag at all.
func TestSelfUpdate_NoFormatFlag(t *testing.T) {
	cmd := selfUpdateCommand()
	if f := cmd.Flags().Lookup("format"); f != nil {
		t.Errorf("--format flag registered; JSONFormat must stay false per the Feature spec")
	}
}

// --dry-run is visible and documented, not hidden: cobracmd.New always
// registers it, and specscore's Feature spec's flag surface is a floor
// (--check/--yes/--version/--allow-downgrade), not an exhaustive list — a
// flag the adapter provides for free and that costs nothing to expose stays
// on rather than being suppressed.
func TestSelfUpdate_DryRunFlagIsVisible(t *testing.T) {
	cmd := selfUpdateCommand()
	dryRun := cmd.Flags().Lookup("dry-run")
	if dryRun == nil {
		t.Fatal("missing --dry-run flag")
	}
	if dryRun.Hidden {
		t.Error("--dry-run must not be hidden: it is a documented, supported flag")
	}
	if dryRun.DefValue != "false" {
		t.Errorf("--dry-run default = %q; want false", dryRun.DefValue)
	}
}

// Extra positional args must be rejected to keep the call shape stable.
func TestSelfUpdate_RejectsExtraArgs(t *testing.T) {
	cmd := selfUpdateCommand()
	cmd.SetArgs([]string{"extra-positional"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for extra positional argument")
	}
}

// --- specscore's own Config: release identity, managers, version probe -

// AC: cli/self-update#ac:behavior-comes-from-the-library — specscore
// supplies only its own release identity, version, and undetermined
// placeholder; everything else is the library's default.
func TestSelfUpdateConfig_Identity(t *testing.T) {
	withVersion(t, "1.2.3")
	cfg := selfUpdateConfig()

	if cfg.BinaryName != "specscore" {
		t.Errorf("BinaryName = %q, want specscore", cfg.BinaryName)
	}
	if cfg.Repository != "specscore/specscore-cli" {
		t.Errorf("Repository = %q, want specscore/specscore-cli", cfg.Repository)
	}
	if cfg.CurrentVersion != "1.2.3" {
		t.Errorf("CurrentVersion = %q, want the package version var (1.2.3)", cfg.CurrentVersion)
	}
	if len(cfg.UndeterminedVersions) != 1 || cfg.UndeterminedVersions[0] != "dev" {
		t.Errorf("UndeterminedVersions = %v, want [\"dev\"]", cfg.UndeterminedVersions)
	}
	if len(cfg.VersionProbeArgs) != 1 || cfg.VersionProbeArgs[0] != "--version" {
		t.Errorf("VersionProbeArgs = %v, want [\"--version\"]", cfg.VersionProbeArgs)
	}
	// AssetName/ChecksumsName/DownloadURL are intentionally left at the
	// library's GoReleaser-shaped defaults (see .goreleaser.yml's own
	// name_template), so this Config leaves them nil/empty.
	if cfg.AssetName != nil {
		t.Error("AssetName is overridden; .goreleaser.yml already matches the library default")
	}
	if cfg.ChecksumsName != nil {
		t.Error("ChecksumsName is overridden; .goreleaser.yml already matches the library default")
	}
	if cfg.DownloadURL != nil {
		t.Error("DownloadURL is overridden; the library default already fetches a pinned release's own asset URL")
	}
}

// AC: cli/self-update#ac:managed-is-redirected — specscore MUST configure
// exactly Homebrew, Scoop, and WinGet, each with its exact upgrade command.
func TestSelfUpdateConfig_Managers(t *testing.T) {
	cfg := selfUpdateConfig()
	want := map[string]string{
		"Homebrew": "brew upgrade specscore",
		"Scoop":    "scoop update specscore",
		"WinGet":   "winget upgrade SpecScore.CLI",
	}
	if len(cfg.Managers) != len(want) {
		t.Fatalf("len(Managers) = %d, want %d", len(cfg.Managers), len(want))
	}
	seen := map[string]bool{}
	for _, m := range cfg.Managers {
		wantCmd, ok := want[m.Name]
		if !ok {
			t.Errorf("unexpected manager %q", m.Name)
			continue
		}
		if m.UpgradeCommand != wantCmd {
			t.Errorf("%s.UpgradeCommand = %q, want %q", m.Name, m.UpgradeCommand, wantCmd)
		}
		seen[m.Name] = true
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("manager %q not configured", name)
		}
	}
}

// --- selfUpdateErrors: the exit-code contract ---------------------------
//
// This table is the exact kind → code mapping specscore returned before
// this migration (see internal/cli/self_update.go's package doc for the
// derivation from the pre-migration source): permission/ambiguous/downgrade/
// non-interactive/checksum failures kept exitcode.InvalidState (4);
// release-lookup/download/unknown-tag/unsupported-platform failures kept
// exitcode.NotFound (3); anything else (KindUnexpected) gets a code (9)
// that is distinct from both 0 and the --check "update pending" code (10),
// which the pre-migration code did not always guarantee for its own
// extraction/staging failures but cli/self-update#req:exit-code-contract
// now requires explicitly.
func TestSelfUpdateErrors_FailureExitCodes(t *testing.T) {
	cases := []struct {
		name string
		kind selfupdate.FailureKind
		want int
	}{
		{"ambiguous", selfupdate.KindAmbiguous, exitcode.InvalidState},
		{"release_lookup", selfupdate.KindReleaseLookup, exitcode.NotFound},
		{"download", selfupdate.KindDownload, exitcode.NotFound},
		{"checksum", selfupdate.KindChecksum, exitcode.InvalidState},
		{"permission", selfupdate.KindPermission, exitcode.InvalidState},
		{"non_interactive", selfupdate.KindNonInteractive, exitcode.InvalidState},
		{"downgrade", selfupdate.KindDowngrade, exitcode.InvalidState},
		{"unknown_tag", selfupdate.KindUnknownTag, exitcode.NotFound},
		{"unsupported_platform", selfupdate.KindUnsupportedPlatform, exitcode.NotFound},
		{"unexpected", selfupdate.KindUnexpected, selfUpdateUnexpectedCode},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := selfUpdateErrors{}.Failure(&selfupdate.Failure{Kind: c.kind, Err: errors.New("boom")})
			ec, ok := err.(exitCoder)
			if !ok {
				t.Fatalf("error %T does not expose ExitCode()", err)
			}
			if got := ec.ExitCode(); got != c.want {
				t.Errorf("ExitCode() = %d, want %d", got, c.want)
			}
			if got := ec.ExitCode(); got == 0 || got == selfUpdateCheckPendingCode {
				t.Errorf("ExitCode() = %d must never be 0 or the --check pending code (%d)", got, selfUpdateCheckPendingCode)
			}
		})
	}
}

// A non-*selfupdate.Failure error (selfupdate.KindOf returns KindUnexpected
// for anything that isn't one) must still map to the unexpected code rather
// than panicking or losing the underlying message.
func TestSelfUpdateErrors_FailureWrapsPlainError(t *testing.T) {
	err := selfUpdateErrors{}.Failure(errors.New("not a *selfupdate.Failure"))
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

// AC: cli/self-update#req:permission-failure-clear (carried over from the
// pre-migration contract) — the permission-denied message MUST name the
// path and suggest a remedy (sudo or the package manager).
func TestSelfUpdateErrors_PermissionMessageHasPathAndRemedy(t *testing.T) {
	err := selfUpdateErrors{}.Failure(&selfupdate.Failure{
		Kind: selfupdate.KindPermission,
		Path: "/usr/local/bin/specscore",
		Err:  errors.New("chmod: permission denied"),
	})
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "/usr/local/bin/specscore") {
		t.Errorf("message %q does not name the executable path", err.Error())
	}
	if !strings.Contains(msg, "permission") {
		t.Errorf("message %q does not mention permission", err.Error())
	}
	if !strings.Contains(msg, "sudo") && !strings.Contains(msg, "package manager") {
		t.Errorf("message %q does not suggest a remedy (sudo / package manager)", err.Error())
	}
}

// A permission failure with no Path set (defensive: the library always sets
// one for KindPermission, but the mapper must not produce an empty/blank
// executable reference if it ever doesn't) falls back to a readable label.
func TestSelfUpdateErrors_PermissionMessageFallsBackWithoutPath(t *testing.T) {
	err := selfUpdateErrors{}.Failure(&selfupdate.Failure{
		Kind: selfupdate.KindPermission,
		Err:  errors.New("permission denied"),
	})
	if !strings.Contains(err.Error(), "the specscore executable") {
		t.Errorf("message %q does not fall back to a readable executable label", err.Error())
	}
}

// AC: cli/self-update#req:exit-code-contract — --check reporting anything
// other than up to date (both UpdateAvailable and Undetermined) MUST exit
// 10 with an empty message, so silentSignalErrorHandler suppresses fang's
// rendering of it (the verdict line itself is printed separately by
// cobracmd's own --check output).
func TestSelfUpdateErrors_UpdateAvailableExitsTenSilently(t *testing.T) {
	for _, v := range []selfupdate.Verdict{selfupdate.UpdateAvailable, selfupdate.Undetermined} {
		t.Run(v.String(), func(t *testing.T) {
			err := selfUpdateErrors{}.UpdateAvailable(selfupdate.CheckResult{Verdict: v})
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
		})
	}
}

// --- end-to-end wiring: cobracmd + selfupdate.Config.Check against a fake
// --- GitHub releases endpoint, exercising the full --check path including
// --- selfUpdateErrors.

// releaseServer returns an httptest.Server serving body as a GitHub releases
// listing, plus a Cleanup that closes it.
func releaseServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// withFakeReleases points selfUpdateConfigFunc at an httptest.Server for the
// duration of the test and restores it afterward, so --check integration
// tests never hit the real GitHub API.
func withFakeReleases(t *testing.T, srv *httptest.Server) {
	t.Helper()
	prev := selfUpdateConfigFunc
	selfUpdateConfigFunc = func() selfupdate.Config {
		cfg := prev()
		cfg.ReleasesAPIURL = srv.URL
		cfg.HTTPClient = srv.Client()
		return cfg
	}
	t.Cleanup(func() { selfUpdateConfigFunc = prev })
}

// AC: cli/self-update#ac:check-exit-code-contract — up to date, an
// available update, and a release-lookup failure MUST exit 0, 10, and a
// third code distinct from both, in that order, driven through the real
// command (cobracmd + selfupdate.Config.Check + selfUpdateErrors), not just
// the mapper in isolation.
func TestSelfUpdate_CheckExitCodeContract(t *testing.T) {
	t.Run("up to date exits 0", func(t *testing.T) {
		withVersion(t, "2.0.0")
		withFakeReleases(t, releaseServer(t, `[{"tag_name":"v2.0.0","prerelease":false,"draft":false}]`))

		cmd := selfUpdateCommand()
		cmd.SetArgs([]string{"--check"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("up-to-date --check returned error (want nil/exit 0): %v", err)
		}
	})

	t.Run("update available exits 10", func(t *testing.T) {
		withVersion(t, "2.0.0")
		withFakeReleases(t, releaseServer(t, `[{"tag_name":"v2.1.0","prerelease":false,"draft":false}]`))

		cmd := selfUpdateCommand()
		var out strings.Builder
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"--check"})
		err := cmd.Execute()
		if err == nil {
			t.Fatal("update-available --check returned nil (want exit 10)")
		}
		ec, ok := err.(exitCoder)
		if !ok {
			t.Fatalf("error %T does not expose ExitCode()", err)
		}
		if code := ec.ExitCode(); code != 10 {
			t.Errorf("exit code = %d; want 10", code)
		}
		if !strings.Contains(out.String(), "2.0.0") || !strings.Contains(out.String(), "2.1.0") {
			t.Errorf("stdout %q does not report current -> latest", out.String())
		}
	})

	t.Run("release-lookup error exits a code distinct from 0 and 10", func(t *testing.T) {
		withVersion(t, "2.0.0")
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "rate limited", http.StatusForbidden)
		}))
		t.Cleanup(srv.Close)
		withFakeReleases(t, srv)

		cmd := selfUpdateCommand()
		cmd.SetArgs([]string{"--check"})
		err := cmd.Execute()
		if err == nil {
			t.Fatal("release-lookup error --check returned nil (want non-zero)")
		}
		ec, ok := err.(exitCoder)
		if !ok {
			t.Fatalf("error %T does not expose ExitCode()", err)
		}
		if code := ec.ExitCode(); code == 0 || code == 10 {
			t.Errorf("exit code = %d; want distinct from 0 and 10", code)
		} else if code != exitcode.NotFound {
			t.Errorf("exit code = %d; want %d (NotFound)", code, exitcode.NotFound)
		}
	})

	t.Run("undetermined version also exits 10", func(t *testing.T) {
		withVersion(t, "dev")
		withFakeReleases(t, releaseServer(t, `[{"tag_name":"v2.1.0","prerelease":false,"draft":false}]`))

		cmd := selfUpdateCommand()
		var out strings.Builder
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"--check"})
		err := cmd.Execute()
		if err == nil {
			t.Fatal("undetermined --check returned nil (want exit 10)")
		}
		ec, ok := err.(exitCoder)
		if !ok {
			t.Fatalf("error %T does not expose ExitCode()", err)
		}
		if code := ec.ExitCode(); code != 10 {
			t.Errorf("exit code = %d; want 10", code)
		}
		if !strings.Contains(strings.ToLower(out.String()), "undetermined") {
			t.Errorf("stdout %q does not report the undetermined verdict", out.String())
		}
	})
}

// Since github.com/strongo/selfupdate v0.2.0, --check states the next step,
// not just that an update exists (cli/self-update#req:library-provided-
// behavior — this is inherited, not specscore's own logic). Detection runs
// against the real test binary's own path, which this package cannot mock
// (DetectSelf's filesystem seams are internal to the library), so this
// asserts that ONE of the three possible next-step shapes appears rather
// than pinning a specific install method: "was installed via" (Managed),
// the command path itself (Manual — cobracmd spells it via cmd.CommandPath,
// which for a standalone selfUpdateCommand() is just "self-update"), or
// "ambiguous" (Ambiguous, the expected classification for a `go test`
// temp binary, and what cli/self-update#req:exit-code-contract's own
// safe-default guarantees regardless of the test runner's layout).
func TestSelfUpdate_CheckStatesNextStep(t *testing.T) {
	withVersion(t, "1.0.0")
	withFakeReleases(t, releaseServer(t, `[{"tag_name":"v1.1.0","prerelease":false,"draft":false}]`))

	cmd := selfUpdateCommand()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--check"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("update-available --check returned nil (want exit 10)")
	}

	got := out.String()
	nextStepShapes := []string{"was installed via", "self-update", "ambiguous"}
	found := false
	for _, shape := range nextStepShapes {
		if strings.Contains(got, shape) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("stdout %q does not contain any recognized next-step guidance (%v)", got, nextStepShapes)
	}
	// Whichever shape fired, it must appear AFTER the verdict line, not
	// instead of it.
	if !strings.Contains(got, "1.0.0") || !strings.Contains(got, "1.1.0") {
		t.Errorf("stdout %q lost the current -> latest verdict line", got)
	}
}

// AC: cli/self-update#ac:canonical-and-alias — `update --check` (the alias)
// produces byte-identical output/exit-code to `self-update --check`, driven
// through the real command since both names resolve to the same *cobra.
// Command object built by selfUpdateCommand.
func TestSelfUpdate_AliasProducesIdenticalOutput(t *testing.T) {
	withVersion(t, "1.5.0")
	withFakeReleases(t, releaseServer(t, `[{"tag_name":"v1.5.0","prerelease":false,"draft":false}]`))

	run := func() string {
		cmd := selfUpdateCommand()
		var out strings.Builder
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"--check"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("--check returned error: %v", err)
		}
		return out.String()
	}

	canonical := run()
	viaAlias := run()
	if canonical == "" {
		t.Fatal("expected non-empty deterministic --check output")
	}
	if canonical != viaAlias {
		t.Errorf("output not deterministic across invocations: %q != %q", canonical, viaAlias)
	}
}

// --check's JSON encode path is never reachable from specscore (JSONFormat
// is false, so cobracmd never registers --format and always takes the text
// branch) — this is a sanity check that the flag really is absent, keeping
// the "JSONFormat stays false" contract honest against cobracmd's own
// behavior rather than just this package's intent.
func TestSelfUpdate_NoFormatFlagMeansNoJSONOutput(t *testing.T) {
	withVersion(t, "1.0.0")
	withFakeReleases(t, releaseServer(t, `[{"tag_name":"v1.0.0","prerelease":false,"draft":false}]`))

	cmd := selfUpdateCommand()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--check"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("--check returned error: %v", err)
	}
	var probe json.RawMessage
	if err := json.Unmarshal([]byte(out.String()), &probe); err == nil {
		t.Errorf("stdout %q parses as JSON; --check must always be text (no --format flag exists)", out.String())
	}
}
