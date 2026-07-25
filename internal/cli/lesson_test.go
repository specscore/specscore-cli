package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// runLesson invokes the `lesson` cobra command tree in-process and captures
// stdout, stderr, and the returned error.
func runLesson(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := lessonCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// setupLessonsSpec stages a minimal spec repo with a spec/lessons/ directory
// at a fresh t.TempDir and chdirs into it, mirroring setupPlansSpec.
func setupLessonsSpec(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "spec", "features"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	lessonsDir := filepath.Join(root, "spec", "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	withCwd(t, root)
	return lessonsDir
}

// writeLessonInDir writes a single-file lesson at lessonsDir/<slug>.md with
// the given status and all four required sections present.
func writeLessonInDir(t *testing.T, lessonsDir, slug, status string) {
	t.Helper()
	content := "# Lesson: " + slug + "\n\n**Status:** " + status +
		"\n\n## Incident\n\nx\n\n## Process gap\n\nx\n\n## Check\n\nx\n\n## Enforcement\n\nx\n"
	if err := os.WriteFile(filepath.Join(lessonsDir, slug+".md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write lesson %s: %v", slug, err)
	}
}

// writeLessonRaw writes the given content verbatim to lessonsDir/<slug>.md.
func writeLessonRaw(t *testing.T, lessonsDir, slug, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(lessonsDir, slug+".md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write lesson %s: %v", slug, err)
	}
}

func TestLessonGroup_NoSubcommand_PrintsHelpExitsZero(t *testing.T) {
	out, _, err := runLesson(t)
	if err != nil {
		t.Fatalf("expected nil error (exit 0), got %v", err)
	}
	if !strings.Contains(out, "Record and query process-gap lessons") {
		t.Errorf("expected help text describing the lesson group, got: %q", out)
	}
}

func TestResolveLessonPath_MissingSlug_Exits3(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)
	_, err := resolveLessonPath(lessonsDir, "does-not-exist")
	if got := exitCodeOfErr(err); got != exitcode.NotFound {
		t.Errorf("exit = %d, want %d", got, exitcode.NotFound)
	}
}

func TestResolveLessonsDir_ProjectResolveError(t *testing.T) {
	bare := t.TempDir() // no spec/ subtree
	_, err := resolveLessonsDir(bare)
	if err == nil {
		t.Fatal("expected resolveSpecRoot error, got nil")
	}
}
