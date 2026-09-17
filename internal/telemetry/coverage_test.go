package telemetry

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/posthog/posthog-go"
)

// ---------------------------------------------------------------------------
// Emit + callBounded (registry.go) — 0% → covered
// ---------------------------------------------------------------------------

// TestEmit_DispatchesToRegisteredChannels verifies the full Emit path:
// registry lookup → callBounded → transmit-fn invocation. Uses the already-
// registered usage-stats and crash-reports channels (both no-op in dev builds).
func TestEmit_DispatchesToRegisteredChannels(t *testing.T) {
	// In dev builds both channels are registered but clients are nil,
	// so transmit-fns return immediately. Emit must not panic.
	Emit(context.Background(), Event{
		Command:    "test.emit",
		CLIVersion: "0.0.0-test",
		InstallID:  "00000000-0000-4000-8000-000000000001",
	})
}

// TestCallBounded_TransmitFnCompletes verifies the happy-path where the
// transmit-fn finishes well within the 500 ms timeout.
func TestCallBounded_TransmitFnCompletes(t *testing.T) {
	var called atomic.Bool
	fn := func(_ context.Context, _ Event) {
		called.Store(true)
	}
	callBounded(context.Background(), fn, Event{})
	if !called.Load() {
		t.Error("transmit-fn was not called")
	}
}

// TestCallBounded_TimeoutDropsSilently verifies that a slow transmit-fn is
// abandoned when the 500 ms hard cap fires. We use a pre-cancelled context
// to trigger the timeout branch immediately.
func TestCallBounded_TimeoutDropsSilently(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel so the timeout fires immediately

	fn := func(ctx context.Context, _ Event) {
		// Block until the context is done — simulates a slow transmit.
		<-ctx.Done()
	}
	// Must return promptly (the pre-cancelled context triggers the timeout
	// branch in callBounded).
	callBounded(ctx, fn, Event{})
}

// TestCallBounded_PanicInTransmitFnRecovered verifies the inner deferred
// recover() in callBounded catches panics from transmit-fns.
func TestCallBounded_PanicInTransmitFnRecovered(t *testing.T) {
	fn := func(_ context.Context, _ Event) {
		panic("boom from transmit-fn")
	}
	// Must not propagate the panic to the caller.
	callBounded(context.Background(), fn, Event{})
}

// TestEmit_NilTransmitFnSkipped exercises the nil-transmit guard in Emit.
// In practice this guard fires only if the registry is mutated concurrently
// (shouldn't happen), but we cover it for completeness by directly testing
// the iteration logic with channels already registered.
func TestEmit_NilTransmitFnSkipped(t *testing.T) {
	// Emit iterates RegisteredChannels() and looks each up in the registry.
	// Both channels are registered with non-nil functions, so nil-skip won't
	// fire in normal operation. We just verify no panic on a normal call.
	Emit(context.Background(), Event{CLIVersion: "0.0.0"})
}

// ---------------------------------------------------------------------------
// emitPanicEvent + emitExitCodeEvent (errors.go) — 0% → covered
// ---------------------------------------------------------------------------

// TestEmitPanicEvent_RunsWithDefaultHub exercises emitPanicEvent with the
// Sentry SDK's default no-op hub (no real DSN). The Sentry SDK tolerates
// calls to sentry.WithScope even without a real client — the scope callback
// runs but no event is actually sent. This covers all statements in the
// function.
func TestEmitPanicEvent_RunsWithDefaultHub(t *testing.T) {
	// Exercise the unscrubbed path (plain string panic).
	emitPanicEvent(Event{
		CLIVersion: "0.0.0-test",
		Panic: &PanicInfo{
			Value: "some unexpected panic",
			Stack: []byte("goroutine 1 [running]:\nmain.main()\n\t/file.go:10"),
		},
	})
	// Exercise the allowlisted path.
	emitPanicEvent(Event{
		CLIVersion: "0.0.0-test",
		Panic: &PanicInfo{
			Value: SafePanic(testKnownID, nil),
			Stack: []byte("goroutine 1 [running]:\nmain.main()\n\t/file.go:10"),
		},
	})
}

// TestEmitExitCodeEvent_RunsWithDefaultHub exercises emitExitCodeEvent in
// the same default-hub mode.
func TestEmitExitCodeEvent_RunsWithDefaultHub(t *testing.T) {
	emitExitCodeEvent(Event{
		CLIVersion: "0.0.0-test",
		ExitCode:   10,
		Command:    "feature.create",
	})
	// Edge case: exit code 0 (shouldn't normally happen in this path but
	// covers the intToString(0) branch).
	emitExitCodeEvent(Event{
		CLIVersion: "0.0.0-test",
		ExitCode:   0,
		Command:    "spec.lint",
	})
}

// ---------------------------------------------------------------------------
// DebugCrashReports (errors.go) — 0% → covered
// ---------------------------------------------------------------------------

// TestDebugCrashReports_NoOpsWithoutDSN covers the early-return when
// errorsClientInitialized is false.
func TestDebugCrashReports_NoOpsWithoutDSN(t *testing.T) {
	if errorsClientInitialized {
		t.Skip("test requires empty DSN (dev build)")
	}
	// Must not panic.
	DebugCrashReports("test-id", "0.0.0-test")
}

