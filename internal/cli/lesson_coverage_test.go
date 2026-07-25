package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/lesson"
)

// TestLessonFieldValue_AllFields exercises every branch of lessonFieldValue,
// including the default case that is unreachable through the validated CLI
// path (parseLessonFields rejects unknown names before lessonFieldValue
// runs).
func TestLessonFieldValue_AllFields(t *testing.T) {
	l := &lesson.Lesson{
		Status:   "  Stated  ",
		Recurred: 3,
		Date:     "2026-07-25",
		Owner:    "alex",
	}
	cases := map[string]string{
		"status":   "Stated", // trimmed
		"recurred": "3",
		"date":     "2026-07-25",
		"owner":    "alex",
		"bogus":    "", // default branch
	}
	for field, want := range cases {
		if got := lessonFieldValue(l, field); got != want {
			t.Errorf("lessonFieldValue(%q) = %q, want %q", field, got, want)
		}
	}
}

// TestParseLessonFields_SkipsEmptyAndDedups covers the empty-part skip and
// the dedup (already-seen) branches of parseLessonFields.
func TestParseLessonFields_SkipsEmptyAndDedups(t *testing.T) {
	got, err := parseLessonFields("status, ,status,recurred")
	if err != nil {
		t.Fatalf("parseLessonFields: %v", err)
	}
	want := []string{"status", "recurred"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("parseLessonFields = %v, want %v", got, want)
	}
}

// TestLessonList_ProjectResolveError covers runLessonList's resolveLessonsDir
// error branch.
func TestLessonList_ProjectResolveError(t *testing.T) {
	bare := t.TempDir() // no spec/ subtree
	_, _, err := runLesson(t, "list", "--project", bare)
	if err == nil {
		t.Fatal("expected error when lessons dir cannot be resolved")
	}
}

// TestLessonList_DiscoverError covers runLessonList's lesson.Discover error
// branch by making spec/lessons a regular file so os.ReadDir fails inside
// Discover.
func TestLessonList_DiscoverError(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "spec", "features"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "spec", "lessons"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	withCwd(t, root)

	_, _, err := runLesson(t, "list")
	if err == nil {
		t.Fatal("expected error when lessons dir is unreadable")
	}
	if !strings.Contains(err.Error(), "discovering lessons") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestLessonList_FieldsStatusFilterExcludes covers the matched()==false
// continue branch inside the --fields output path.
func TestLessonList_FieldsStatusFilterExcludes(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)
	writeLessonInDir(t, lessonsDir, "recorded-one", "Recorded")
	writeLessonInDir(t, lessonsDir, "stated-one", "Stated")

	stdout, _, err := runLesson(t, "list", "--fields", "status", "--status", "Recorded")
	if err != nil {
		t.Fatalf("lesson list --fields status --status Recorded: %v", err)
	}
	if !strings.Contains(stdout, "slug: recorded-one") {
		t.Errorf("expected recorded-one in output: %s", stdout)
	}
	if strings.Contains(stdout, "stated-one") {
		t.Errorf("stated-one should be filtered out: %s", stdout)
	}
}

// TestLessonList_FieldsYAMLEncodeError covers the yaml.Encode error branch of
// the --fields path using a writer that always fails.
func TestLessonList_FieldsYAMLEncodeError(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)
	writeLessonInDir(t, lessonsDir, "any", "Recorded")

	cmd := lessonCommand()
	cmd.SetOut(&errWriter{})
	cmd.SetErr(&errWriter{})
	cmd.SetArgs([]string{"list", "--fields", "status"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when writer fails during yaml encode")
	}
}

// TestLessonList_YAMLEncodeError covers the yaml.Encode error branch of the
// non-fields path using a failing writer.
func TestLessonList_YAMLEncodeError(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)
	writeLessonInDir(t, lessonsDir, "any", "Recorded")

	cmd := lessonCommand()
	cmd.SetOut(&errWriter{})
	cmd.SetErr(&errWriter{})
	cmd.SetArgs([]string{"list", "--format", "yaml"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when writer fails during yaml encode")
	}
}

// TestLessonInfo_YAMLEncodeError covers runLessonInfo's yaml.Encode error
// branch.
func TestLessonInfo_YAMLEncodeError(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)
	writeLessonInDir(t, lessonsDir, "any", "Recorded")

	cmd := lessonCommand()
	cmd.SetOut(&errWriter{})
	cmd.SetErr(&errWriter{})
	cmd.SetArgs([]string{"info", "any"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when writer fails during yaml encode")
	}
}

// TestLessonInfo_ParseError covers runLessonInfo's lesson.Parse error branch:
// the lesson file exists (so resolveLessonPath's stat succeeds) but is
// unreadable, so Parse's os.Open fails. chmod-based, so skip when running as
// root.
func TestLessonInfo_ParseError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping as root: chmod won't restrict access")
	}
	lessonsDir := setupLessonsSpec(t)
	writeLessonInDir(t, lessonsDir, "locked", "Recorded")
	locked := filepath.Join(lessonsDir, "locked.md")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o644) })

	_, _, err := runLesson(t, "info", "locked")
	if err == nil {
		t.Fatal("expected error when lesson file is unreadable")
	}
	if !strings.Contains(err.Error(), "parsing lesson") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestLessonInfo_ProjectResolveError covers runLessonInfo's resolveLessonsDir
// error branch.
func TestLessonInfo_ProjectResolveError(t *testing.T) {
	bare := t.TempDir() // no spec/ subtree
	_, _, err := runLesson(t, "info", "any", "--project", bare)
	if err == nil {
		t.Fatal("expected error when lessons dir cannot be resolved")
	}
}

// TestLessonInfo_FormatText_SupersededByShown covers the SupersededBy-present
// branch of writeLessonInfoText.
func TestLessonInfo_FormatText_SupersededByShown(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)
	writeLessonRaw(t, lessonsDir, "old",
		"# Lesson: Old\n\n**Status:** Superseded\n**Superseded By:** new-lesson\n\n"+
			"## Incident\n\nx\n\n## Process gap\n\nx\n\n## Check\n\nx\n\n## Enforcement\n\nx\n")

	stdout, _, err := runLesson(t, "info", "old", "--format", "text")
	if err != nil {
		t.Fatalf("lesson info --format text: %v", err)
	}
	if !strings.Contains(stdout, "Superseded By:  new-lesson") {
		t.Errorf("text output missing Superseded By: %s", stdout)
	}
}
