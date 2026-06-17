package lifecycle

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Rewrite — ErrStatusLineNotFound branch (tested in lifecycle_test.go)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Rewrite — file not found
// ---------------------------------------------------------------------------

func TestRewrite_FileNotFound(t *testing.T) {
	_, err := Rewrite(filepath.Join(t.TempDir(), "no-such-file.md"), "Approved")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// ---------------------------------------------------------------------------
// Rollback — file not found
// ---------------------------------------------------------------------------

func TestRollback_FileNotFound(t *testing.T) {
	err := Rollback(filepath.Join(t.TempDir(), "no-such-file.md"), "**Status:** Draft\n")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// ---------------------------------------------------------------------------
// Rollback — no status line (tested in lifecycle_test.go)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// readStatus — file not found (tested in lifecycle_test.go)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// readStatus — scanner error (> 1MB line)
// ---------------------------------------------------------------------------

func TestReadStatus_NoStatusFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.md")
	if err := os.WriteFile(path, []byte("# Title\n\nSomething else.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := readStatus(path)
	if err != ErrStatusLineNotFound {
		t.Errorf("expected ErrStatusLineNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// writeFileAtomic — destination does not exist (Stat fails)
// ---------------------------------------------------------------------------

func TestWriteFileAtomic_DestNotExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent-file")
	err := writeFileAtomic(path, []byte("data"))
	if err == nil {
		t.Fatal("expected error when destination does not exist")
	}
}

// ---------------------------------------------------------------------------
// Rewrite + Rollback happy-path with CRLF line endings
// ---------------------------------------------------------------------------

func TestRewrite_CRLFPreserved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.md")
	content := "# Title\r\n\r\n**Status:** Draft\r\n\r\nBody.\r\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	orig, err := Rewrite(path, "Approved")
	if err != nil {
		t.Fatalf("Rewrite: %v", err)
	}
	if !strings.Contains(orig, "Draft") {
		t.Errorf("original line should contain Draft; got %q", orig)
	}
	// Verify the rewritten file has Approved
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "**Status:** Approved") {
		t.Errorf("rewritten file should contain Approved; got %q", data)
	}
	// CRLF should be preserved
	if !strings.Contains(string(data), "\r\n") {
		t.Error("CRLF should be preserved")
	}

	// Now rollback
	if err := Rollback(path, orig); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	data, _ = os.ReadFile(path)
	if string(data) != content {
		t.Errorf("rollback did not restore original:\n got:  %q\n want: %q", data, content)
	}
}

// ---------------------------------------------------------------------------
// dirOf — various path forms
// ---------------------------------------------------------------------------

func TestDirOf_VariousPaths(t *testing.T) {
	cases := []struct {
		input, want string
	}{
		{"/a/b/c", "/a/b"},
		{"a/b", "a"},
		{"filename", "."},
		{"/root", "/"},
	}
	for _, tc := range cases {
		got := dirOf(tc.input)
		if got != tc.want {
			t.Errorf("dirOf(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// splitKeepTerminators — no trailing newline (tested in lifecycle_test.go)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Validate — file not found
// ---------------------------------------------------------------------------

func TestValidate_FileNotFound(t *testing.T) {
	_, err := Validate(KindIdea, filepath.Join(t.TempDir(), "missing.md"), IdeaApproved)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// ---------------------------------------------------------------------------
// Validate — invalid transition
// ---------------------------------------------------------------------------

func TestValidate_InvalidTransition(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "idea.md")
	if err := os.WriteFile(path, []byte("# Title\n\n**Status:** Stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	from, err := Validate(KindIdea, path, IdeaDraft)
	if err == nil {
		t.Fatal("expected error for illegal transition")
	}
	if from != IdeaStale {
		t.Errorf("from = %q, want %q", from, IdeaStale)
	}
}

// ---------------------------------------------------------------------------
// init() — kindStatuses coverage (validateMatrix and computeStatusUnion)
// ---------------------------------------------------------------------------

func TestKindStatuses_PopulatedByInit(t *testing.T) {
	for _, kind := range []Kind{KindIdea, KindFeature} {
		statuses := kindStatuses[kind]
		if len(statuses) == 0 {
			t.Errorf("kindStatuses[%q] is empty", kind)
		}
	}
}

func TestValidateMatrix_SelfLoop(t *testing.T) {
	rows := []transitionRow{
		{From: "A", To: "B"},
		{From: "C", To: "C"},
	}
	err := validateMatrix(rows)
	if err == nil {
		t.Fatal("expected error for self-loop")
	}
	if !strings.Contains(err.Error(), "self-loop") {
		t.Errorf("error should mention self-loop; got %v", err)
	}
}

// ---------------------------------------------------------------------------
// writeFileAtomic — happy path with existing file (exercises full write path)
// ---------------------------------------------------------------------------

func TestWriteFileAtomic_HappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.md")
	// Create the file first (writeFileAtomic requires dst to exist for Stat)
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Overwrite via writeFileAtomic
	newContent := []byte("new content here\n")
	if err := writeFileAtomic(path, newContent); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}
	// Verify content
	got, _ := os.ReadFile(path)
	if string(got) != string(newContent) {
		t.Errorf("got %q, want %q", got, newContent)
	}
	// Verify mode preserved
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644", info.Mode().Perm())
	}
}

// ---------------------------------------------------------------------------
// writeFileAtomic — read-only directory (CreateTemp fails)
// ---------------------------------------------------------------------------

func TestWriteFileAtomic_ReadOnlyDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.md")
	if err := os.WriteFile(path, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Make directory read-only so CreateTemp fails
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	err := writeFileAtomic(path, []byte("new"))
	if err == nil {
		t.Fatal("expected error when dir is read-only")
	}
}

// ---------------------------------------------------------------------------
// Rewrite — happy path LF
// ---------------------------------------------------------------------------

func TestRewrite_LFHappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.md")
	content := "# Title\n\n**Status:** Draft\n\nBody.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	orig, err := Rewrite(path, "Approved")
	if err != nil {
		t.Fatalf("Rewrite: %v", err)
	}
	if !strings.Contains(orig, "Draft") {
		t.Errorf("original line should contain Draft; got %q", orig)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "**Status:** Approved") {
		t.Errorf("rewritten file should contain Approved; got %q", data)
	}
	// Other lines preserved
	if !strings.Contains(string(data), "# Title") {
		t.Error("title should be preserved")
	}
	if !strings.Contains(string(data), "Body.") {
		t.Error("body should be preserved")
	}
}

// ---------------------------------------------------------------------------
// Rewrite — with indented status line
// ---------------------------------------------------------------------------

func TestRewrite_IndentedStatusLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.md")
	content := "# Title\n\n  **Status:** Under Review\n\nBody.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	orig, err := Rewrite(path, "Approved")
	if err != nil {
		t.Fatalf("Rewrite: %v", err)
	}
	if !strings.Contains(orig, "Under Review") {
		t.Errorf("original should contain 'Under Review'; got %q", orig)
	}
	data, _ := os.ReadFile(path)
	// Indentation should be preserved
	if !strings.Contains(string(data), "  **Status:** Approved") {
		t.Errorf("indented rewrite failed: %q", data)
	}
}

// ---------------------------------------------------------------------------
// Rollback — happy path
// ---------------------------------------------------------------------------

func TestRollback_HappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.md")
	content := "# Title\n\n**Status:** Draft\n\nBody.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	// Rewrite to Approved
	orig, err := Rewrite(path, "Approved")
	if err != nil {
		t.Fatalf("Rewrite: %v", err)
	}
	// Rollback
	if err := Rollback(path, orig); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != content {
		t.Errorf("rollback failed:\n got:  %q\n want: %q", data, content)
	}
}

// ---------------------------------------------------------------------------
// writeFileAtomic — custom permissions are preserved
// ---------------------------------------------------------------------------

func TestWriteFileAtomic_CustomPermPreserved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exec.md")
	if err := os.WriteFile(path, []byte("original"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(path, []byte("new content")); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %v, want 0755", info.Mode().Perm())
	}
	got, _ := os.ReadFile(path)
	if string(got) != "new content" {
		t.Errorf("got %q, want %q", got, "new content")
	}
}

// ---------------------------------------------------------------------------
// writeFileAtomic — large content (exercises io.Copy + Sync fully)
// ---------------------------------------------------------------------------

func TestWriteFileAtomic_LargeContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.md")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 100KB content to ensure Copy/Sync paths are fully exercised.
	large := strings.Repeat("x", 100*1024)
	if err := writeFileAtomic(path, []byte(large)); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}
	got, _ := os.ReadFile(path)
	if len(got) != 100*1024 {
		t.Errorf("expected 100KB, got %d bytes", len(got))
	}
}

// ---------------------------------------------------------------------------
// Rewrite — no trailing newline (tested in lifecycle_test.go)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// writeFileAtomic — injected CreateTemp failure
// ---------------------------------------------------------------------------

func TestWriteFileAtomic_CreateTempError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.md")
	original := []byte("original content")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}

	old := osCreateTemp
	osCreateTemp = func(dir, pattern string) (*os.File, error) {
		return nil, fmt.Errorf("injected createtemp error")
	}
	t.Cleanup(func() { osCreateTemp = old })

	err := writeFileAtomic(path, []byte("new content"))
	if err == nil {
		t.Fatal("expected error from injected CreateTemp failure")
	}
	// Original file should be unchanged.
	got, _ := os.ReadFile(path)
	if string(got) != string(original) {
		t.Errorf("original file changed: got %q, want %q", got, original)
	}
}

