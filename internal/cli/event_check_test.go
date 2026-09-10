package cli

// Tests for `event check --base <git-ref>` — the ledger-monotonicity gate.
// See spec/features/cli/event/check/README.md and the lesson
// an-append-only-ledger-merge-must-prove-the-result-is-not-shorter-than-either-parent.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// assertCheckExitCode fails the test unless err is a non-nil exitcode.Error
// (or wraps one) carrying exactly `code`.
func assertCheckExitCode(t *testing.T, err error, code int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error with exit code %d, got nil", code)
	}
	type exitCoder interface{ ExitCode() int }
	var ec exitCoder
	if !errors.As(err, &ec) {
		t.Fatalf("error %v does not carry an exit code (want %d)", err, code)
	}
	if ec.ExitCode() != code {
		t.Fatalf("exit code = %d, want %d (error: %v)", ec.ExitCode(), code, err)
	}
}

// writeCheckLedger overwrites the project's default JSONL ledger
// (.specscore/events.jsonl) with one JSONL line per uuid, using the minimum
// valid envelope shape event.Validate accepts.
func writeCheckLedger(t *testing.T, projectRoot string, uuids ...string) {
	t.Helper()
	dir := filepath.Join(projectRoot, ".specscore")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, id := range uuids {
		b.WriteString(`{"name":"lesson.observed","version":1,"uuid":"`)
		b.WriteString(id)
		b.WriteString(`","timestamp":"2026-09-09T12:00:00Z","actor":{"kind":"external","id":"t"},"artifact":{"type":"lesson","id":"x","path":"spec/lessons/x/README.md","revision":"uncommitted"},"payload":{}}`)
		b.WriteString("\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newCheckTestRepo initializes a real git repo with an empty specscore.yaml
// (so the default .specscore/events.jsonl sink applies) and returns its
// root. Tests skip themselves when git is unavailable on the host.
func newCheckTestRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "config", "user.email", "t@example.com")
	runGit(t, dir, "config", "user.name", "T")
	if err := os.WriteFile(filepath.Join(dir, "specscore.yaml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

const (
	checkUUID1 = "00000000-0000-4000-8000-000000000001"
	checkUUID2 = "00000000-0000-4000-8000-000000000002"
)

// TestEventCheck_WorkingTreeRetainsAllBaseUUIDs covers AC
// cli/event/check#ac:working-tree-retains-all-base-uuids-passes: the
// working-tree ledger has every UUID present at --base (here, byte-for-byte
// identical) and the command must exit 0.
func TestEventCheck_WorkingTreeRetainsAllBaseUUIDs(t *testing.T) {
	dir := newCheckTestRepo(t)
	writeCheckLedger(t, dir, checkUUID1)
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "base")

	out, errOut, err := runEvent(t, "check", "--project", dir, "--base", "HEAD")
	requireCLISuccess(t, err)
	if !strings.Contains(out, "ok:") {
		t.Fatalf("stdout = %q, want an ok: message", out)
	}
	if errOut != "" {
		t.Fatalf("stderr = %q, want empty on success", errOut)
	}
}

// TestEventCheck_WorkingTreeSupersetOfBase covers the superset case: the
// working tree has every base UUID plus more (the ordinary "an event was
// appended since base" case) and must still exit 0.
func TestEventCheck_WorkingTreeSupersetOfBase(t *testing.T) {
	dir := newCheckTestRepo(t)
	writeCheckLedger(t, dir, checkUUID1)
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "base")
	writeCheckLedger(t, dir, checkUUID1, checkUUID2)

	_, _, err := runEvent(t, "check", "--project", dir, "--base", "HEAD")
	requireCLISuccess(t, err)
}

// TestEventCheck_MissingBaseUUID_FailsNonZero covers AC
// cli/event/check#ac:working-tree-missing-base-uuid-fails-nonzero: the
// working-tree ledger lost an event UUID present at --base. This is the
// exact failure mode of the incident that prompted this command (a bad
// merge silently emptied the ledger).
func TestEventCheck_MissingBaseUUID_FailsNonZero(t *testing.T) {
	dir := newCheckTestRepo(t)
	writeCheckLedger(t, dir, checkUUID1, checkUUID2)
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "base")
	// Simulate the bad merge: the working tree lost checkUUID2.
	writeCheckLedger(t, dir, checkUUID1)

	out, errOut, err := runEvent(t, "check", "--project", dir, "--base", "HEAD")
	requireCLIError(t, err)
	combined := out + errOut
	if !strings.Contains(combined, checkUUID2) {
		t.Fatalf("output = %q, want the missing UUID %q", combined, checkUUID2)
	}
	if strings.Contains(combined, checkUUID1) {
		t.Fatalf("output = %q, must not name the retained UUID %q as missing", combined, checkUUID1)
	}
}

// TestEventCheck_WorkingTreeLedgerFullyAbsent_FailsNonZero covers the case
// where the working-tree ledger file does not exist at all while --base had
// events: every one of them must be reported missing, not silently treated
// as vacuously fine.
func TestEventCheck_WorkingTreeLedgerFullyAbsent_FailsNonZero(t *testing.T) {
	dir := newCheckTestRepo(t)
	writeCheckLedger(t, dir, checkUUID1)
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "base")
	if err := os.Remove(filepath.Join(dir, ".specscore", "events.jsonl")); err != nil {
		t.Fatal(err)
	}

	out, errOut, err := runEvent(t, "check", "--project", dir, "--base", "HEAD")
	requireCLIError(t, err)
	if !strings.Contains(out+errOut, checkUUID1) {
		t.Fatalf("output = %q, want the missing UUID %q", out+errOut, checkUUID1)
	}
}