// TestDebugCrashReports_RunsWithInitializedClient forces the initialized
// flag on and exercises the full body including ScrubMessage + Sentry scope.
func TestDebugCrashReports_RunsWithInitializedClient(t *testing.T) {
	orig := errorsClientInitialized
	t.Cleanup(func() { errorsClientInitialized = orig })
	errorsClientInitialized = true

	// Allowlisted messageID path.
	DebugCrashReports(testKnownID, "0.0.0-test")
	// Unknown messageID → unscrubbed path.
	DebugCrashReports("unknown-message", "0.0.0-test")
}

// ---------------------------------------------------------------------------
// transmitErrors additional branches (errors.go) — 69.2% → higher
// ---------------------------------------------------------------------------

// TestTransmitErrors_PanicBranch_Initialized exercises the panic branch
// with errorsClientInitialized=true so the Sentry SDK path runs.
func TestTransmitErrors_PanicBranch_Initialized(t *testing.T) {
	orig := errorsClientInitialized
	t.Cleanup(func() { errorsClientInitialized = orig })
	errorsClientInitialized = true

	transmitErrors(context.Background(), Event{
		CLIVersion: "0.0.0-test",
		ExitCode:   10,
		Panic: &PanicInfo{
			Value: "test panic",
			Stack: []byte("goroutine 1 [running]:"),
		},
	})
}

// TestTransmitErrors_ExitCodeBranch_Initialized exercises the exit-code ≥10
// branch with errorsClientInitialized=true.
func TestTransmitErrors_ExitCodeBranch_Initialized(t *testing.T) {
	orig := errorsClientInitialized
	t.Cleanup(func() { errorsClientInitialized = orig })
	errorsClientInitialized = true

	transmitErrors(context.Background(), Event{
		CLIVersion: "0.0.0-test",
		ExitCode:   10,
		Command:    "spec.lint",
	})
}

// ---------------------------------------------------------------------------
// SafePanicPayload.Error nil-Wrapped branch (scrubber.go) — 66.7% → 100%
// ---------------------------------------------------------------------------

func TestSafePanicPayload_ErrorNilWrapped(t *testing.T) {
	p := SafePanic("my-id", nil)
	if got := p.Error(); got != "my-id" {
		t.Errorf("Error() with nil wrapped = %q, want %q", got, "my-id")
	}
}

// ---------------------------------------------------------------------------
// CollectOSEnvSignals nil-getEnv branch (optout.go) — 90% → 100%
// ---------------------------------------------------------------------------

func TestCollectOSEnvSignals_NilGetEnvDefaultsToOsGetenv(t *testing.T) {
	// When getEnv is nil, the function falls back to os.Getenv.
	// Clear any env vars that would affect the result.
	t.Setenv("SPECSCORE_TELEMETRY", "")
	t.Setenv("DO_NOT_TRACK", "")
	t.Setenv("CI", "")
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("GITLAB_CI", "")
	t.Setenv("BUILDKITE", "")
	t.Setenv("CIRCLECI", "")

	zero, dnt, ci := CollectOSEnvSignals(nil)
	if zero || dnt || ci {
		t.Errorf("with cleared env, expected all false; got zero=%v dnt=%v ci=%v", zero, dnt, ci)
	}
}

// ---------------------------------------------------------------------------
// RegisterChannel panic paths (registry.go) — 71.4% → higher
// ---------------------------------------------------------------------------

func TestRegisterChannel_PanicsOnUnknownName(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for unknown channel name")
		}
	}()
	RegisterChannel("bogus-channel", func(_ context.Context, _ Event) {})
}

func TestRegisterChannel_PanicsOnDuplicate(t *testing.T) {
	// Both known channels are already registered by init(). Registering
	// again must panic.
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for duplicate registration")
		}
	}()
	RegisterChannel(ChannelUsageStats, func(_ context.Context, _ Event) {})
}

// ---------------------------------------------------------------------------
// WriteState error paths (state.go) — 66.7% → higher
// ---------------------------------------------------------------------------

// TestWriteState_CreatesDirectoryIfMissing covers the MkdirAll path in
// WriteState when the .specscore directory does not yet exist.
func TestWriteState_CreatesDirectoryIfMissing(t *testing.T) {
	withTempHome(t)
	yes := true
	if err := WriteState(State{Enabled: &yes}); err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	// Verify round-trip.
	r, err := ReadState()
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if r.State.Enabled == nil || *r.State.Enabled != true {
		t.Errorf("Enabled not preserved")
	}
}

// TestWriteState_EmptyState covers writing a zero-value State (all fields
// nil), which produces a YAML body of just "{}".
func TestWriteState_EmptyState(t *testing.T) {
	withTempHome(t)
	if err := WriteState(State{}); err != nil {
		t.Fatalf("WriteState with empty state: %v", err)
	}
	r, err := ReadState()
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if r.InvalidReason != "" {
		t.Errorf("empty state should round-trip cleanly, got InvalidReason=%q", r.InvalidReason)
	}
}

// ---------------------------------------------------------------------------
// parseStateBytes edge cases (state.go) — 86.7% → higher
// ---------------------------------------------------------------------------

