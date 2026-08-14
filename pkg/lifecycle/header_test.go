package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeHeaderFixture(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return path
}

// Empty/whitespace successor is a no-op.
func TestSetSupersededBy_EmptyIsNoOp(t *testing.T) {
	t.Parallel()
	body := "# Plan: X\n\n**Status:** Approved\n**Supersedes:** —\n"
	path := writeHeaderFixture(t, body)
	orig, wrote, err := SetSupersededBy(path, "   ")
	if err != nil || wrote || orig != nil {
		t.Fatalf("expected no-op, got orig=%v wrote=%v err=%v", orig, wrote, err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != body {
		t.Errorf("file changed:\n%s", got)
	}
}

// Insert immediately after the Supersedes line.
func TestSetSupersededBy_InsertAfterSupersedes(t *testing.T) {
	t.Parallel()
	body := "# Plan: X\n\n**Status:** Approved\n**Supersedes:** —\n\n## Summary\n"
	path := writeHeaderFixture(t, body)
	orig, wrote, err := SetSupersededBy(path, "auth-v2")
	if err != nil || !wrote {
		t.Fatalf("SetSupersededBy: wrote=%v err=%v", wrote, err)
	}
	if string(orig) != body {
		t.Errorf("original bytes mismatch")
	}
	got, _ := os.ReadFile(path)
	want := "# Plan: X\n\n**Status:** Approved\n**Supersedes:** —\n**Superseded By:** auth-v2\n\n## Summary\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestSetSupersededBy_DecisionHeader(t *testing.T) {
	t.Parallel()
	body := "# Decision: Replace storage\n\n**Status:** Approved\n**Supersedes:** —\n\n## Context\n"
	path := writeHeaderFixture(t, body)
	if _, wrote, err := SetSupersededBy(path, "0002-new-storage"); err != nil {
		t.Fatalf("SetSupersededBy: %v", err)
	} else if !wrote {
		t.Fatal("SetSupersededBy did not write the Decision header")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "# Decision: Replace storage\n\n**Status:** Approved\n**Supersedes:** —\n**Superseded By:** 0002-new-storage\n\n## Context\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// No Supersedes line → insert after the Status line.
func TestSetSupersededBy_InsertAfterStatus(t *testing.T) {
	t.Parallel()
	body := "# Plan: X\n\n**Status:** Approved\n\n## Summary\n"
	path := writeHeaderFixture(t, body)
	if _, _, err := SetSupersededBy(path, "auth-v2"); err != nil {
		t.Fatalf("SetSupersededBy: %v", err)
	}
	got, _ := os.ReadFile(path)
	want := "# Plan: X\n\n**Status:** Approved\n**Superseded By:** auth-v2\n\n## Summary\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestSetSupersededByBytes_FailsClosedOutsideCanonicalHeader(t *testing.T) {
	t.Parallel()
	if _, wrote, err := SetSupersededByBytes([]byte("# Notes\n\n**Status:** Approved\n"), "auth-v2"); !errors.Is(err, ErrStatusLineNotFound) || wrote {
		t.Fatalf("non-lifecycle document: wrote=%v err=%v", wrote, err)
	}

	body := []byte("# Plan: X\n\n**Status:** Approved\n```markdown\n**Superseded By:** example-only\n```\n")
	got, wrote, err := SetSupersededByBytes(body, "auth-v2")
	if err != nil || !wrote {
		t.Fatalf("fenced example: wrote=%v err=%v", wrote, err)
	}
	want := "# Plan: X\n\n**Status:** Approved\n**Superseded By:** auth-v2\n```markdown\n**Superseded By:** example-only\n```\n"
	if string(got) != want {
		t.Fatalf("fenced example changed instead of canonical header:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestCanonicalLifecycleHeaderHelpers_DefensiveBranches(t *testing.T) {
	t.Parallel()
	lines := splitKeepTerminators([]byte("# Plan: X\n```markdown\n**Status:** example\n**Supersedes:** example\n```\n"))
	structure := StructuralMarkdownMask(lines, "")
	if got := findSupersedesLineIndexInHeader(lines, structure, -1, len(lines)); got != -1 {
		t.Errorf("negative Supersedes range start = %d, want -1", got)
	}
	if got := findStatusLineIndexInHeader(lines, structure, -1, len(lines)); got != -1 {
		t.Errorf("negative Status range start = %d, want -1", got)
	}
	if got := findSupersedesLineIndexInHeader(lines, structure, 1, len(lines)); got != -1 {
		t.Errorf("fenced Supersedes index = %d, want -1", got)
	}
	if got := findStatusLineIndexInHeader(lines, structure, 1, len(lines)); got != -1 {
		t.Errorf("fenced Status index = %d, want -1", got)
	}

	setextLines := splitKeepTerminators([]byte("Notes\n  ===\n# Plan: Hidden\n"))
	setextStructure := StructuralMarkdownMask(setextLines, "")
	if start, end := canonicalLifecycleHeaderRange(setextLines, setextStructure); start != -1 || end != -1 {
		t.Errorf("Setext prelude range = [%d,%d), want [-1,-1)", start, end)
	}
	if !isATXH1("   # Plan: Indented") {
		t.Error("three-space ATX H1 was not recognized")
	}
	if isSetextH1Line([]string{"Notes", "    ===="}, []bool{true, true}, 1) {
		t.Error("indented code was recognized as a Setext H1")
	}
}

// An existing **Superseded By:** line is rewritten in place, preserving
// indentation and trailing whitespace.
func TestSetSupersededBy_RewriteExisting(t *testing.T) {
	t.Parallel()
	body := "# Plan: X\n\n**Status:** Approved\n**Superseded By:** old-plan  \n"
	path := writeHeaderFixture(t, body)
	if _, _, err := SetSupersededBy(path, "auth-v2"); err != nil {
		t.Fatalf("SetSupersededBy: %v", err)
	}
	got, _ := os.ReadFile(path)
	want := "# Plan: X\n\n**Status:** Approved\n**Superseded By:** auth-v2  \n"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// CRLF line endings are preserved on insertion.
func TestSetSupersededBy_CRLFPreserved(t *testing.T) {
	t.Parallel()
	body := "# Plan: X\r\n\r\n**Status:** Approved\r\n**Supersedes:** —\r\n"
	path := writeHeaderFixture(t, body)
	if _, _, err := SetSupersededBy(path, "auth-v2"); err != nil {
		t.Fatalf("SetSupersededBy: %v", err)
	}
	got, _ := os.ReadFile(path)
	want := "# Plan: X\r\n\r\n**Status:** Approved\r\n**Supersedes:** —\r\n**Superseded By:** auth-v2\r\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A status line with no terminator (EOF) still gets a sane inserted line.
func TestSetSupersededBy_NoTerminatorAnchor(t *testing.T) {
	t.Parallel()
	body := "# Plan: X\n\n**Status:** Approved"
	path := writeHeaderFixture(t, body)
	if _, _, err := SetSupersededBy(path, "auth-v2"); err != nil {
		t.Fatalf("SetSupersededBy: %v", err)
	}
	got, _ := os.ReadFile(path)
	want := "# Plan: X\n\n**Status:** Approved\n**Superseded By:** auth-v2\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// No Status and no Supersedes anchor → ErrStatusLineNotFound, file untouched.
func TestSetSupersededBy_NoAnchor(t *testing.T) {
	t.Parallel()
	body := "# Plan: X\n\nNo header fields.\n"
	path := writeHeaderFixture(t, body)
	_, wrote, err := SetSupersededBy(path, "auth-v2")
	if !errors.Is(err, ErrStatusLineNotFound) {
		t.Fatalf("expected ErrStatusLineNotFound, got %v", err)
	}
	if wrote {
		t.Error("wrote should be false")
	}
	got, _ := os.ReadFile(path)
	if string(got) != body {
		t.Errorf("file changed:\n%s", got)
	}
}

// Read failure: a missing file surfaces the os error.
func TestSetSupersededBy_ReadError(t *testing.T) {
	t.Parallel()
	_, _, err := SetSupersededBy(filepath.Join(t.TempDir(), "nope.md"), "auth-v2")
	if err == nil {
		t.Fatal("expected read error, got nil")
	}
}

// Write failure on the rewrite-in-place path: a read-only directory makes the
// atomic write fail.
func TestSetSupersededBy_RewriteWriteError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("read-only directory is not enforced for root")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.md")
	if err := os.WriteFile(path, []byte("# Plan: X\n\n**Status:** Approved\n**Superseded By:** old\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if _, _, err := SetSupersededBy(path, "auth-v2"); err == nil {
		t.Fatal("expected write error, got nil")
	}
}

// Write failure on the insertion path: read-only directory.
func TestSetSupersededBy_InsertWriteError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("read-only directory is not enforced for root")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.md")
	if err := os.WriteFile(path, []byte("# Plan: X\n\n**Status:** Approved\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if _, _, err := SetSupersededBy(path, "auth-v2"); err == nil {
		t.Fatal("expected write error, got nil")
	}
}

// findSupersedesLineIndex returns -1 when absent (covered indirectly by the
// after-Status case, exercised directly here for clarity).
func TestFindSupersedesLineIndex(t *testing.T) {
	t.Parallel()
	lines := splitKeepTerminators([]byte("# Plan: X\n\n**Status:** Approved\n**Supersedes:** —\n"))
	if got := findSupersedesLineIndex(lines); got != 3 {
		t.Errorf("got %d, want 3", got)
	}
	none := splitKeepTerminators([]byte("# Plan: X\n\n**Status:** Approved\n"))
	if got := findSupersedesLineIndex(none); got != -1 {
		t.Errorf("got %d, want -1", got)
	}
}

// --- SetSupersedes ---
//
// Mirrors the SetSupersededBy suite above; SetSupersedes has a single anchor
// (Status) rather than SetSupersededBy's dual anchor (Supersedes, else
// Status), since a Decision's `**Supersedes:**` field is what's being
// written here — there is no earlier field to prefer inserting after.

// Empty/whitespace target is a no-op.
func TestSetSupersedes_EmptyIsNoOp(t *testing.T) {
	t.Parallel()
	body := "# Decision: X\n\n**Status:** Approved\n**Supersedes:** —\n"
	path := writeHeaderFixture(t, body)
	orig, wrote, err := SetSupersedes(path, "   ")
	if err != nil || wrote || orig != nil {
		t.Fatalf("expected no-op, got orig=%v wrote=%v err=%v", orig, wrote, err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != body {
		t.Errorf("file changed:\n%s", got)
	}
}

// An existing **Supersedes:** line is rewritten in place, preserving
// indentation and trailing whitespace — the common case, since a Decision's
// scaffold always emits the field (defaulted to "—").
func TestSetSupersedes_RewriteExisting(t *testing.T) {
	t.Parallel()
	body := "**Status:** Approved\n**Supersedes:** —  \n"
	path := writeHeaderFixture(t, body)
	orig, wrote, err := SetSupersedes(path, "0001-old")
	if err != nil || !wrote {
		t.Fatalf("SetSupersedes: wrote=%v err=%v", wrote, err)
	}
	if string(orig) != body {
		t.Errorf("original bytes mismatch")
	}
	got, _ := os.ReadFile(path)
	want := "**Status:** Approved\n**Supersedes:** 0001-old  \n"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// No Supersedes line → insert after the Status line.
func TestSetSupersedes_InsertAfterStatus(t *testing.T) {
	t.Parallel()
	body := "# Decision: X\n\n**Status:** Approved\n\n## Summary\n"
	path := writeHeaderFixture(t, body)
	if _, _, err := SetSupersedes(path, "0001-old"); err != nil {
		t.Fatalf("SetSupersedes: %v", err)
	}
	got, _ := os.ReadFile(path)
	want := "# Decision: X\n\n**Status:** Approved\n**Supersedes:** 0001-old\n\n## Summary\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// CRLF line endings are preserved on insertion.
func TestSetSupersedes_CRLFPreserved(t *testing.T) {
	t.Parallel()
	body := "**Status:** Approved\r\n"
	path := writeHeaderFixture(t, body)
	if _, _, err := SetSupersedes(path, "0001-old"); err != nil {
		t.Fatalf("SetSupersedes: %v", err)
	}
	got, _ := os.ReadFile(path)
	want := "**Status:** Approved\r\n**Supersedes:** 0001-old\r\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A status line with no terminator (EOF) still gets a sane inserted line.
func TestSetSupersedes_NoTerminatorAnchor(t *testing.T) {
	t.Parallel()
	body := "**Status:** Approved"
	path := writeHeaderFixture(t, body)
	if _, _, err := SetSupersedes(path, "0001-old"); err != nil {
		t.Fatalf("SetSupersedes: %v", err)
	}
	got, _ := os.ReadFile(path)
	want := "**Status:** Approved\n**Supersedes:** 0001-old\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// No Status anchor → ErrStatusLineNotFound, file untouched.
func TestSetSupersedes_NoAnchor(t *testing.T) {
	t.Parallel()
	body := "# Decision: X\n\nNo header fields.\n"
	path := writeHeaderFixture(t, body)
	_, wrote, err := SetSupersedes(path, "0001-old")
	if !errors.Is(err, ErrStatusLineNotFound) {
		t.Fatalf("expected ErrStatusLineNotFound, got %v", err)
	}
	if wrote {
		t.Error("wrote should be false")
	}
	got, _ := os.ReadFile(path)
	if string(got) != body {
		t.Errorf("file changed:\n%s", got)
	}
}

// Read failure: a missing file surfaces the os error.
func TestSetSupersedes_ReadError(t *testing.T) {
	t.Parallel()
	_, _, err := SetSupersedes(filepath.Join(t.TempDir(), "nope.md"), "0001-old")
	if err == nil {
		t.Fatal("expected read error, got nil")
	}
}

// Write failure on the rewrite-in-place path: a read-only directory makes the
// atomic write fail.
func TestSetSupersedes_RewriteWriteError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("read-only directory is not enforced for root")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "decision.md")
	if err := os.WriteFile(path, []byte("**Status:** Approved\n**Supersedes:** —\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if _, _, err := SetSupersedes(path, "0001-old"); err == nil {
		t.Fatal("expected write error, got nil")
	}
}

// Write failure on the insertion path: read-only directory.
func TestSetSupersedes_InsertWriteError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("read-only directory is not enforced for root")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "decision.md")
	if err := os.WriteFile(path, []byte("**Status:** Approved\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if _, _, err := SetSupersedes(path, "0001-old"); err == nil {
		t.Fatal("expected write error, got nil")
	}
}