// TestEventCheck_LedgerAbsentAtBase_TreatedAsEmpty covers the "ledger did
// not exist yet at base" case: --base names a real, resolvable commit that
// predates the ledger file's creation. This MUST be zero base UUIDs, not an
// error.
func TestEventCheck_LedgerAbsentAtBase_TreatedAsEmpty(t *testing.T) {
	dir := newCheckTestRepo(t)
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "no ledger yet")
	writeCheckLedger(t, dir, checkUUID1)

	_, _, err := runEvent(t, "check", "--project", dir, "--base", "HEAD")
	requireCLISuccess(t, err)
}

// TestEventCheck_UnresolvableBase_FailsInvalidArgs covers AC
// cli/event/check#ac:unresolvable-base-fails-clearly.
func TestEventCheck_UnresolvableBase_FailsInvalidArgs(t *testing.T) {
	dir := newCheckTestRepo(t)
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "base")

	_, errOut, err := runEvent(t, "check", "--project", dir, "--base", "does-not-exist-ref")
	assertCheckExitCode(t, err, exitcode.InvalidArgs)
	if !strings.Contains(errOut+err.Error(), "does-not-exist-ref") {
		t.Fatalf("error output = %q / %v, want it to name the unresolved ref", errOut, err)
	}
}

// TestEventCheck_MissingBaseFlag_FailsInvalidArgs asserts --base is
// enforced as required.
func TestEventCheck_MissingBaseFlag_FailsInvalidArgs(t *testing.T) {
	dir := newCheckTestRepo(t)
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "base")

	_, _, err := runEvent(t, "check", "--project", dir)
	assertCheckExitCode(t, err, exitcode.InvalidArgs)
}

// TestEventCheck_Help asserts the verb registers, exits 0 on --help, and
// documents --base (AC cli/event/check#ac:verb-registers-and-helps).
func TestEventCheck_Help(t *testing.T) {
	out, _, err := runEvent(t, "check", "--help")
	requireCLISuccess(t, err)
	if !strings.Contains(out, "--base") {
		t.Fatalf("help output = %q, want it to document --base", out)
	}
}

// TestEventCheck_NoProjectRoot_FailsNotFound asserts the shared
// project-autodetect contract (exit 3) applies when no specscore.yaml is
// found, matching `event merge`'s behavior.
func TestEventCheck_NoProjectRoot_FailsNotFound(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runEvent(t, "check", "--project", dir, "--base", "HEAD")
	assertCheckExitCode(t, err, exitcode.NotFound)
}
