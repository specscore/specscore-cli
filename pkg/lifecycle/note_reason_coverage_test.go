package lifecycle

// Coverage for AppendResolutionNote error/edge branches and NewReasonRequiredSet's
// empty-set path.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppendResolutionNote_EmptyNoteNoOp(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.md")
	if err := os.WriteFile(p, []byte("# t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	orig, wrote, err := AppendResolutionNote(p, "   \n\t ")
	if err != nil || wrote || orig != nil {
		t.Fatalf("empty note must be a no-op, got wrote=%v err=%v", wrote, err)
	}
}

func TestAppendResolutionNote_ReadError(t *testing.T) {
	_, _, err := AppendResolutionNote(filepath.Join(t.TempDir(), "nope.md"), "note")
	if err == nil {
		t.Fatal("expected ReadFile error")
	}
}

func TestAppendResolutionNote_WriteError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.md")
	if err := os.WriteFile(p, []byte("# t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil { // dir not writable → atomic temp write fails
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o755)
	if _, _, err := AppendResolutionNote(p, "a note"); err == nil {
		t.Fatal("expected writeFileAtomic error in read-only dir")
	}
}

func TestAppendResolutionNote_InsertAtTop(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.md")
	// Minimal content so the Resolution section inserts with no preceding
	// blank line to add (exercises ensureLeadingBlank's idx-0 branch path).
	if err := os.WriteFile(p, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := AppendResolutionNote(p, "top note"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if want := "## Resolution"; !contains(string(b), want) {
		t.Fatalf("missing %q in %q", want, string(b))
	}
}

func TestNewReasonRequiredSet_Empty(t *testing.T) {
	s := NewReasonRequiredSet()
	if s.RequiresReason("Queued", "Rejected") {
		t.Fatal("empty set must require no reason")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
