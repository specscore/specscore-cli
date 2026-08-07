package decision

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- removeActiveIndexRow ---

func TestRemoveActiveIndexRow_RemovesMatchingRow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")
	body := activeIndexBody(
		[4]string{"0001", "0001-old", "Old", "Approved"},
		[4]string{"0002", "0002-new", "New", "Approved"},
	)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	orig, changed, err := removeActiveIndexRow(path, "0001-old")
	if err != nil || !changed {
		t.Fatalf("removeActiveIndexRow: changed=%v err=%v", changed, err)
	}
	if string(orig) != body {
		t.Errorf("original bytes mismatch")
	}
	got, _ := os.ReadFile(path)
	if strings.Contains(string(got), "0001-old") {
		t.Errorf("row not removed:\n%s", got)
	}
	if !strings.Contains(string(got), "0002-new") {
		t.Errorf("unrelated row removed:\n%s", got)
	}
}

func TestRemoveActiveIndexRow_MissingFile(t *testing.T) {
	orig, changed, err := removeActiveIndexRow(filepath.Join(t.TempDir(), "nope.md"), "0001-old")
	if err != nil || changed || orig != nil {
		t.Fatalf("expected no-op, got orig=%v changed=%v err=%v", orig, changed, err)
	}
}

func TestRemoveActiveIndexRow_ReadError(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "README.md"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, _, err := removeActiveIndexRow(filepath.Join(dir, "README.md"), "0001-old")
	if err == nil {
		t.Fatal("expected read error, got nil")
	}
}

func TestRemoveActiveIndexRow_NoMatchingRow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")
	body := activeIndexBody([4]string{"0002", "0002-new", "New", "Approved"})
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	orig, changed, err := removeActiveIndexRow(path, "0001-old")
	if err != nil || changed || orig != nil {
		t.Fatalf("expected no-op, got orig=%v changed=%v err=%v", orig, changed, err)
	}
}

func TestRemoveActiveIndexRow_WriteError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")
	body := activeIndexBody([4]string{"0001", "0001-old", "Old", "Approved"})
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	orig := osWriteFileFn
	osWriteFileFn = func(string, []byte, os.FileMode) error { return errors.New("write boom") }
	t.Cleanup(func() { osWriteFileFn = orig })

	_, _, err := removeActiveIndexRow(path, "0001-old")
	if err == nil {
		t.Fatal("expected write error, got nil")
	}
}

// --- appendArchivedIndexEntry ---

func TestAppendArchivedIndexEntry_ReplacesPlaceholder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")
	if err := os.WriteFile(path, []byte(archivedDecisionsIndexStub), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	orig, err := appendArchivedIndexEntry(path, "2026-08-01", "0001-old", "Rejected", "no longer needed")
	if err != nil {
		t.Fatalf("appendArchivedIndexEntry: %v", err)
	}
	if string(orig) != archivedDecisionsIndexStub {
		t.Errorf("original bytes mismatch")
	}
	got, _ := os.ReadFile(path)
	if strings.Contains(string(got), "_No archived decisions yet._") {
		t.Errorf("placeholder should be replaced:\n%s", got)
	}
	if !strings.Contains(string(got), "- 2026-08-01 — [0001-old](0001-old.md) — Rejected — no longer needed") {
		t.Errorf("entry not written:\n%s", got)
	}
	if !strings.Contains(string(got), "## Open Questions") {
		t.Errorf("OQ section should be preserved:\n%s", got)
	}
}

func TestAppendArchivedIndexEntry_InsertsChronologically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")
	seed := "# Archived Decisions\n\n" +
		"- 2026-08-05 — [0002-later](0002-later.md) — Deprecated — later reason\n\n" +
		"## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := appendArchivedIndexEntry(path, "2026-08-01", "0001-earlier", "Rejected", "earlier reason"); err != nil {
		t.Fatalf("appendArchivedIndexEntry: %v", err)
	}
	got, _ := os.ReadFile(path)
	earlierIdx := strings.Index(string(got), "0001-earlier")
	laterIdx := strings.Index(string(got), "0002-later")
	if earlierIdx < 0 || laterIdx < 0 || earlierIdx > laterIdx {
		t.Errorf("entries not in chronological order:\n%s", got)
	}
}

