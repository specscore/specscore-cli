package cli

// Tests for `event merge-driver` (the git custom merge driver for
// .specscore/events.jsonl) and `merge-driver install`/`merge-driver index`
// (the repo attribute/config wiring plus the generated-index driver). See
// docs/merge-drivers.md.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/specscore/specscore-cli/pkg/event"
	"github.com/specscore/specscore-cli/pkg/exitcode"
)

func driverTestEvent(id, payload string) event.Event {
	return event.Event{
		Name:      "lesson.observed",
		Version:   1,
		UUID:      id,
		Timestamp: time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC),
		Actor:     event.Actor{Kind: "external", ID: "merge-driver-test"},
		Artifact:  event.Artifact{Type: "lesson", ID: "ledger", Path: "spec/lessons/ledger/README.md", Revision: "uncommitted"},
		Payload:   json.RawMessage(payload),
	}
}

func writeDriverLedger(t *testing.T, path string, events ...event.Event) {
	t.Helper()
	var b []byte
	for _, e := range events {
		line, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		b = append(b, line...)
		b = append(b, '\n')
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// runEventMergeDriverCmd invokes `event merge-driver <base> <ours> <theirs>`
// in-process, the shape git itself calls with %O %A %B substituted.
func runEventMergeDriverCmd(t *testing.T, base, ours, theirs string) (string, string, error) {
	t.Helper()
	return runEvent(t, "merge-driver", base, ours, theirs)
}

// TestEventMergeDriver_PureAppendsBothSides covers the Facts scenario
// directly: two branches each append a distinct new event with no shared
// history beyond the base. The driver must produce a superset containing
// every event from both sides, with <ours> bytes/order preserved and only
// <theirs>-only events appended (sorted by UUID, matching `event merge`'s
// documented contract).
func TestEventMergeDriver_PureAppendsBothSides(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.jsonl")
	ours := filepath.Join(dir, "ours.jsonl")
	theirs := filepath.Join(dir, "theirs.jsonl")

	shared := driverTestEvent("00000000-0000-4000-8000-000000000001", `{"shared":true}`)
	oursOnly := driverTestEvent("00000000-0000-4000-8000-000000000003", `{"branch":"ours"}`)
	theirsOnly := driverTestEvent("00000000-0000-4000-8000-000000000002", `{"branch":"theirs"}`)

	writeDriverLedger(t, base, shared)
	writeDriverLedger(t, ours, shared, oursOnly)
	writeDriverLedger(t, theirs, shared, theirsOnly)

	out, _, err := runEventMergeDriverCmd(t, base, ours, theirs)
	if err != nil {
		t.Fatalf("merge-driver: %v", err)
	}
	if !strings.Contains(out, "added=1") {
		t.Fatalf("output = %q, want added=1 (theirs-only event)", out)
	}

	got, err := os.ReadFile(ours)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(got), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("merged ledger has %d lines, want 3 (shared + oursOnly + theirsOnly): %v", len(lines), lines)
	}
	// <ours> bytes/order preserved as the first two lines (shared, then
	// oursOnly, exactly as writeDriverLedger wrote them).
	if !strings.Contains(lines[0], "000000000001") || !strings.Contains(lines[1], "000000000003") {
		t.Fatalf("ours prefix was rewritten: %v", lines)
	}
	// theirs-only event appended after.
	if !strings.Contains(lines[2], "000000000002") {
		t.Fatalf("theirs-only event missing/misplaced: %v", lines)
	}
	for _, line := range lines {
		var decoded map[string]any
		if jsonErr := json.Unmarshal([]byte(line), &decoded); jsonErr != nil {
			t.Fatalf("merged line is not valid JSON: %q: %v", line, jsonErr)
		}
	}
}

// TestEventMergeDriver_IdenticalEventBothSides covers the case where the
// exact same event (e.g. from a common ancestor or an identically-replayed
// emission) is present on both <ours> and <theirs>: it must be deduplicated
// by UUID, not appended twice.
func TestEventMergeDriver_IdenticalEventBothSides(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.jsonl")
	ours := filepath.Join(dir, "ours.jsonl")
	theirs := filepath.Join(dir, "theirs.jsonl")

	e := driverTestEvent("00000000-0000-4000-8000-000000000010", `{"a":1,"b":2}`)
	writeDriverLedger(t, base)
	writeDriverLedger(t, ours, e)
	writeDriverLedger(t, theirs, e)

	out, _, err := runEventMergeDriverCmd(t, base, ours, theirs)
	if err != nil {
		t.Fatalf("merge-driver: %v", err)
	}
	if !strings.Contains(out, "added=0") || !strings.Contains(out, "skipped=1") {
		t.Fatalf("output = %q, want added=0 skipped=1 (deduplicated)", out)
	}
	got, err := os.ReadFile(ours)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(got), "00000000-0000-4000-8000-000000000010") != 1 {
		t.Fatalf("event UUID appears more than once in merged ledger:\n%s", got)
	}
}

