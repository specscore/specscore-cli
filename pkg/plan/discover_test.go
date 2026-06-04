package plan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscover_SelectsPlansSortedSkipsReadmeAndNonPlans(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "plans")

	planBody := func(title string) string {
		return "# Plan: " + title + "\n\n**Status:** Draft\n\n## Tasks\n\n### Task 1: x\n\nBody.\n"
	}
	// Two real plans, intentionally written out of alphabetical order.
	writePlan(t, plansDir, "zebra", planBody("Zebra"))
	writePlan(t, plansDir, "alpha", planBody("Alpha"))
	// README.md — excluded by IsSingleFilePlanPath.
	writePlan(t, plansDir, "README", "# Plans index\n")
	// A .md file that is not a Plan (no `# Plan:` title) — Parse keeps
	// HasPlanTitle == false, so Discover drops it.
	writePlan(t, plansDir, "notes", "# Notes\n\nNot a plan.\n")

	got, err := Discover(plansDir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 plans, got %d: %+v", len(got), got)
	}
	if got[0].Slug != "alpha" || got[1].Slug != "zebra" {
		t.Errorf("expected [alpha, zebra], got [%s, %s]", got[0].Slug, got[1].Slug)
	}
}

func TestDiscover_AbsentDirIsEmpty(t *testing.T) {
	got, err := Discover(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("Discover on absent dir: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(got))
	}
}

func TestDiscover_ReadDirError(t *testing.T) {
	// A regular file (not a directory) at plansDir makes os.ReadDir return a
	// non-IsNotExist error, which Discover propagates.
	dir := t.TempDir()
	notDir := filepath.Join(dir, "plans")
	if err := os.WriteFile(notDir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(notDir); err == nil {
		t.Fatal("expected error when plansDir is a regular file")
	}
}

func TestDiscover_ParseError(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A dangling symlink named *.md is a direct-child candidate whose Parse()
	// (os.Open) fails because its target does not exist.
	link := filepath.Join(plansDir, "dangling.md")
	if err := os.Symlink(filepath.Join(dir, "nonexistent-target"), link); err != nil {
		t.Skipf("symlinks unsupported on this platform: %v", err)
	}
	if _, err := Discover(plansDir); err == nil {
		t.Fatal("expected Parse error for dangling symlink candidate")
	}
}

func TestDiscover_SkipsSubdirectories(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "plans")
	writePlan(t, plansDir, "single", "# Plan: Single\n\n## Tasks\n\n### Task 1: x\n\nBody.\n")
	// Directory-form plan: spec/plans/<slug>/README.md — out of scope.
	if err := os.MkdirAll(filepath.Join(plansDir, "dirform"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plansDir, "dirform", "README.md"), []byte("# Plan: Dir\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Discover(plansDir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 1 || got[0].Slug != "single" {
		t.Fatalf("expected only [single], got %+v", got)
	}
}