// ---------------------------------------------------------------------------
// writeFileAtomic — injected io.Copy failure
// ---------------------------------------------------------------------------

func TestWriteFileAtomic_CopyError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.md")
	original := []byte("original content")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}

	old := ioCopy
	ioCopy = func(dst io.Writer, src io.Reader) (int64, error) {
		return 0, fmt.Errorf("injected copy error")
	}
	t.Cleanup(func() { ioCopy = old })

	err := writeFileAtomic(path, []byte("new content"))
	if err == nil {
		t.Fatal("expected error from injected Copy failure")
	}
	// Original file should be unchanged.
	got, _ := os.ReadFile(path)
	if string(got) != string(original) {
		t.Errorf("original file changed: got %q, want %q", got, original)
	}
}

// ---------------------------------------------------------------------------
// writeFileAtomic — injected os.Chmod failure
// ---------------------------------------------------------------------------

func TestWriteFileAtomic_ChmodError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.md")
	original := []byte("original content")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}

	old := osChmod
	osChmod = func(name string, mode os.FileMode) error {
		return fmt.Errorf("injected chmod error")
	}
	t.Cleanup(func() { osChmod = old })

	err := writeFileAtomic(path, []byte("new content"))
	if err == nil {
		t.Fatal("expected error from injected Chmod failure")
	}
	// Original file should be unchanged.
	got, _ := os.ReadFile(path)
	if string(got) != string(original) {
		t.Errorf("original file changed: got %q, want %q", got, original)
	}
}