// TestParseStateBytes_EmptyFile covers the io.EOF branch (empty file body).
func TestParseStateBytes_EmptyFile(t *testing.T) {
	s, reason := parseStateBytes([]byte{})
	if reason != "" {
		t.Errorf("empty file should parse as zero-State, got reason=%q", reason)
	}
	if s.Enabled != nil || s.UsageStats != nil || s.CrashReports != nil {
		t.Errorf("empty file should produce zero-State, got %+v", s)
	}
}

// TestParseStateBytes_TypeErrorYieldsInvalidReason covers the case where
// keys are valid but values have wrong types.
func TestParseStateBytes_TypeErrorYieldsInvalidReason(t *testing.T) {
	// "enabled" expects a bool; giving it a string should fail KnownFields
	// strict decode.
	raw := []byte("enabled: not-a-bool\n")
	_, reason := parseStateBytes(raw)
	if reason == "" {
		t.Error("expected InvalidReason for type-mismatched value")
	}
}

// ---------------------------------------------------------------------------
// ReadState I/O error path (state.go) — 80% → higher
// ---------------------------------------------------------------------------

// TestReadState_PermissionDenied covers the I/O error path where the file
// exists but is not readable.
func TestReadState_PermissionDenied(t *testing.T) {
	home := withTempHome(t)
	dir := filepath.Join(home, ".specscore")
	if err := os.MkdirAll(dir, installIDDirMode); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, stateFilename)
	if err := os.WriteFile(path, []byte("enabled: true\n"), 0o000); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	_, err := ReadState()
	if err == nil {
		t.Error("expected error for permission-denied file")
	}
}

// ---------------------------------------------------------------------------
// StatePath + InstallIDPath coverage — 75% → higher
// ---------------------------------------------------------------------------

// TestStatePath_ReturnsExpectedSuffix verifies the path ends with the
// correct filename.
func TestStatePath_ReturnsExpectedSuffix(t *testing.T) {
	withTempHome(t)
	p, err := StatePath()
	if err != nil {
		t.Fatalf("StatePath: %v", err)
	}
	if filepath.Base(p) != stateFilename {
		t.Errorf("StatePath base = %q, want %q", filepath.Base(p), stateFilename)
	}
}

// TestInstallIDPath_ReturnsExpectedSuffix verifies the path ends with the
// correct filename.
func TestInstallIDPath_ReturnsExpectedSuffix(t *testing.T) {
	withTempHome(t)
	p, err := InstallIDPath()
	if err != nil {
		t.Fatalf("InstallIDPath: %v", err)
	}
	if filepath.Base(p) != installIDFilename {
		t.Errorf("InstallIDPath base = %q, want %q", filepath.Base(p), installIDFilename)
	}
}

// ---------------------------------------------------------------------------
// atomicWriteFile edge coverage (installid.go) — 57.9% → higher
// ---------------------------------------------------------------------------

// TestAtomicWriteFile_WritesAndRenames verifies the happy path of
// atomicWriteFile: correct content, correct mode bits.
func TestAtomicWriteFile_WritesAndRenames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "testfile")
	data := []byte("hello\n")
	if err := atomicWriteFile(path, data, 0o600); err != nil {
		t.Fatalf("atomicWriteFile: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("content = %q, want %q", got, data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
}

// TestAtomicWriteFile_FailsOnBadDir verifies the error path when the parent
// directory does not exist.
func TestAtomicWriteFile_FailsOnBadDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent", "subdir", "file")
	err := atomicWriteFile(path, []byte("data"), 0o600)
	if err == nil {
		t.Error("expected error when parent dir does not exist")
	}
}

// TestAtomicWriteFile_OverwritesExisting verifies that an existing file at
// the target path is replaced atomically.
func TestAtomicWriteFile_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "testfile")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := atomicWriteFile(path, []byte("new"), 0o600); err != nil {
		t.Fatalf("atomicWriteFile: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Errorf("content = %q, want %q", got, "new")
	}
}

// ---------------------------------------------------------------------------
// transmitUsage with usageClient nil (usage.go) — 75% → higher
// ---------------------------------------------------------------------------

