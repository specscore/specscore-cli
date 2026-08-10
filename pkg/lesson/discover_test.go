package lesson

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscover_SelectsLessonsSortedSkipsReadmeAndNonLessons(t *testing.T) {
	dir := t.TempDir()
	lessonsDir := filepath.Join(dir, "lessons")

	body := func(title string) string {
		return "# Lesson: " + title + "\n\n**Status:** Recorded\n\n## Incident\n\nx\n"
	}
	// Two real lessons, intentionally written out of alphabetical order.
	writeLesson(t, lessonsDir, "zebra", body("Zebra"))
	writeLesson(t, lessonsDir, "alpha", body("Alpha"))
	// README.md — excluded by IsSingleFileLessonPath.
	writeLesson(t, lessonsDir, "README", "# Lessons index\n")
	// A .md file that is not a Lesson (no `# Lesson:` title) — Parse keeps
	// HasLessonTitle == false, so Discover drops it.
	writeLesson(t, lessonsDir, "notes", "# Notes\n\nNot a lesson.\n")

	got, err := Discover(lessonsDir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 lessons, got %d: %+v", len(got), got)
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
	// A regular file (not a directory) at lessonsDir makes os.ReadDir return a
	// non-IsNotExist error, which Discover propagates.
	dir := t.TempDir()
	notDir := filepath.Join(dir, "lessons")
	if err := os.WriteFile(notDir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(notDir); err == nil {
		t.Fatal("expected error when lessonsDir is a regular file")
	}
}

func TestDiscover_ParseError(t *testing.T) {
	dir := t.TempDir()
	lessonsDir := filepath.Join(dir, "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A dangling symlink named *.md is a direct-child candidate whose Parse()
	// (os.Open) fails because its target does not exist.
	link := filepath.Join(lessonsDir, "dangling.md")
	if err := os.Symlink(filepath.Join(dir, "nonexistent-target"), link); err != nil {
		t.Skipf("symlinks unsupported on this platform: %v", err)
	}
	if _, err := Discover(lessonsDir); err == nil {
		t.Fatal("expected Parse error for dangling symlink candidate")
	}
}

func TestDiscover_AcceptsCanonicalDirectoryForm(t *testing.T) {
	dir := t.TempDir()
	lessonsDir := filepath.Join(dir, "lessons")
	writeLesson(t, lessonsDir, "single", "# Lesson: Single\n\n## Incident\n\nx\n")
	// A canonical directory-form Lesson is discovered beside compatibility files.
	if err := os.MkdirAll(filepath.Join(lessonsDir, "dirform"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lessonsDir, "dirform", "README.md"), []byte("# Lesson: Dir\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Discover(lessonsDir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 2 || got[0].Slug != "dirform" || got[1].Slug != "single" {
		t.Fatalf("expected [dirform, single], got %+v", got)
	}
}
