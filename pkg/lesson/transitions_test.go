package lesson

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// lessonBody returns a minimal lint-shaped flat Lesson body in the given
// status.
func lessonBody(status string) string {
	return "---\nformat: https://specscore.md/lesson-specification\nstatus: " + status + "\n---\n\n" +
		"# Lesson: Kinder Fake\n\n" +
		"**Status:** " + status + "\n" +
		"**Date:** 2026-07-25\n" +
		"**Owner:** alex\n" +
		"**Recurred:** 0\n\n" +
		"## Incident\n\nSomething broke.\n\n" +
		"## Process gap\n\nNo check caught it.\n\n" +
		"## Check\n\nAdd a test.\n\n" +
		"## Enforcement\n\nRecorded only.\n\n" +
		"---\n*This document follows the https://specscore.md/lesson-specification*\n"
}

// stageLesson writes a flat lesson at spec/lessons/<slug>.md under a fresh
// SpecRoot and returns (specRoot, lessonPath).
func stageLesson(t *testing.T, slug, status string) (string, string) {
	t.Helper()
	root := t.TempDir()
	lessonsDir := filepath.Join(root, "spec", "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(lessonsDir, slug+".md")
	if err := os.WriteFile(path, []byte(lessonBody(status)), 0o644); err != nil {
		t.Fatalf("write lesson: %v", err)
	}
	return root, path
}

func okHook() error { return nil }

func codeOf(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var e *exitcode.Error
	if !errors.As(err, &e) {
		t.Fatalf("error is not *exitcode.Error: %T (%v)", err, err)
	}
	return e.ExitCode()
}

func TestChangeStatus_HappyPath_RecordedToStated(t *testing.T) {
	root, path := stageLesson(t, "kinder-fake", "Recorded")
	res, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "kinder-fake", To: lifecycle.LessonStated, PostMutation: okHook,
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if res.From != lifecycle.LessonRecorded || res.To != lifecycle.LessonStated {
		t.Errorf("result = %+v", res)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "**Status:** Stated") {
		t.Errorf("status not rewritten:\n%s", body)
	}
	if !strings.Contains(string(body), "status: Stated") {
		t.Errorf("frontmatter mirror not rewritten:\n%s", body)
	}
}

func TestChangeStatus_HappyPath_RecordedToEnforced_SkipAhead(t *testing.T) {
	root, path := stageLesson(t, "kinder-fake", "Recorded")
	res, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "kinder-fake", To: lifecycle.LessonEnforced, PostMutation: okHook,
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if res.From != lifecycle.LessonRecorded || res.To != lifecycle.LessonEnforced {
		t.Errorf("result = %+v", res)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "**Status:** Enforced") {
		t.Errorf("status not rewritten:\n%s", body)
	}
}

func TestChangeStatus_HappyPath_StatedToEnforced(t *testing.T) {
	root, _ := stageLesson(t, "kinder-fake", "Stated")
	res, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "kinder-fake", To: lifecycle.LessonEnforced, PostMutation: okHook,
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if res.From != lifecycle.LessonStated {
		t.Errorf("from = %q", res.From)
	}
}

func TestChangeStatus_Withdrawn_WritesResolution(t *testing.T) {
	root, path := stageLesson(t, "kinder-fake", "Stated")
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "kinder-fake", To: lifecycle.LessonWithdrawn,
		Note: "turned out to be a one-off", PostMutation: okHook,
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "**Status:** Withdrawn") {
		t.Errorf("status not rewritten:\n%s", body)
	}
	if !strings.Contains(string(body), "## Resolution") ||
		!strings.Contains(string(body), "turned out to be a one-off") {
		t.Errorf("resolution note not written:\n%s", body)
	}
}

