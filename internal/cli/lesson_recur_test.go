package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
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
	occurrences, err := lesson.DiscoverOccurrences(filepath.Join(root, "spec", "lessons", "kinder-fake", "README.md"))
	if err != nil || len(occurrences) != 1 {
		t.Fatalf("expected one immutable occurrence, got %#v, err=%v", occurrences, err)
	}
	body, _ := os.ReadFile(occurrences[0].Path)
	if !strings.Contains(string(body), "happened again") {
		t.Errorf("recurrence summary not recorded: %s", body)
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

// TestLessonRecur_NoWarningOnActiveStatuses covers the non-terminal branch of
// warnIfLessonRetired: Recorded, Stated, and Enforced are all legitimate
// targets for a recurrence and must not print a warning.
func TestLessonRecur_NoWarningOnActiveStatuses(t *testing.T) {
	for _, status := range []string{"Recorded", "Stated", "Enforced"} {
		t.Run(status, func(t *testing.T) {
			stageLesson(t, "kinder-fake", status)
			_, stderr, err := runLesson(t, "recur", "kinder-fake")
			if err != nil {
				t.Fatalf("lesson recur: %v", err)
			}
			if strings.Contains(stderr, "warning") {
				t.Errorf("unexpected warning for status %q: %q", status, stderr)
			}
		})
	}
}

// AC: a recurrence against a retired lesson (Withdrawn or Superseded) exits
// 0 and still records the occurrence — the evidence is worth keeping — but
// warns on stderr rather than succeeding silently, since it is evidence the
// retirement itself was wrong.
func TestLessonRecur_WarnsOnRetiredStatuses(t *testing.T) {
	for _, status := range []string{"Withdrawn", "Superseded"} {
		t.Run(status, func(t *testing.T) {
			root := stageLesson(t, "kinder-fake", status)
			stdout, stderr, err := runLesson(t, "recur", "kinder-fake", "--note", "happened again anyway")
			if err != nil {
				t.Fatalf("lesson recur: %v (stderr=%s)", err, stderr)
			}
			if stdout != "kinder-fake: recurred 1\n" {
				t.Errorf("stdout = %q", stdout)
			}
			if !strings.Contains(stderr, "warning:") || !strings.Contains(stderr, status) {
				t.Errorf("expected a warning naming %q, got stderr=%q", status, stderr)
			}
			occurrences, err := lesson.DiscoverOccurrences(filepath.Join(root, "spec", "lessons", "kinder-fake", "README.md"))
			if err != nil || len(occurrences) != 1 {
				t.Fatalf("recurrence must still be recorded despite warning: %#v, err=%v", occurrences, err)
			}
		})
	}
}

// TestLessonRecur_ParseError covers runLessonRecur's lesson.Parse error
// branch via the lessonParseFn seam.
func TestLessonRecur_ParseError(t *testing.T) {
	root := stageLesson(t, "kinder-fake", "Stated")
	deps := defaultLessonCLIDeps()
	deps.parse = func(string) (*lesson.Lesson, error) {
		return nil, errors.New("parse boom")
	}
	cmd := lessonRecurCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": root})

	err := runLessonRecurWithDeps(cmd, []string{"kinder-fake"}, deps)
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d", got, exitcode.Unexpected)
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
	root := stageLesson(t, "kinder-fake", "Stated")
	if err := os.RemoveAll(filepath.Join(root, "spec", "lessons", "kinder-fake")); err != nil {
		t.Fatal(err)
	}
	writeLessonInDir(t, filepath.Join(root, "spec", "lessons"), "kinder-fake", "Stated")
	deps := defaultLessonCLIDeps()
	deps.recurWithPostMutation = func(string, string, func(int) error) (int, error) {
		return 0, errors.New("boom")
	}
	cmd := lessonRecurCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": root})

	err := runLessonRecurWithDeps(cmd, []string{"kinder-fake"}, deps)
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d", got, exitcode.Unexpected)
	}
}

// Historical test symbol retained: legacy recurrence no longer invokes a
// repository-wide lint fixer. Its bounded index callback failing after the
// body rewrite must retain the visible recurrence and prepared recovery event.
func TestLessonRecur_LintFixFails(t *testing.T) {
	root := stageLesson(t, "kinder-fake", "Stated")
	canonicalDir := filepath.Join(root, "spec", "lessons", "kinder-fake")
	if err := os.RemoveAll(canonicalDir); err != nil {
		t.Fatal(err)
	}
	flatPath := filepath.Join(root, "spec", "lessons", "kinder-fake.md")
	writeLessonInDir(t, filepath.Join(root, "spec", "lessons"), "kinder-fake", "Stated")
	deps := defaultLessonCLIDeps()
	deps.indexUpsert = func(string, *lesson.Lesson) error { return errors.New("bounded index failure") }
	cmd := lessonRecurCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": root})
	err := runLessonRecurWithDeps(cmd, []string{"kinder-fake"}, deps)
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d", got, exitcode.Unexpected)
	}
	body, readErr := os.ReadFile(flatPath)
	if readErr != nil || !strings.Contains(string(body), "**Recurred:** 1") {
		t.Fatalf("post-publication index failure lost recurrence: %q, %v", body, readErr)
	}
}

func TestLessonRecur_IndexUpsertFails(t *testing.T) {
	root := stageLesson(t, "kinder-fake", "Stated")
	if err := os.RemoveAll(filepath.Join(root, "spec", "lessons", "kinder-fake")); err != nil {
		t.Fatal(err)
	}
	writeLessonInDir(t, filepath.Join(root, "spec", "lessons"), "kinder-fake", "Stated")
	deps := defaultLessonCLIDeps()
	deps.indexUpsert = func(string, *lesson.Lesson) error {
		return errors.New("index boom")
	}
	cmd := lessonRecurCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": root})

	err := runLessonRecurWithDeps(cmd, []string{"kinder-fake"}, deps)
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d", got, exitcode.Unexpected)
	}
}
