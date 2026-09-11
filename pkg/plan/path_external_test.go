package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveFileRejectsSymlinkAndAmbiguousForms(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("# Plan: Outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveFile(root, "escape"); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("symlink error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "both.md"), []byte("# Plan: Flat\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "both"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "both", "README.md"), []byte("# Plan: Directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveFile(root, "both"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguity error = %v", err)
	}
}

func TestValidateWritePathRejectsSymlinkedAncestor(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "parent")); err != nil {
		t.Fatal(err)
	}
	target, err := PathForID(root, "parent/child")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateWritePath(root, target); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("write containment error = %v", err)
	}
}

func TestValidateIDAllowsRecursiveNestedPlans(t *testing.T) {
	if err := ValidateID("roadmap/child"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateID("roadmap/child/grandchild"); err != nil {
		t.Fatal(err)
	}
}