func TestChangeStatus_Superseded_WritesSuccessorAndNote(t *testing.T) {
	root, path := stageLesson(t, "kinder-fake", "Enforced")
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "kinder-fake", To: lifecycle.LessonSuperseded,
		Note: "generalized", Successor: "kinder-fake-v2", PostMutation: okHook,
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	body, _ := os.ReadFile(path)
	for _, want := range []string{"**Status:** Superseded", "**Superseded By:** kinder-fake-v2", "## Resolution", "generalized"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("missing %q in:\n%s", want, body)
		}
	}
}

func TestChangeStatus_NotFound(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "spec", "lessons"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "ghost", To: lifecycle.LessonStated, PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.NotFound {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.NotFound, err)
	}
}

func TestChangeStatus_IllegalTransition(t *testing.T) {
	root, _ := stageLesson(t, "kinder-fake", "Enforced")
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "kinder-fake", To: lifecycle.LessonStated, PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidState, err)
	}
	if !strings.Contains(err.Error(), "Enforced") {
		t.Errorf("error should name current status: %q", err.Error())
	}
}

func TestChangeStatus_NoLegalTargetsMessage(t *testing.T) {
	// Withdrawn is terminal — no legal targets from it.
	root, _ := stageLesson(t, "kinder-fake", "Withdrawn")
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "kinder-fake", To: lifecycle.LessonStated, PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d", got, exitcode.InvalidState)
	}
	if !strings.Contains(err.Error(), "no legal targets") {
		t.Errorf("error should state no legal targets: %q", err.Error())
	}
}

func TestChangeStatus_NoStatusLine(t *testing.T) {
	root := t.TempDir()
	lessonsDir := filepath.Join(root, "spec", "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(lessonsDir, "kinder-fake.md")
	if err := os.WriteFile(path, []byte("# Lesson: Kinder Fake\n\nNo status here.\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "kinder-fake", To: lifecycle.LessonStated, PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
}

// TestChangeStatus_ReadStatusError covers the generic "reading lesson
// status" branch: resolving <slug> to a *directory* named kinder-fake.md lets
// os.Stat succeed (so resolution picks it) but lifecycle.Validate's status
// read then fails with a non-ENOENT, non-ErrStatusLineNotFound os error
// (EISDIR).
func TestChangeStatus_ReadStatusError(t *testing.T) {
	root := t.TempDir()
	lessonsDir := filepath.Join(root, "spec", "lessons")
	if err := os.MkdirAll(filepath.Join(lessonsDir, "kinder-fake.md"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "kinder-fake", To: lifecycle.LessonStated, PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
	if !strings.Contains(err.Error(), "reading lesson status") {
		t.Errorf("expected reading-lesson-status error, got: %q", err.Error())
	}
}

// TestChangeStatus_RewriteError covers the Rewrite-failure branch: Validate
// reads the file fine, but a read-only lessons directory makes Rewrite's
// atomic temp-write + rename fail.
func TestChangeStatus_RewriteError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("read-only directory is not enforced for root")
	}
	root, _ := stageLesson(t, "kinder-fake", "Recorded")
	lessonsDir := filepath.Join(root, "spec", "lessons")
	if err := os.Chmod(lessonsDir, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(lessonsDir, 0o755) })

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "kinder-fake", To: lifecycle.LessonStated, PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
	if !strings.Contains(err.Error(), "rewriting status line") {
		t.Errorf("expected rewrite error, got: %q", err.Error())
	}
}

func TestChangeStatus_PostMutationFails_RollsBack(t *testing.T) {
	root, path := stageLesson(t, "kinder-fake", "Enforced")
	boom := errors.New("lint failed")
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "kinder-fake", To: lifecycle.LessonSuperseded,
		Note: "replaced", Successor: "kinder-fake-v2",
		PostMutation: func() error { return boom },
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom error, got %v", err)
	}
	body, _ := os.ReadFile(path)
	// Full rollback: status restored, no note, no successor line.
	if !strings.Contains(string(body), "**Status:** Enforced") {
		t.Errorf("status not rolled back:\n%s", body)
	}
	if strings.Contains(string(body), "## Resolution") || strings.Contains(string(body), "Superseded By") {
		t.Errorf("body mutations not rolled back:\n%s", body)
	}
}

func TestChangeStatus_SuccessorWriteFails_RollsBack(t *testing.T) {
	root, path := stageLesson(t, "kinder-fake", "Enforced")
	orig := setSupersededByFn
	setSupersededByFn = func(string, string) ([]byte, bool, error) {
		return nil, false, errors.New("successor write boom")
	}
	t.Cleanup(func() { setSupersededByFn = orig })

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "kinder-fake", To: lifecycle.LessonSuperseded,
		Note: "replaced", Successor: "kinder-fake-v2", PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d", got, exitcode.Unexpected)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "**Status:** Enforced") {
		t.Errorf("status not rolled back after successor-write failure:\n%s", body)
	}
}