// Two entries sharing the same date break the tie by slug.
func TestAppendArchivedIndexEntry_SameDateBreaksTieBySlug(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")
	seed := "# Archived Decisions\n\n" +
		"- 2026-08-01 — [0002-zeta](0002-zeta.md) — Deprecated — z reason\n\n" +
		"## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := appendArchivedIndexEntry(path, "2026-08-01", "0001-alpha", "Rejected", "a reason"); err != nil {
		t.Fatalf("appendArchivedIndexEntry: %v", err)
	}
	got, _ := os.ReadFile(path)
	alphaIdx := strings.Index(string(got), "0001-alpha")
	zetaIdx := strings.Index(string(got), "0002-zeta")
	if alphaIdx < 0 || zetaIdx < 0 || alphaIdx > zetaIdx {
		t.Errorf("same-date entries should be ordered by slug (alpha before zeta):\n%s", got)
	}
}

// No "## Open Questions" heading present — the entries region runs to EOF.
func TestAppendArchivedIndexEntry_NoOpenQuestionsHeading(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")
	seed := "# Archived Decisions\n\n_No archived decisions yet._\n"
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := appendArchivedIndexEntry(path, "2026-08-01", "0001-old", "Rejected", "reason"); err != nil {
		t.Fatalf("appendArchivedIndexEntry: %v", err)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "[0001-old](0001-old.md)") {
		t.Errorf("entry not written:\n%s", got)
	}
}

func TestAppendArchivedIndexEntry_ReadError(t *testing.T) {
	_, err := appendArchivedIndexEntry(filepath.Join(t.TempDir(), "nope.md"), "2026-08-01", "0001-old", "Rejected", "reason")
	if err == nil {
		t.Fatal("expected read error, got nil")
	}
}

func TestAppendArchivedIndexEntry_WriteError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")
	if err := os.WriteFile(path, []byte(archivedDecisionsIndexStub), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	orig := osWriteFileFn
	osWriteFileFn = func(string, []byte, os.FileMode) error { return errors.New("write boom") }
	t.Cleanup(func() { osWriteFileFn = orig })

	_, err := appendArchivedIndexEntry(path, "2026-08-01", "0001-old", "Rejected", "reason")
	if err == nil {
		t.Fatal("expected write error, got nil")
	}
}

// --- EnsureArchivedIndexStub ---

func TestEnsureArchivedIndexStub_CreatesWhenAbsent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "spec", "decisions"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	created, err := EnsureArchivedIndexStub(root)
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	got, err := os.ReadFile(filepath.Join(root, "spec", "decisions", "archived", "README.md"))
	if err != nil {
		t.Fatalf("reading stub: %v", err)
	}
	if string(got) != archivedDecisionsIndexStub {
		t.Errorf("stub content mismatch:\n%s", got)
	}
}

func TestEnsureArchivedIndexStub_NoOpWhenPresent(t *testing.T) {
	root := t.TempDir()
	archivedDir := filepath.Join(root, "spec", "decisions", "archived")
	if err := os.MkdirAll(archivedDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	existing := "custom content\n"
	if err := os.WriteFile(filepath.Join(archivedDir, "README.md"), []byte(existing), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	created, err := EnsureArchivedIndexStub(root)
	if err != nil || created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	got, _ := os.ReadFile(filepath.Join(archivedDir, "README.md"))
	if string(got) != existing {
		t.Errorf("existing content should be untouched:\n%s", got)
	}
}

func TestEnsureArchivedIndexStub_MkdirError(t *testing.T) {
	root := t.TempDir()
	orig := osMkdirAllFn
	osMkdirAllFn = func(string, os.FileMode) error { return errors.New("mkdir boom") }
	t.Cleanup(func() { osMkdirAllFn = orig })

	_, err := EnsureArchivedIndexStub(root)
	if err == nil {
		t.Fatal("expected mkdir error, got nil")
	}
}

func TestEnsureArchivedIndexStub_StatError_NonENOENT(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "spec", "decisions", "archived"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	orig := osStatFn
	osStatFn = func(string) (os.FileInfo, error) { return nil, errors.New("stat boom") }
	t.Cleanup(func() { osStatFn = orig })

	_, err := EnsureArchivedIndexStub(root)
	if err == nil {
		t.Fatal("expected stat error, got nil")
	}
}

func TestEnsureArchivedIndexStub_WriteError(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "spec", "decisions", "archived"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	orig := osWriteFileFn
	osWriteFileFn = func(string, []byte, os.FileMode) error { return errors.New("write boom") }
	t.Cleanup(func() { osWriteFileFn = orig })

	_, err := EnsureArchivedIndexStub(root)
	if err == nil {
		t.Fatal("expected write error, got nil")
	}
}
