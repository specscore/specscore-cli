package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lint"
)

func TestLessonRecur_HappyPath(t *testing.T) {
	root := stageLesson(t, "kinder-fake", "Stated")

	stdout, stderr, err := runLesson(t, "recur", "kinder-fake", "--note", "happened again")
	if err != nil {
		t.Fatalf("lesson recur: %v (stderr=%s)", err, stderr)
	}
	if stdout != "kinder-fake: recurred 1\n" {
		t.Errorf("stdout = %q", stdout)
	}
	body, _ := os.ReadFile(filepath.Join(root, "spec", "lessons", "kinder-fake.md"))
	if !strings.Contains(string(body), "**Recurred:** 1") || !strings.Contains(string(body), "happened again") {
		t.Errorf("recurrence not recorded:\n%s", body)
	}
}

func TestLessonRecur_NoNote(t *testing.T) {
	stageLesson(t, "kinder-fake", "Stated")
	stdout, _, err := runLesson(t, "recur", "kinder-fake")
	if err != nil {
		t.Fatalf("lesson recur: %v", err)
	}
	if stdout != "kinder-fake: recurred 1\n" {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestLessonRecur_MissingSlugExits2(t *testing.T) {
	setupLessonsSpec(t)
	_, _, err := runLesson(t, "recur")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d", got, exitcode.InvalidArgs)
	}
}

func TestLessonRecur_TooManyArgsExits2(t *testing.T) {
	setupLessonsSpec(t)
	_, _, err := runLesson(t, "recur", "a", "b")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d", got, exitcode.InvalidArgs)
	}
}

func TestLessonRecur_InvalidSlugExits2(t *testing.T) {
	setupLessonsSpec(t)
	_, _, err := runLesson(t, "recur", "Bad_Slug")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d", got, exitcode.InvalidArgs)
	}
}

func TestLessonRecur_ProjectResolveError(t *testing.T) {
	bare := t.TempDir()
	_, _, err := runLesson(t, "recur", "kinder-fake", "--project", bare)
	if err == nil {
		t.Fatal("expected resolveSpecRoot error, got nil")
	}
}

func TestLessonRecur_NotFoundExits3(t *testing.T) {
	setupLessonsSpec(t)
	_, _, err := runLesson(t, "recur", "ghost")
	if got := exitCodeOfErr(err); got != exitcode.NotFound {
		t.Errorf("exit = %d, want %d", got, exitcode.NotFound)
	}
}

func TestLessonRecur_RecurFnFails(t *testing.T) {
	stageLesson(t, "kinder-fake", "Stated")
	orig := lessonRecurFn
	lessonRecurFn = func(string, string) (int, error) {
		return 0, errors.New("boom")
	}
	t.Cleanup(func() { lessonRecurFn = orig })

	_, _, err := runLesson(t, "recur", "kinder-fake")
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d", got, exitcode.Unexpected)
	}
}

func TestLessonRecur_LintFixFails(t *testing.T) {
	stageLesson(t, "kinder-fake", "Stated")
	orig := lintLintFn
	lintLintFn = func(lint.Options) ([]lint.Violation, error) {
		return nil, errors.New("fix boom")
	}
	t.Cleanup(func() { lintLintFn = orig })

	_, _, err := runLesson(t, "recur", "kinder-fake")
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d", got, exitcode.Unexpected)
	}
}