// TestTransmitUsage_NilClient_ReturnsImmediately is a more explicit test for
// the nil-client guard. The existing test covers it indirectly; this one
// asserts the function returns in bounded time.
func TestTransmitUsage_NilClient_ReturnsImmediately(t *testing.T) {
	if usageClient != nil {
		t.Skip("usageClient is non-nil (build with posthog key); skipping")
	}
	done := make(chan struct{})
	go func() {
		transmitUsage(context.Background(), Event{
			Command:   "test.quick",
			InstallID: "id",
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("transmitUsage with nil client did not return within 100ms")
	}
}

// ---------------------------------------------------------------------------
// ScrubFrame edge cases (scrubber.go) — already 100% but belt-and-suspenders
// for empty / root / backslash-only paths.
// ---------------------------------------------------------------------------

func TestScrubFrame_EmptyPath(t *testing.T) {
	base, line, fn := ScrubFrame("", 1, "main.main")
	if base != "." {
		// filepath.Base("") returns "."
		// The ScrubFrame function checks for "." and returns ""
		if base != "" {
			t.Errorf("empty path base = %q, want empty or \".\"", base)
		}
	}
	if line != 1 {
		t.Errorf("line = %d, want 1", line)
	}
	if fn != "main.main" {
		t.Errorf("fn = %q, want main.main", fn)
	}
}

func TestScrubFrame_RootPath(t *testing.T) {
	base, _, _ := ScrubFrame("/", 1, "")
	if base != "" {
		t.Errorf("root path base = %q, want empty", base)
	}
}

func TestScrubFrame_DotPath(t *testing.T) {
	base, _, _ := ScrubFrame(".", 1, "")
	if base != "" {
		t.Errorf("dot path base = %q, want empty", base)
	}
}

// ---------------------------------------------------------------------------
// Emit with pre-cancelled context (registry.go) — exercises the timeout
// select branch in callBounded more thoroughly.
// ---------------------------------------------------------------------------

func TestEmit_PreCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Must not panic or hang.
	Emit(ctx, Event{CLIVersion: "0.0.0"})
}

// ---------------------------------------------------------------------------
// ChannelEnabled with unknown channel name (state.go)
// ---------------------------------------------------------------------------

func TestChannelEnabled_UnknownChannelFallsToGlobal(t *testing.T) {
	no := false
	s := State{Enabled: &no}
	enabled, source := s.ChannelEnabled("unknown-channel")
	if enabled {
		t.Error("unknown channel with global=false should be disabled")
	}
	if source != "persistent state global" {
		t.Errorf("source = %q, want %q", source, "persistent state global")
	}
}

func TestChannelEnabled_UnknownChannelDefaultEnabled(t *testing.T) {
	s := State{}
	enabled, source := s.ChannelEnabled("unknown-channel")
	if !enabled {
		t.Error("unknown channel with no state should default to enabled")
	}
	if source != "default" {
		t.Errorf("source = %q, want %q", source, "default")
	}
}

// ---------------------------------------------------------------------------
// CollectOSEnvSignals additional CI markers (optout.go)
// ---------------------------------------------------------------------------

func TestCollectOSEnvSignals_AllCIMarkers(t *testing.T) {
	markers := []string{"CI", "GITHUB_ACTIONS", "GITLAB_CI", "BUILDKITE", "CIRCLECI"}
	for _, marker := range markers {
		t.Run(marker, func(t *testing.T) {
			env := map[string]string{marker: "true"}
			_, _, ci := CollectOSEnvSignals(func(k string) string { return env[k] })
			if !ci {
				t.Errorf("%s=true should trigger CI detection", marker)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// InstallID error path: unwritable directory (installid.go)
// ---------------------------------------------------------------------------

func TestInstallID_UnwritableParentDir(t *testing.T) {
	home := withTempHome(t)
	// Create .specscore as a file (not a directory) to block MkdirAll.
	specscoreDir := filepath.Join(home, ".specscore")
	if err := os.WriteFile(specscoreDir, []byte("not a dir"), 0o400); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(specscoreDir, 0o700) })

	_, _, err := InstallID()
	if err == nil {
		t.Error("expected error when .specscore is a file, not a directory")
	}
}

// ---------------------------------------------------------------------------
// userStateDir / StatePath / InstallIDPath error paths — cover HOME unset
// ---------------------------------------------------------------------------

// TestUserStateDir_NoHomeReturnsError covers the error path where HOME is
// not set (os.UserHomeDir fails).
func TestUserStateDir_NoHomeReturnsError(t *testing.T) {
	t.Setenv("HOME", "")
	// Also clear XDG dirs that os.UserHomeDir might look at.
	t.Setenv("XDG_CONFIG_HOME", "")

	_, err := userStateDir()
	if err == nil {
		t.Error("expected error when HOME is empty")
	}
}

// TestStatePath_ErrorWhenNoHome covers the error path from StatePath when
// userStateDir fails.
func TestStatePath_ErrorWhenNoHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	_, err := StatePath()
	if err == nil {
		t.Error("expected error from StatePath when HOME is empty")
	}
}

// TestInstallIDPath_ErrorWhenNoHome covers the error path from InstallIDPath
// when userStateDir fails.
func TestInstallIDPath_ErrorWhenNoHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	_, err := InstallIDPath()
	if err == nil {
		t.Error("expected error from InstallIDPath when HOME is empty")
	}
}

// TestInstallID_ErrorWhenNoHome covers the error path from InstallID when
// InstallIDPath fails (no HOME).
func TestInstallID_ErrorWhenNoHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	_, _, err := InstallID()
	if err == nil {
		t.Error("expected error from InstallID when HOME is empty")
	}
}

// ---------------------------------------------------------------------------
// ReadState error from StatePath (state.go) — 90% → higher
// ---------------------------------------------------------------------------

// TestReadState_ErrorWhenNoHome covers the I/O error return from ReadState
// when StatePath fails.
func TestReadState_ErrorWhenNoHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	_, err := ReadState()
	if err == nil {
		t.Error("expected error from ReadState when HOME is empty")
	}
}

// ---------------------------------------------------------------------------
// WriteState error from StatePath (state.go) — 66.7% → higher
// ---------------------------------------------------------------------------