// TestEventMergeDriver_ConflictingEditRefusesAndLeavesOursUntouched covers
// the deterministic-refusal rule this task's brief requires: the same event
// UUID with DIFFERENT canonical content on the two sides is never silently
// resolved by picking a side — the driver exits non-zero (git then reports
// an ordinary merge conflict for a human to resolve) and <ours> is left
// byte-for-byte unchanged, so nothing is lost even on the "losing" side.
func TestEventMergeDriver_ConflictingEditRefusesAndLeavesOursUntouched(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.jsonl")
	ours := filepath.Join(dir, "ours.jsonl")
	theirs := filepath.Join(dir, "theirs.jsonl")

	oursEvent := driverTestEvent("00000000-0000-4000-8000-000000000020", `{"rewritten":"ours"}`)
	theirsEvent := driverTestEvent("00000000-0000-4000-8000-000000000020", `{"rewritten":"theirs"}`)
	writeDriverLedger(t, base)
	writeDriverLedger(t, ours, oursEvent)
	writeDriverLedger(t, theirs, theirsEvent)
	oursBefore, err := os.ReadFile(ours)
	if err != nil {
		t.Fatal(err)
	}

	_, errOut, err := runEventMergeDriverCmd(t, base, ours, theirs)
	if err == nil {
		t.Fatal("expected a merge conflict error for divergent content under the same UUID; got nil")
	}
	if exitCodeOf(err) != exitcode.Conflict {
		t.Fatalf("exit code = %d, want exitcode.Conflict (%d); err=%v", exitCodeOf(err), exitcode.Conflict, err)
	}
	if !strings.Contains(errOut, "000000000020") {
		t.Fatalf("stderr = %q, want it to name the conflicting UUID", errOut)
	}
	oursAfter, err := os.ReadFile(ours)
	if err != nil {
		t.Fatal(err)
	}
	if string(oursAfter) != string(oursBefore) {
		t.Fatalf("<ours> was rewritten despite a refused merge:\n before=%q\n after=%q", oursBefore, oursAfter)
	}
}

// TestEventMergeDriver_MissingOursIsTreatedAsEmpty exercises the boundary
// git itself can present: a file added only on <theirs> has no <ours>
// counterpart on disk yet. The driver must still succeed, producing exactly
// <theirs>'s events.
func TestEventMergeDriver_MissingOursIsTreatedAsEmpty(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.jsonl")
	ours := filepath.Join(dir, "ours.jsonl") // never created
	theirs := filepath.Join(dir, "theirs.jsonl")
	writeDriverLedger(t, base)
	writeDriverLedger(t, theirs, driverTestEvent("00000000-0000-4000-8000-000000000030", `{}`))

	out, _, err := runEventMergeDriverCmd(t, base, ours, theirs)
	if err != nil {
		t.Fatalf("merge-driver: %v", err)
	}
	if !strings.Contains(out, "added=1") {
		t.Fatalf("output = %q, want added=1", out)
	}
	got, err := os.ReadFile(ours)
	if err != nil {
		t.Fatalf("ours was not created: %v", err)
	}
	if !strings.Contains(string(got), "000000000030") {
		t.Fatalf("ours content = %q, want theirs's event", got)
	}
}

func TestEventMergeDriverCommand_WrongArgCount(t *testing.T) {
	_, _, err := runEvent(t, "merge-driver", "only-one-arg")
	if err == nil {
		t.Fatal("expected an error for wrong argument count; got nil")
	}
}

func TestEventMergeDriverCommand_Help(t *testing.T) {
	out, _, err := runEvent(t, "merge-driver", "--help")
	if err != nil || !strings.Contains(out, "merge-driver install") || !strings.Contains(out, "%O %A %B") {
		t.Fatalf("help output = %q, err=%v", out, err)
	}
}

// --- Integration: a real git repository proves `git merge` itself succeeds
// cleanly on the Facts scenario (two branches concurrently append to
// events.jsonl) once `merge-driver install` has wired the driver up. This
// exercises the actual git merge machinery, not just the CLI in isolation.