func TestChangeStatus_NoteWriteFails_RollsBack(t *testing.T) {
	root, path := stageLesson(t, "kinder-fake", "Enforced")
	orig := appendNoteFn
	appendNoteFn = func(string, string) ([]byte, bool, error) {
		return nil, false, errors.New("note write boom")
	}
	t.Cleanup(func() { appendNoteFn = orig })

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "kinder-fake", To: lifecycle.LessonWithdrawn,
		Note: "why", PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d", got, exitcode.Unexpected)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "**Status:** Enforced") {
		t.Errorf("status not rolled back after note-write failure:\n%s", body)
	}
}

func TestChangeStatus_GuardErrors(t *testing.T) {
	cases := []struct {
		name string
		opts ChangeStatusOptions
		want int
	}{
		{"no-specroot", ChangeStatusOptions{Slug: "a", To: lifecycle.LessonStated, PostMutation: okHook}, exitcode.Unexpected},
		{"no-slug", ChangeStatusOptions{SpecRoot: "/x", To: lifecycle.LessonStated, PostMutation: okHook}, exitcode.InvalidArgs},
		{"no-to", ChangeStatusOptions{SpecRoot: "/x", Slug: "a", PostMutation: okHook}, exitcode.InvalidArgs},
		{"no-hook", ChangeStatusOptions{SpecRoot: "/x", Slug: "a", To: lifecycle.LessonStated}, exitcode.Unexpected},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ChangeStatus(tc.opts)
			if got := codeOf(t, err); got != tc.want {
				t.Errorf("exit = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestResolveLessonFile_StatError covers the non-ENOENT stat branch: making
// spec/lessons itself a regular file makes a stat of
// spec/lessons/kinder-fake.md fail with ENOTDIR (not IsNotExist).
func TestResolveLessonFile_StatError(t *testing.T) {
	root := t.TempDir()
	specDir := filepath.Join(root, "spec")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// "lessons" is a FILE where a directory is expected.
	if err := os.WriteFile(filepath.Join(specDir, "lessons"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := ResolveLessonFile(filepath.Join(specDir, "lessons"), "kinder-fake")
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
}

func TestLegalChangeStatusTargets(t *testing.T) {
	names := LegalChangeStatusTargetNames()
	want := map[string]bool{
		"Stated": true, "Enforced": true, "Withdrawn": true, "Superseded": true,
	}
	if len(names) != len(want) {
		t.Fatalf("targets = %v, want %d entries", names, len(want))
	}
	for _, n := range names {
		if !want[n] {
			t.Errorf("unexpected target %q", n)
		}
	}
	// "Recorded" is never a target (it is only the initial state).
	for _, n := range names {
		if n == "Recorded" {
			t.Error("Recorded must not be a legal change-status target")
		}
	}
	// Sorted alphabetically.
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Errorf("targets not sorted: %v", names)
		}
	}
}

func TestLegalTransitionMatrix_Rendering(t *testing.T) {
	out := LegalTransitionMatrix()
	for _, want := range []string{"Legal transitions", "Recorded", "Stated", "Enforced", "Withdrawn", "Superseded"} {
		if !strings.Contains(out, want) {
			t.Errorf("matrix missing %q:\n%s", want, out)
		}
	}
}