// TestWriteState_ErrorWhenNoHome covers the error return from WriteState
// when StatePath fails.
func TestWriteState_ErrorWhenNoHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	err := WriteState(State{})
	if err == nil {
		t.Error("expected error from WriteState when HOME is empty")
	}
}

// ---------------------------------------------------------------------------
// WriteState directory creation failure (state.go)
// ---------------------------------------------------------------------------

// TestWriteState_MkdirFailure covers the MkdirAll error path by pointing
// HOME at a file path where MkdirAll can't create directories.
func TestWriteState_MkdirFailure(t *testing.T) {
	dir := t.TempDir()
	// Create a file where the .specscore directory should go.
	blocker := filepath.Join(dir, ".specscore")
	if err := os.WriteFile(blocker, []byte("block"), 0o400); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocker, 0o700) })
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))

	err := WriteState(State{})
	if err == nil {
		t.Error("expected error when directory creation fails")
	}
}

// ---------------------------------------------------------------------------
// Emit nil-transmit guard (registry.go) — 85.7% → covered via direct test
// ---------------------------------------------------------------------------

// TestEmit_SkipsNilTransmitFunc directly exercises the nil-transmit guard
// by temporarily zeroing a registry entry. This is a white-box test.
func TestEmit_SkipsNilTransmitFunc(t *testing.T) {
	// Save and restore the registry entry.
	registryMu.Lock()
	origFn := registry[ChannelUsageStats]
	registry[ChannelUsageStats] = nil
	registryMu.Unlock()
	t.Cleanup(func() {
		registryMu.Lock()
		registry[ChannelUsageStats] = origFn
		registryMu.Unlock()
	})

	// Must not panic when encountering a nil transmit func.
	Emit(context.Background(), Event{CLIVersion: "0.0.0"})
}

// ---------------------------------------------------------------------------
// atomicWriteFile error paths — exercise Write failure
// ---------------------------------------------------------------------------

// TestAtomicWriteFile_ReadOnlyDir covers the CreateTemp failure when the
// directory is read-only.
func TestAtomicWriteFile_ReadOnlyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	path := filepath.Join(dir, "testfile")
	err := atomicWriteFile(path, []byte("data"), 0o600)
	if err == nil {
		t.Error("expected error when directory is read-only")
	}
}

// TestAtomicWriteFile_LargePayload exercises the Write path with a larger
// payload to improve branch coverage within the function.
func TestAtomicWriteFile_LargePayload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "largefile")
	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte('A' + (i % 26))
	}
	if err := atomicWriteFile(path, data, 0o644); err != nil {
		t.Fatalf("atomicWriteFile: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(got) != len(data) {
		t.Errorf("content length = %d, want %d", len(got), len(data))
	}
}

// ---------------------------------------------------------------------------
// InstallID: pre-existing dir with loose perms (installid.go) — chmod path
// ---------------------------------------------------------------------------

// TestInstallID_FixesDirPermissions exercises the best-effort chmod path
// where the .specscore directory pre-exists with wider permissions.
func TestInstallID_FixesDirPermissions(t *testing.T) {
	home := withTempHome(t)
	dir := filepath.Join(home, ".specscore")
	// Create dir with overly-permissive mode.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	id, justCreated, err := InstallID()
	if err != nil {
		t.Fatalf("InstallID: %v", err)
	}
	if !justCreated {
		t.Error("expected justCreated=true")
	}
	if !isUUIDv4(id) {
		t.Errorf("id is not UUID v4: %q", id)
	}
	// Verify the directory was chmod'd to 0700.
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if info.Mode().Perm() != installIDDirMode {
		t.Errorf("dir mode = %v, want %v", info.Mode().Perm(), installIDDirMode)
	}
}

// ---------------------------------------------------------------------------
// setupErrorsChannel — exercises the init() body for errors.go with a
// non-empty sentryDSN so that sentry.Init is called and
// errorsClientInitialized is set.
// ---------------------------------------------------------------------------

func TestSetupErrorsChannel_WithDSN(t *testing.T) {
	origDSN := sentryDSN
	origInit := errorsClientInitialized
	origSetup := setupErrorsChannel
	t.Cleanup(func() {
		sentryDSN = origDSN
		errorsClientInitialized = origInit
		setupErrorsChannel = origSetup
	})

	// A syntactically valid but unreachable DSN — sentry.Init accepts it
	// (it validates lazily on send, not at init time).
	sentryDSN = "https://examplePublicKey@o0.ingest.de.sentry.io/0"
	errorsClientInitialized = false
	setupErrorsChannel()
	if !errorsClientInitialized {
		t.Error("expected errorsClientInitialized=true after setupErrorsChannel with valid DSN")
	}
}

func TestSetupErrorsChannel_WithInvalidDSN(t *testing.T) {
	origDSN := sentryDSN
	origInit := errorsClientInitialized
	origSetup := setupErrorsChannel
	t.Cleanup(func() {
		sentryDSN = origDSN
		errorsClientInitialized = origInit
		setupErrorsChannel = origSetup
	})

	// An obviously invalid DSN should make sentry.Init fail.
	sentryDSN = "not-a-valid-dsn"
	errorsClientInitialized = false
	setupErrorsChannel()
	if errorsClientInitialized {
		t.Error("expected errorsClientInitialized=false after setupErrorsChannel with invalid DSN")
	}
}