// ---------------------------------------------------------------------------
// writeFileAtomic — injected os.Rename failure
// ---------------------------------------------------------------------------

func TestWriteFileAtomic_RenameError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.md")
	original := []byte("original content")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}

	old := osRename
	osRename = func(oldpath, newpath string) error {
		return fmt.Errorf("injected rename error")
	}
	t.Cleanup(func() { osRename = old })

	err := writeFileAtomic(path, []byte("new content"))
	if err == nil {
		t.Fatal("expected error from injected Rename failure")
	}
	// Original file should be unchanged.
	got, _ := os.ReadFile(path)
	if string(got) != string(original) {
		t.Errorf("original file changed: got %q, want %q", got, original)
	}
}

// ---------------------------------------------------------------------------
// writeFileAtomic — injected fileSync failure
// ---------------------------------------------------------------------------

func TestWriteFileAtomic_SyncError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.md")
	original := []byte("original content")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}

	old := fileSync
	fileSync = func(f *os.File) error {
		return fmt.Errorf("injected sync error")
	}
	t.Cleanup(func() { fileSync = old })

	err := writeFileAtomic(path, []byte("new content"))
	if err == nil {
		t.Fatal("expected error from injected Sync failure")
	}
	// Original file should be unchanged.
	got, _ := os.ReadFile(path)
	if string(got) != string(original) {
		t.Errorf("original file changed: got %q, want %q", got, original)
	}
}

// ---------------------------------------------------------------------------
// writeFileAtomic — injected fileClose failure
// ---------------------------------------------------------------------------

func TestWriteFileAtomic_CloseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.md")
	original := []byte("original content")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}

	old := fileClose
	fileClose = func(f *os.File) error {
		_ = f.Close() // actually close so no fd leak
		return fmt.Errorf("injected close error")
	}
	t.Cleanup(func() { fileClose = old })

	err := writeFileAtomic(path, []byte("new content"))
	if err == nil {
		t.Fatal("expected error from injected Close failure")
	}
	// Original file should be unchanged.
	got, _ := os.ReadFile(path)
	if string(got) != string(original) {
		t.Errorf("original file changed: got %q, want %q", got, original)
	}
}

// ---------------------------------------------------------------------------
// readStatus — scanner error (huge line exceeds buffer)
// ---------------------------------------------------------------------------

func TestReadStatus_ScannerError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.md")
	// Create a file with a single line exceeding the 1MB scanner buffer
	// limit. This triggers scanner.Err() != nil.
	huge := strings.Repeat("x", 2*1024*1024) + "\n"
	if err := os.WriteFile(path, []byte(huge), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := readStatus(path)
	if err == nil {
		t.Fatal("expected error for scanner overflow")
	}
	if err == ErrStatusLineNotFound {
		t.Error("expected scanner error, not ErrStatusLineNotFound")
	}
}
