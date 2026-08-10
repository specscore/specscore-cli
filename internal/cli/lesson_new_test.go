package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

func readLesson(t *testing.T, root, slug string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "spec", "lessons", slug, "README.md"))
	if err != nil {
		t.Fatalf("reading lesson %s: %v", slug, err)
	}
	return string(b)
}

// AC: scaffold-emits-frontmatter-and-sections — the embedded (offline)
// scaffold carries format:/status: frontmatter mirroring the body, all four
// required sections, and the footer, AND `lesson new` leaves the tree
// lint-clean (including the lessons index row) because it runs an internal
// `spec lint --fix` pass — the near-zero-friction guarantee.
func TestLessonNew_EmbeddedEmitsFrontmatterSectionsAndIndex(t *testing.T) {
	root := setupSpecRoot(t)
	if err := projectdef.WriteSpecConfig(root, lessonTestConfig()); err != nil {
		t.Fatal(err)
	}
	withCwd(t, root)

	_, stderr, err := runLesson(t, "new", "kinder-fake", "--owner", "alex")
	if err != nil {
		t.Fatalf("lesson new: %v (stderr=%s)", err, stderr)
	}
	if strings.Contains(stderr, "no specscore.yaml") {
		t.Errorf("configured project must not take legacy fallback: %q", stderr)
	}
	b, err := os.ReadFile(filepath.Join(root, "spec", "lessons", "kinder-fake", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		"---\nformat: https://specscore.md/lesson-specification\nstatus: Recorded\n---",
		"# Lesson: Kinder Fake",
		"**Status:** Recorded",
		"**Owner:** alex",
		"## Lesson",
		"## Process Gap",
		"## Tracking",
		"## Enforcement",
		"*This document follows the https://specscore.md/lesson-specification*",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("scaffold missing %q:\n%s", want, s)
		}
	}

	if _, err := os.Stat(filepath.Join(root, "spec", "lessons", "kinder-fake", "occurrences")); err != nil {
		t.Fatalf("canonical occurrence directory missing: %v", err)
	}
}

func TestLessonNew_InvalidSlug(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	_, _, err := runLesson(t, "new", "Bad_Slug")
	if got := exitCodeOf(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d", got, exitcode.InvalidArgs)
	}
	if _, statErr := os.Stat(filepath.Join(root, "spec", "lessons", "Bad_Slug.md")); statErr == nil {
		t.Error("no file should be created for an invalid slug")
	}
}

func TestLessonNew_Collision(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	if _, _, err := runLesson(t, "new", "dup"); err != nil {
		t.Fatalf("first lesson new: %v", err)
	}
	orig := readLesson(t, root, "dup")

	_, _, err := runLesson(t, "new", "dup")
	if got := exitCodeOf(err); got != exitcode.Conflict {
		t.Errorf("exit = %d, want %d (Conflict)", got, exitcode.Conflict)
	}
	if cur := readLesson(t, root, "dup"); cur != orig {
		t.Error("conflicting run must leave the existing file untouched")
	}

	if _, _, err := runLesson(t, "new", "dup", "--title", "Duplicate Again", "--force"); err != nil {
		t.Fatalf("--force run failed: %v", err)
	}
	if !strings.Contains(readLesson(t, root, "dup"), "# Lesson: Duplicate Again") {
		t.Error("--force should overwrite with the new title")
	}
}

// AC: ancestor-indexes-materialized
func TestLessonNew_AncestorIndexesMaterialized(t *testing.T) {
	root := setupSpecRoot(t) // has spec/features + spec/ideas, but no spec/README.md or spec/lessons
	withCwd(t, root)

	if _, _, err := runLesson(t, "new", "l1"); err != nil {
		t.Fatalf("lesson new: %v", err)
	}
	for _, rel := range []string{"spec/README.md", "spec/lessons/README.md", "spec/lessons/l1/README.md", "spec/lessons/l1/occurrences"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}
}

func TestLessonNew_TitleOwnerDefaults(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	t.Setenv("USER", "envuser")
	if _, _, err := runLesson(t, "new", "defaults-lesson"); err != nil {
		t.Fatalf("lesson new: %v", err)
	}
	s := readLesson(t, root, "defaults-lesson")
	if !strings.Contains(s, "# Lesson: Defaults Lesson") {
		t.Errorf("title not defaulted from slug:\n%s", s)
	}
	if !strings.Contains(s, "**Owner:** envuser") {
		t.Errorf("owner not defaulted from $USER:\n%s", s)
	}
}

func TestLessonNew_ResolveSpecRootError(t *testing.T) {
	dir := t.TempDir() // no spec/ tree
	withCwd(t, dir)
	_, _, err := runLesson(t, "new", "p")
	if err == nil {
		t.Fatal("expected error when no spec root can be resolved")
	}
}