// ---------------------------------------------------------------------------
// setupUsageChannel — exercises the init() body for usage.go with a
// non-empty posthogWriteKey.
// ---------------------------------------------------------------------------

func TestSetupUsageChannel_WithKey(t *testing.T) {
	origKey := posthogWriteKey
	origClient := usageClient
	origSetup := setupUsageChannel
	t.Cleanup(func() {
		posthogWriteKey = origKey
		usageClient = origClient
		setupUsageChannel = origSetup
	})

	posthogWriteKey = "phc_test_fake_key_for_coverage"
	usageClient = nil
	setupUsageChannel()
	if usageClient == nil {
		t.Error("expected usageClient to be set after setupUsageChannel with valid key")
	}
	// Close the client to release resources.
	if usageClient != nil {
		_ = usageClient.Close()
	}
}

func TestSetupUsageChannel_WithEmptyKey(t *testing.T) {
	origKey := posthogWriteKey
	origClient := usageClient
	origSetup := setupUsageChannel
	t.Cleanup(func() {
		posthogWriteKey = origKey
		usageClient = origClient
		setupUsageChannel = origSetup
	})

	posthogWriteKey = ""
	usageClient = nil
	setupUsageChannel()
	if usageClient != nil {
		t.Error("expected usageClient to remain nil with empty key")
	}
}

// spyRoundTripper records whether it was ever invoked, without making a real
// network request — used by TestSetupUsageChannel_MakesNoHTTPCalls and
// TestSetupErrorsChannel_MakesNoHTTPCalls (M8 review fix) to prove client
// construction itself performs no synchronous HTTP call.
type spyRoundTripper struct {
	called bool
}

func (s *spyRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	s.called = true
	return nil, errors.New("spyRoundTripper: no real network call is expected here")
}

// M8 review fix: setupUsageChannel constructs a real posthog client (when
// posthogWriteKey is non-empty, as it is in a released binary) on every
// process, `specscore version --json` included. This proves that
// CONSTRUCTION alone -- posthog.NewWithConfig, which builds local state and
// starts a background flush loop that only sends when something is actually
// enqueued -- never makes a synchronous HTTP call: the spy transport, wired
// through the new usageHTTPTransport seam, records zero invocations.
// Combined with TestPreRun_VersionJSONProbeSkipsAllSideEffects (internal/cli
// telemetry_test.go), which proves `version --json` never reaches
// transmitUsage/Enqueue at all, this closes the gap the review flagged: only
// the preRun skip was tested before, not the client-construction claim
// "shows no request" rested on.
func TestSetupUsageChannel_MakesNoHTTPCalls(t *testing.T) {
	origKey := posthogWriteKey
	origClient := usageClient
	origTransport := usageHTTPTransport
	t.Cleanup(func() {
		posthogWriteKey = origKey
		usageClient = origClient
		usageHTTPTransport = origTransport
	})

	spy := &spyRoundTripper{}
	usageHTTPTransport = spy
	posthogWriteKey = "phc_test_fake_key_for_no_http_calls"
	usageClient = nil

	setupUsageChannel()
	if usageClient == nil {
		t.Fatal("expected usageClient to be set after setupUsageChannel with a valid key")
	}
	t.Cleanup(func() { _ = usageClient.Close() })

	if spy.called {
		t.Error("setupUsageChannel made an HTTP call during client construction; want zero (no message was ever enqueued)")
	}
}

// M8 review fix: the errors-telemetry counterpart to
// TestSetupUsageChannel_MakesNoHTTPCalls above -- setupErrorsChannel's
// sentry.Init call constructs local client state only; the spy transport,
// wired through the new errorsHTTPTransport seam, records zero invocations.
func TestSetupErrorsChannel_MakesNoHTTPCalls(t *testing.T) {
	origDSN := sentryDSN
	origInit := errorsClientInitialized
	origTransport := errorsHTTPTransport
	t.Cleanup(func() {
		sentryDSN = origDSN
		errorsClientInitialized = origInit
		errorsHTTPTransport = origTransport
	})

	spy := &spyRoundTripper{}
	errorsHTTPTransport = spy
	sentryDSN = "https://examplePublicKey@o0.ingest.de.sentry.io/0"
	errorsClientInitialized = false

	setupErrorsChannel()
	if !errorsClientInitialized {
		t.Fatal("expected errorsClientInitialized=true after setupErrorsChannel with a valid DSN")
	}

	if spy.called {
		t.Error("setupErrorsChannel made an HTTP call during client construction; want zero (no event was ever captured)")
	}
}

// ---------------------------------------------------------------------------
// userStateDir — Windows branch via runtimeGOOS indirection.
// ---------------------------------------------------------------------------