func TestEventMergeDriverIntegration_ConcurrentAppendMergesCleanlyViaGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}
	specscoreBin := buildSpecscoreBinaryForTest(t)
	// Put the freshly built binary FIRST on every git subprocess's PATH so
	// git's invocation of `specscore event merge-driver ...` resolves to
	// this build, never to whatever `specscore` (if any) the host machine
	// already has installed — a stale or absent binary on PATH would make
	// the driver invocation silently no-op (git only checks the driver
	// command's exit code) or fail, and either would be mistaken for this
	// test's own pass/fail signal rather than an environment leak.
	env := append(os.Environ(), "PATH="+filepath.Dir(specscoreBin)+string(os.PathListSeparator)+os.Getenv("PATH"))

	repo := t.TempDir()
	git := func(args ...string) string { return runGitCmdWithEnv(t, repo, env, args...) }

	git("init", "-q", "-b", "main")
	git("config", "user.email", "t@example.com")
	git("config", "user.name", "T")

	if err := os.WriteFile(filepath.Join(repo, "specscore.yaml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	ledgerDir := filepath.Join(repo, ".specscore")
	if err := os.MkdirAll(ledgerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	baseEvent := driverTestEvent("00000000-0000-4000-8000-0000000000b0", `{"seed":true}`)
	writeDriverLedger(t, filepath.Join(ledgerDir, "events.jsonl"), baseEvent)
	git("add", ".")
	git("commit", "-q", "-m", "seed ledger")

	// Install the driver on this clone (repo-local git config + .gitattributes).
	runSpecscoreCmd(t, specscoreBin, repo, "merge-driver", "install")
	// .gitattributes is generated content, not a hand-authored source file —
	// commit it on main so both branches (and the merge itself) see it.
	git("add", ".gitattributes")
	git("commit", "-q", "-m", "wire up merge drivers")

	git("checkout", "-q", "-b", "lane-a")
	laneAEvent := driverTestEvent("00000000-0000-4000-8000-0000000000a1", `{"lane":"a"}`)
	writeDriverLedger(t, filepath.Join(ledgerDir, "events.jsonl"), baseEvent, laneAEvent)
	git("commit", "-am", "lane-a appends")

	git("checkout", "-q", "main")
	git("checkout", "-q", "-b", "lane-b")
	laneBEvent := driverTestEvent("00000000-0000-4000-8000-0000000000b1", `{"lane":"b"}`)
	writeDriverLedger(t, filepath.Join(ledgerDir, "events.jsonl"), baseEvent, laneBEvent)
	git("commit", "-am", "lane-b appends")

	git("checkout", "-q", "main")
	git("merge", "-q", "--no-ff", "lane-a", "-m", "merge lane-a")

	// This is the crux: merging lane-b into main touches the SAME line range
	// of events.jsonl that a plain text merge conflicts on (the Facts
	// scenario). With the driver installed, `git merge` must exit 0.
	mergeCmd := exec.Command("git", "merge", "--no-ff", "lane-b", "-m", "merge lane-b")
	mergeCmd.Dir = repo
	mergeCmd.Env = env
	out, err := mergeCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git merge lane-b failed (merge driver did not resolve the concurrent append cleanly): %v\n%s", err, out)
	}

	merged, err := os.ReadFile(filepath.Join(ledgerDir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"0000000000b0", "0000000000a1", "0000000000b1"} {
		if !strings.Contains(string(merged), want) {
			t.Fatalf("merged ledger missing event %s; content:\n%s", want, merged)
		}
	}
	lines := strings.Split(strings.TrimSuffix(string(merged), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("merged ledger has %d lines, want exactly 3 (no duplicates): %v", len(lines), lines)
	}
	statusOut := git("status", "--porcelain")
	if len(strings.TrimSpace(statusOut)) != 0 {
		t.Fatalf("git status not clean after merge: %s", statusOut)
	}
}

// runGitCmdWithEnv runs `git <args...>` in dir with the given process
// environment (see the integration test above for why this must not
// silently inherit whatever `specscore` the host machine has on PATH) and
// returns combined stdout+stderr. Fails the test on a non-zero exit.
func runGitCmdWithEnv(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s (in %s): %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return string(out)
}

// buildSpecscoreBinaryForTest compiles the specscore CLI once per test
// binary run and returns the path to the executable. The integration test
// needs a real `specscore` on PATH because it is `git merge` (a separate
// process) that invokes the merge driver, not this test's own process.
func buildSpecscoreBinaryForTest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	binPath := filepath.Join(dir, "specscore")
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/specscore")
	cmd.Dir = repoRootForTest(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("building specscore for integration test: %v\n%s", err, out)
	}
	return binPath
}

// repoRootForTest returns this module's root (two directories up from
// internal/cli).
func repoRootForTest(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(filepath.Dir(wd))
}

// runSpecscoreCmd runs the freshly built specscore binary with dir as its
// working directory (mirroring how a developer would run it from a
// checkout) and fails the test on a non-zero exit.
func runSpecscoreCmd(t *testing.T, bin, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	// Prepend dir's own binary-less PATH entry so `specscore` (the driver
	// command git config now names) also resolves to this build during the
	// integration test, matching a real install where the CLI is on PATH.
	binDir := filepath.Dir(bin)
	cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s (in %s): %v\n%s", bin, strings.Join(args, " "), dir, err, out)
	}
	return string(out)
}