func TestLessonNew_ConfigPreflightWritesNothing(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	cmd := lessonCommand()
	cmd.SetArgs([]string{"new", "must-not-write"})
	if err := cmd.Execute(); exitCodeOf(err) != exitcode.InvalidState {
		t.Fatalf("exit = %d, want InvalidState; err=%v", exitCodeOf(err), err)
	}
	for _, rel := range []string{"specscore.yaml", "spec/README.md", "spec/lessons/must-not-write/README.md"} {
		if _, err := os.Stat(filepath.Join(root, rel)); err == nil {
			t.Errorf("preflight unexpectedly wrote %s", rel)
		}
	}
}

func TestLessonNew_AncestorIndexError(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	specDir := filepath.Join(root, "spec")
	if err := os.Chmod(specDir, 0o555); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(specDir, 0o755) }()

	_, _, err := runLesson(t, "new", "p")
	if got := exitCodeOf(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected)", got, exitcode.Unexpected)
	}
}

func TestLessonNew_WriteError(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	lessonsDir := filepath.Join(root, "spec", "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFileT(t, filepath.Join(root, "spec", "README.md"), "# spec\n")
	writeFileT(t, filepath.Join(lessonsDir, "README.md"), "# lessons\n")
	if err := os.Chmod(lessonsDir, 0o555); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(lessonsDir, 0o755) }()

	_, _, err := runLesson(t, "new", "p")
	if got := exitCodeOf(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected)", got, exitcode.Unexpected)
	}
}

// AC: internal-fix-pass-failure — a lint --fix failure removes the partial
// file and exits 10.
func TestLessonNew_LintFixFails(t *testing.T) {
	root := setupSpecRoot(t)
	if err := projectdef.WriteSpecConfig(root, lessonTestConfig()); err != nil {
		t.Fatal(err)
	}
	withCwd(t, root)
	orig := lintLintFn
	lintLintFn = func(opts lint.Options) ([]lint.Violation, error) {
		if opts.Fix {
			return nil, errors.New("fix boom")
		}
		return nil, nil
	}
	t.Cleanup(func() { lintLintFn = orig })

	_, _, err := runLesson(t, "new", "boom")
	if got := exitCodeOf(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected)", got, exitcode.Unexpected)
	}
	if _, statErr := os.Stat(filepath.Join(root, "spec", "lessons", "boom", "README.md")); statErr == nil {
		t.Error("partial file should be removed after a fix-pass failure")
	}
}

// AC: internal-verify-pass-failure — the second (non-fix) lint call erroring
// exits 10.
func TestLessonNew_LintVerifyFails(t *testing.T) {
	root := setupSpecRoot(t)
	if err := projectdef.WriteSpecConfig(root, lessonTestConfig()); err != nil {
		t.Fatal(err)
	}
	withCwd(t, root)
	orig := lintLintFn
	lintLintFn = func(opts lint.Options) ([]lint.Violation, error) {
		if opts.Fix {
			return nil, nil
		}
		return nil, errors.New("verify boom")
	}
	t.Cleanup(func() { lintLintFn = orig })

	_, _, err := runLesson(t, "new", "boom")
	if got := exitCodeOf(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected)", got, exitcode.Unexpected)
	}
}

// AC: generated-lesson-lint-error-surfaced — a remaining error-severity
// violation touching the new file (or the lessons/ family) after the fix
// pass surfaces as a failure rather than a silent success.
func TestLessonNew_GeneratedLessonFailsLint(t *testing.T) {
	root := setupSpecRoot(t)
	if err := projectdef.WriteSpecConfig(root, lessonTestConfig()); err != nil {
		t.Fatal(err)
	}
	withCwd(t, root)
	orig := lintLintFn
	lintLintFn = func(opts lint.Options) ([]lint.Violation, error) {
		if opts.Fix {
			return nil, nil
		}
		return []lint.Violation{
			{File: "lessons/boom.md", Line: 1, Rule: "L-001", Severity: "error", Message: "boom"},
		}, nil
	}
	t.Cleanup(func() { lintLintFn = orig })

	_, _, err := runLesson(t, "new", "boom")
	if got := exitCodeOf(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected)", got, exitcode.Unexpected)
	}
	if !strings.Contains(err.Error(), "generated lesson failed lint") {
		t.Errorf("expected generated-lesson-failed-lint message, got: %v", err)
	}
}

// AC: scaffolded-lesson-is-lint-clean — on a lint-clean project, lesson new
// introduces no error-severity violations anywhere in the tree.
func TestLessonNew_LintCleanOutsideFile(t *testing.T) {
	root := setupLintCleanProject(t)
	if err := projectdef.WriteSpecConfig(root, lessonTestConfig()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runLesson(t, "new", "clean-lesson", "--project", root); err != nil {
		t.Fatalf("lesson new: %v", err)
	}
	specSub := filepath.Join(root, "spec")
	violations, err := lint.Lint(lint.Options{SpecRoot: specSub})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		if v.Severity == "error" {
			t.Errorf("unexpected error-severity violation: %s:%d [%s] %s",
				v.File, v.Line, v.Rule, v.Message)
		}
	}
}