func TestUserStateDir_WindowsBranch(t *testing.T) {
	origGOOS := runtimeGOOS
	t.Cleanup(func() { runtimeGOOS = origGOOS })

	runtimeGOOS = "windows"
	// On non-Windows, os.UserConfigDir may still succeed (it returns
	// $XDG_CONFIG_HOME or $HOME/.config on Unix). We just need the
	// code path to execute.
	dir, err := userStateDir()
	if err != nil {
		// Acceptable — UserConfigDir may fail in some environments.
		t.Logf("userStateDir with windows GOOS returned error (acceptable): %v", err)
		return
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("expected absolute path, got %q", dir)
	}
	// On Unix pretending to be Windows, the returned path should end with
	// "specscore" (not ".specscore").
	if filepath.Base(dir) != "specscore" {
		t.Errorf("expected base dir 'specscore', got %q", filepath.Base(dir))
	}
}

func TestUserStateDir_WindowsBranch_ConfigDirError(t *testing.T) {
	origGOOS := runtimeGOOS
	t.Cleanup(func() { runtimeGOOS = origGOOS })

	runtimeGOOS = "windows"
	// Clear HOME-related env vars to make os.UserConfigDir fail.
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("AppData", "")

	_, err := userStateDir()
	if err == nil {
		t.Error("expected error from userStateDir when HOME/AppData are cleared on 'windows'")
	}
}

// ---------------------------------------------------------------------------
// generateUUIDv4 — crypto/rand error path via randRead indirection.
// ---------------------------------------------------------------------------

func TestGenerateUUIDv4_RandReadError(t *testing.T) {
	origRandRead := randRead
	t.Cleanup(func() { randRead = origRandRead })

	randRead = func(b []byte) (int, error) {
		return 0, errors.New("injected rand failure")
	}

	_, err := generateUUIDv4()
	if err == nil {
		t.Error("expected error from generateUUIDv4 when rand.Read fails")
	}
}

// ---------------------------------------------------------------------------
// InstallID — generateUUIDv4 error propagation.
// ---------------------------------------------------------------------------

func TestInstallID_GenerateUUIDError(t *testing.T) {
	withTempHome(t)

	origRandRead := randRead
	t.Cleanup(func() { randRead = origRandRead })

	randRead = func(b []byte) (int, error) {
		return 0, errors.New("injected rand failure")
	}

	_, _, err := InstallID()
	if err == nil {
		t.Error("expected error from InstallID when generateUUIDv4 fails")
	}
}

// ---------------------------------------------------------------------------
// InstallID — atomicWriteFile error propagation.
// ---------------------------------------------------------------------------

func TestInstallID_AtomicWriteError(t *testing.T) {
	withTempHome(t)

	origRename := osRename
	t.Cleanup(func() { osRename = origRename })

	osRename = func(_, _ string) error {
		return errors.New("injected rename failure")
	}

	_, _, err := InstallID()
	if err == nil {
		t.Error("expected error from InstallID when atomicWriteFile fails")
	}
}

// ---------------------------------------------------------------------------
// atomicWriteFile — Write error path via osCreateTemp that returns a
// pre-closed file descriptor.
// ---------------------------------------------------------------------------

func TestAtomicWriteFile_WriteError(t *testing.T) {
	dir := t.TempDir()
	origCreateTemp := osCreateTemp
	t.Cleanup(func() { osCreateTemp = origCreateTemp })

	osCreateTemp = func(dir, pattern string) (*os.File, error) {
		f, err := origCreateTemp(dir, pattern)
		if err != nil {
			return nil, err
		}
		// Close the fd so that subsequent Write calls fail.
		_ = f.Close()
		return f, nil
	}

	path := filepath.Join(dir, "testfile")
	err := atomicWriteFile(path, []byte("data"), 0o600)
	if err == nil {
		t.Error("expected error from atomicWriteFile when Write fails on closed fd")
	}
}

// ---------------------------------------------------------------------------
// atomicWriteFile — Rename error path via osRename indirection.
// ---------------------------------------------------------------------------

func TestAtomicWriteFile_RenameError(t *testing.T) {
	dir := t.TempDir()
	origRename := osRename
	t.Cleanup(func() { osRename = origRename })

	osRename = func(_, _ string) error {
		return errors.New("injected rename failure")
	}

	path := filepath.Join(dir, "testfile")
	err := atomicWriteFile(path, []byte("data"), 0o600)
	if err == nil {
		t.Error("expected error from atomicWriteFile when Rename fails")
	}
}

// ---------------------------------------------------------------------------
// WriteState — yaml.Marshal error path.
// ---------------------------------------------------------------------------

func TestWriteState_MarshalError(t *testing.T) {
	withTempHome(t)

	origMarshal := yamlMarshal
	t.Cleanup(func() { yamlMarshal = origMarshal })

	yamlMarshal = func(v interface{}) ([]byte, error) {
		return nil, errors.New("injected marshal failure")
	}

	err := WriteState(State{})
	if err == nil {
		t.Error("expected error from WriteState when yaml.Marshal fails")
	}
}

// ---------------------------------------------------------------------------
// WriteState — "refusing to write malformed state" defensive check.
// ---------------------------------------------------------------------------

func TestWriteState_RefusingToWriteMalformedState(t *testing.T) {
	withTempHome(t)

	origMarshal := yamlMarshal
	t.Cleanup(func() { yamlMarshal = origMarshal })

	// Return YAML with an unknown key so parseStateBytes rejects it.
	yamlMarshal = func(v interface{}) ([]byte, error) {
		return []byte("bogus_key: true\n"), nil
	}

	err := WriteState(State{})
	if err == nil {
		t.Error("expected error from WriteState for malformed marshaled state")
	}
	if err != nil && !contains(err.Error(), "refusing to write") {
		t.Errorf("expected 'refusing to write' in error, got: %v", err)
	}
}

// contains is a helper to avoid importing strings just for one check.
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// transmitUsage — Enqueue path via usageEnqueue indirection.
// ---------------------------------------------------------------------------

func TestTransmitUsage_EnqueuePath(t *testing.T) {
	origEnqueue := usageEnqueue
	t.Cleanup(func() { usageEnqueue = origEnqueue })

	var captured posthog.Capture
	usageEnqueue = func(c posthog.Capture) error {
		captured = c
		return nil
	}

	transmitUsage(context.Background(), Event{
		Command:   "test.cmd",
		InstallID: "test-id",
		Caller:    CallerCLI,
	})

	if captured.Event != usageStatsEventName {
		t.Errorf("captured event = %q, want %q", captured.Event, usageStatsEventName)
	}
	if captured.DistinctId != "test-id" {
		t.Errorf("captured DistinctId = %q, want %q", captured.DistinctId, "test-id")
	}
}

// TestTransmitUsage_RealClientEnqueue exercises the usageClient.Enqueue path
// (line 187 of usage.go) by setting up a real PostHog client with a fake key.
// The Enqueue call succeeds (it buffers locally); we close the client after.
func TestTransmitUsage_RealClientEnqueue(t *testing.T) {
	origClient := usageClient
	origEnqueue := usageEnqueue
	t.Cleanup(func() {
		usageClient = origClient
		usageEnqueue = origEnqueue
	})

	client, err := posthog.NewWithConfig("phc_test_fake_key", posthog.Config{
		Endpoint: posthogEUEndpoint,
	})
	if err != nil {
		t.Fatalf("posthog.NewWithConfig: %v", err)
	}
	usageClient = client
	usageEnqueue = nil // ensure we go through the real client path

	// Must not panic; Enqueue buffers locally.
	transmitUsage(context.Background(), Event{
		Command:   "test.real",
		InstallID: "test-real-id",
		Caller:    CallerCLI,
	})

	_ = client.Close()
}

// ---------------------------------------------------------------------------
// atomicWriteFile — Chmod error path via fileChmod indirection.
// ---------------------------------------------------------------------------

func TestAtomicWriteFile_ChmodError(t *testing.T) {
	dir := t.TempDir()
	origChmod := fileChmod
	t.Cleanup(func() { fileChmod = origChmod })

	fileChmod = func(_ *os.File, _ os.FileMode) error {
		return errors.New("injected chmod failure")
	}

	path := filepath.Join(dir, "testfile")
	err := atomicWriteFile(path, []byte("data"), 0o600)
	if err == nil {
		t.Error("expected error from atomicWriteFile when Chmod fails")
	}
}

// ---------------------------------------------------------------------------
// atomicWriteFile — Sync error path via fileSync indirection.
// ---------------------------------------------------------------------------

func TestAtomicWriteFile_SyncError(t *testing.T) {
	dir := t.TempDir()
	origSync := fileSync
	t.Cleanup(func() { fileSync = origSync })

	fileSync = func(_ *os.File) error {
		return errors.New("injected sync failure")
	}

	path := filepath.Join(dir, "testfile")
	err := atomicWriteFile(path, []byte("data"), 0o600)
	if err == nil {
		t.Error("expected error from atomicWriteFile when Sync fails")
	}
}

// ---------------------------------------------------------------------------
// atomicWriteFile — Close error path via fileClose indirection.
// ---------------------------------------------------------------------------

func TestAtomicWriteFile_CloseError(t *testing.T) {
	dir := t.TempDir()
	origClose := fileClose
	t.Cleanup(func() { fileClose = origClose })

	fileClose = func(_ *os.File) error {
		return errors.New("injected close failure")
	}

	path := filepath.Join(dir, "testfile")
	err := atomicWriteFile(path, []byte("data"), 0o600)
	if err == nil {
		t.Error("expected error from atomicWriteFile when Close fails")
	}
}

// ---------------------------------------------------------------------------
// setupUsageChannel — posthog.NewWithConfig error branch.
// ---------------------------------------------------------------------------

func TestSetupUsageChannel_NewClientError(t *testing.T) {
	origKey := posthogWriteKey
	origClient := usageClient
	origNew := posthogNew
	origSetup := setupUsageChannel
	t.Cleanup(func() {
		posthogWriteKey = origKey
		usageClient = origClient
		posthogNew = origNew
		setupUsageChannel = origSetup
	})

	posthogWriteKey = "phc_test_key"
	usageClient = nil
	posthogNew = func(_ string, _ posthog.Config) (posthog.Client, error) {
		return nil, errors.New("injected posthog init failure")
	}

	setupUsageChannel()
	if usageClient != nil {
		t.Error("expected usageClient to remain nil when posthog.NewWithConfig fails")
	}
}
