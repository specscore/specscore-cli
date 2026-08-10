package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/lesson"
)

// lessonRulesEnv is a minimal lint environment: spec root with a lessons/
// subdir.
type lessonRulesEnv struct {
	specRoot string
}

func newLessonRulesEnv(t *testing.T) *lessonRulesEnv {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "lessons"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &lessonRulesEnv{specRoot: root}
}

func (e *lessonRulesEnv) writeLesson(t *testing.T, slug, body string) string {
	t.Helper()
	path := filepath.Join(e.specRoot, "lessons", slug+".md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func (e *lessonRulesEnv) writeIndex(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(e.specRoot, "lessons", "README.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// fullLesson returns a lint-clean Lesson body: all four sections present, a
// canonical status.
func fullLesson(status string) string {
	return "# Lesson: Kinder Fake\n\n" +
		"**Status:** " + status + "\n" +
		"**Date:** 2026-07-25\n" +
		"**Owner:** alex\n" +
		"**Recurred:** 0\n\n" +
		"## Incident\n\nx\n\n## Process gap\n\nx\n\n## Check\n\nx\n\n## Enforcement\n\nx\n\n" +
		"---\n*This document follows the https://specscore.md/lesson-specification*\n"
}

func runLessonRules(t *testing.T, e *lessonRulesEnv) []Violation {
	t.Helper()
	c := newLessonRulesChecker()
	v, err := c.check(e.specRoot)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func lessonViolation(vs []Violation, rule string) *Violation {
	for i := range vs {
		if vs[i].Rule == rule {
			return &vs[i]
		}
	}
	return nil
}

func TestLessonRulesChecker_NameAndSeverity(t *testing.T) {
	c := newLessonRulesChecker()
	if c.name() != "L-001" {
		t.Errorf("name() = %q, want L-001", c.name())
	}
	if c.severity() != "error" {
		t.Errorf("severity() = %q, want error", c.severity())
	}
}

func TestLessonRulesChecker_NoLessonsDir(t *testing.T) {
	root := t.TempDir()
	c := newLessonRulesChecker()
	v, err := c.check(root)
	if err != nil {
		t.Fatal(err)
	}
	if v != nil {
		t.Errorf("expected no violations, got %v", v)
	}
}

func TestLessonRulesChecker_LessonsDirIsFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "lessons"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := newLessonRulesChecker()
	v, err := c.check(root)
	if err != nil {
		t.Fatal(err)
	}
	if v != nil {
		t.Errorf("expected no violations when lessons is a file, got %v", v)
	}
}

func TestLessonRulesChecker_SkipsReadmeAndNonLessonFiles(t *testing.T) {
	e := newLessonRulesEnv(t)
	e.writeIndex(t, "# Lessons\n\n## Index\n\n## Open Questions\n\nNone at this time.\n")
	e.writeLesson(t, "notes", "# Notes\n\nNot a lesson.\n")
	v := runLessonRules(t, e)
	if len(v) != 0 {
		t.Errorf("expected no violations, got %v", v)
	}
}

func TestLessonRulesChecker_ReadDirError(t *testing.T) {
	root := t.TempDir()
	lessonsDir := filepath.Join(root, "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() == 0 {
		t.Skip("unreadable directory is not enforced for root")
	}
	if err := os.Chmod(lessonsDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(lessonsDir, 0o755) })
	c := newLessonRulesChecker()
	if _, err := c.check(root); err == nil {
		t.Fatal("expected error reading an unreadable lessons dir")
	}
}

// TestLessonRulesChecker_SortsDiverseViolations exercises every branch of the
// final sort.SliceStable comparator: violations that differ by File, by
// Line (same File), and by Rule (same File and Line — the case every index
// violation hits, since index violations always carry Line 0).
func TestLessonRulesChecker_SortsDiverseViolations(t *testing.T) {
	e := newLessonRulesEnv(t)
	// bad-lesson: missing every section (L-001 at its TitleLine) AND an
	// invalid status (L-002 at its StatusLine) — two violations, same file,
	// different lines.
	e.writeLesson(t, "bad-lesson", "# Lesson: Bad\n\n**Status:** Banana\n")
	// missing-lesson: a fully valid Lesson simply absent from the index
	// (L-003).
	e.writeLesson(t, "missing-lesson", fullLesson("Recorded"))
	// drifted-lesson: listed in the index with a stale Status (L-004).
	e.writeLesson(t, "drifted-lesson", fullLesson("Stated"))
	e.writeIndex(t, "# Lessons\n\n## Index\n\n"+
		"| Lesson | Status | Recurred | Date | Owner |\n|---|---|---|---|---|\n"+
		"| [drifted-lesson](drifted-lesson.md) | Recorded | 0 | 2026-07-25 | alex |\n\n"+
		"## Open Questions\n\nNone at this time.\n")

	v := runLessonRules(t, e)
	if lessonViolation(v, "L-001") == nil || lessonViolation(v, "L-002") == nil ||
		lessonViolation(v, "L-003") == nil || lessonViolation(v, "L-004") == nil {
		t.Fatalf("expected one violation of each L-001..L-004, got %+v", v)
	}
	// Sorted: File ascending, then Line, then Rule — so the two index
	// violations (same File, both Line 0) must appear with L-003 before
	// L-004.
	for i := 1; i < len(v); i++ {
		if v[i-1].File > v[i].File {
			t.Errorf("violations not sorted by File: %+v", v)
		}
	}
}

func TestLessonRulesChecker_ParseError(t *testing.T) {
	e := newLessonRulesEnv(t)
	link := filepath.Join(e.specRoot, "lessons", "dangling.md")
	if err := os.Symlink(filepath.Join(e.specRoot, "nonexistent-target"), link); err != nil {
		t.Skipf("symlinks unsupported on this platform: %v", err)
	}
	c := newLessonRulesChecker()
	if _, err := c.check(e.specRoot); err == nil {
		t.Fatal("expected Parse error for dangling symlink candidate")
	}
}

func TestL001_MissingSections(t *testing.T) {
	e := newLessonRulesEnv(t)
	e.writeLesson(t, "partial", "# Lesson: Partial\n\n**Status:** Recorded\n\n## Incident\n\nx\n")
	v := runLessonRules(t, e)
	got := lessonViolation(v, "L-001")
	if got == nil {
		t.Fatal("expected an L-001 violation")
	}
	for _, want := range []string{"Process gap", "Check", "Enforcement"} {
		if !strings.Contains(got.Message, want) {
			t.Errorf("L-001 message missing %q: %q", want, got.Message)
		}
	}
	if strings.Contains(got.Message, "Incident,") || strings.Contains(got.Message, ": Incident") {
		t.Errorf("L-001 should not list a present section as missing: %q", got.Message)
	}
}

func TestL001_AllSectionsPresent_NoViolation(t *testing.T) {
	e := newLessonRulesEnv(t)
	e.writeLesson(t, "kinder-fake", fullLesson("Recorded"))
	v := runLessonRules(t, e)
	if got := lessonViolation(v, "L-001"); got != nil {
		t.Errorf("unexpected L-001 violation: %+v", got)
	}
}

func TestL002_InvalidStatus(t *testing.T) {
	e := newLessonRulesEnv(t)
	e.writeLesson(t, "kinder-fake", fullLesson("Banana"))
	v := runLessonRules(t, e)
	got := lessonViolation(v, "L-002")
	if got == nil {
		t.Fatal("expected an L-002 violation")
	}
	if !strings.Contains(got.Message, "Banana") {
		t.Errorf("L-002 message should name the offending value: %q", got.Message)
	}
}

func TestL002_ValidStatuses_NoViolation(t *testing.T) {
	for _, s := range []string{"Recorded", "Stated", "Enforced", "Withdrawn", "Superseded"} {
		e := newLessonRulesEnv(t)
		e.writeLesson(t, "kinder-fake", fullLesson(s))
		v := runLessonRules(t, e)
		if got := lessonViolation(v, "L-002"); got != nil {
			t.Errorf("status %q: unexpected L-002 violation: %+v", s, got)
		}
	}
}

func TestL002_NoStatusLine_NoViolation(t *testing.T) {
	e := newLessonRulesEnv(t)
	e.writeLesson(t, "no-status", "# Lesson: No Status\n\n## Incident\n\nx\n\n## Process gap\n\nx\n\n## Check\n\nx\n\n## Enforcement\n\nx\n")
	v := runLessonRules(t, e)
	if got := lessonViolation(v, "L-002"); got != nil {
		t.Errorf("unexpected L-002 violation for an absent Status line: %+v", got)
	}
}

// ----- L-003 / L-004: lessons index -----

func TestL003_MissingIndexRow(t *testing.T) {
	e := newLessonRulesEnv(t)
	e.writeLesson(t, "kinder-fake", fullLesson("Recorded"))
	e.writeIndex(t, "# Lessons\n\n## Index\n\n| Lesson | Status | Recurred | Date | Owner |\n|---|---|---|---|---|\n\n## Open Questions\n\nNone at this time.\n")
	v := runLessonRules(t, e)
	got := lessonViolation(v, "L-003")
	if got == nil {
		t.Fatal("expected an L-003 violation")
	}
	if !strings.Contains(got.Message, "kinder-fake") {
		t.Errorf("L-003 message should name the missing slug: %q", got.Message)
	}
}

func TestL004_DriftedRow(t *testing.T) {
	e := newLessonRulesEnv(t)
	e.writeLesson(t, "kinder-fake", fullLesson("Stated"))
	e.writeIndex(t, "# Lessons\n\n## Index\n\n"+
		"| Lesson | Status | Recurred | Date | Owner |\n|---|---|---|---|---|\n"+
		"| [kinder-fake](kinder-fake.md) | Recorded | 0 | 2026-07-25 | alex |\n\n"+
		"## Open Questions\n\nNone at this time.\n")
	v := runLessonRules(t, e)
	got := lessonViolation(v, "L-004")
	if got == nil {
		t.Fatal("expected an L-004 violation")
	}
	if !strings.Contains(got.Message, "kinder-fake") {
		t.Errorf("L-004 message should name the drifted slug: %q", got.Message)
	}
}

func TestLessonIndexRules_InSyncNoViolations(t *testing.T) {
	e := newLessonRulesEnv(t)
	e.writeLesson(t, "kinder-fake", fullLesson("Stated"))
	e.writeIndex(t, "# Lessons\n\n## Index\n\n"+
		"| Lesson | Status | Recurred | Date | Owner |\n|---|---|---|---|---|\n"+
		"| [kinder-fake](kinder-fake.md) | Stated | 0 | 2026-07-25 | alex |\n\n"+
		"## Open Questions\n\nNone at this time.\n")
	v := runLessonRules(t, e)
	if got := lessonViolation(v, "L-003"); got != nil {
		t.Errorf("unexpected L-003: %+v", got)
	}
	if got := lessonViolation(v, "L-004"); got != nil {
		t.Errorf("unexpected L-004: %+v", got)
	}
}

func TestLessonIndexRules_NoIndexFile_NoViolations(t *testing.T) {
	// A missing spec/lessons/README.md is the generic readme-exists rule's
	// job, not L-003/L-004 — mirrors ideaIndexRules' identical early return.
	e := newLessonRulesEnv(t)
	e.writeLesson(t, "kinder-fake", fullLesson("Recorded"))
	v := runLessonRules(t, e)
	if got := lessonViolation(v, "L-003"); got != nil {
		t.Errorf("unexpected L-003 with no index file: %+v", got)
	}
}

func TestLessonIndexRules_ReadRowsError(t *testing.T) {
	e := newLessonRulesEnv(t)
	e.writeLesson(t, "kinder-fake", fullLesson("Recorded"))
	// A directory named README.md makes os.Stat succeed but reading it as a
	// table fail (EISDIR on read), exercising the readLessonIndexRows error
	// branch.
	if err := os.MkdirAll(filepath.Join(e.specRoot, "lessons", "README.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	v := runLessonRules(t, e)
	if got := lessonViolation(v, "L-003"); got != nil {
		t.Errorf("unexpected L-003 when the index cannot be read: %+v", got)
	}
}

func TestLessonIndexRules_SkipsSlashedLinkTargets(t *testing.T) {
	e := newLessonRulesEnv(t)
	e.writeLesson(t, "kinder-fake", fullLesson("Recorded"))
	e.writeIndex(t, "# Lessons\n\n## Index\n\n"+
		"| Lesson | Status | Recurred | Date | Owner |\n|---|---|---|---|---|\n"+
		"| [other](sub/other.md) | Recorded | 0 | 2026-07-25 | alex |\n\n"+
		"## Open Questions\n\nNone at this time.\n")
	v := runLessonRules(t, e)
	// The slashed row is skipped entirely (neither counted as satisfying
	// kinder-fake nor causing a spurious drift report against it), so
	// kinder-fake is still reported missing.
	if got := lessonViolation(v, "L-003"); got == nil || !strings.Contains(got.Message, "kinder-fake") {
		t.Errorf("expected L-003 naming kinder-fake, got %v", v)
	}
}

func TestLessonRulesChecker_Fix_RegeneratesIndex(t *testing.T) {
	e := newLessonRulesEnv(t)
	e.writeLesson(t, "kinder-fake", fullLesson("Stated"))
	idx := e.writeIndex(t, "# Lessons\n\n## Index\n\n"+
		"| Lesson | Status | Recurred | Date | Owner |\n|---|---|---|---|---|\n\n"+
		"## Open Questions\n\nNone at this time.\n")

	c := newLessonRulesChecker()
	c.fixIndex = true
	if err := c.fix(e.specRoot); err != nil {
		t.Fatalf("fix: %v", err)
	}
	body, _ := os.ReadFile(idx)
	s := string(body)
	for _, want := range []string{"kinder-fake", "Stated"} {
		if !strings.Contains(s, want) {
			t.Errorf("fixed index missing %q:\n%s", want, s)
		}
	}
	// Idempotent: a second fix pass makes no further change and leaves the
	// tree lint-clean.
	if err := c.fix(e.specRoot); err != nil {
		t.Fatalf("fix (2nd): %v", err)
	}
	v := runLessonRules(t, e)
	if got := lessonViolation(v, "L-003"); got != nil {
		t.Errorf("unexpected L-003 after fix: %+v", got)
	}
	if got := lessonViolation(v, "L-004"); got != nil {
		t.Errorf("unexpected L-004 after fix: %+v", got)
	}
}

func TestLessonRulesChecker_Fix_Disabled_NoOp(t *testing.T) {
	e := newLessonRulesEnv(t)
	e.writeLesson(t, "kinder-fake", fullLesson("Stated"))
	idx := e.writeIndex(t, "# Lessons\n\n## Index\n\n"+
		"| Lesson | Status | Recurred | Date | Owner |\n|---|---|---|---|---|\n\n"+
		"## Open Questions\n\nNone at this time.\n")
	before, _ := os.ReadFile(idx)

	c := newLessonRulesChecker() // fixIndex left false
	if err := c.fix(e.specRoot); err != nil {
		t.Fatalf("fix: %v", err)
	}
	after, _ := os.ReadFile(idx)
	if string(before) != string(after) {
		t.Errorf("fix should be a no-op when fixIndex is false")
	}
}

func TestLessonRulesChecker_Fix_NoLessonsDir(t *testing.T) {
	root := t.TempDir()
	c := newLessonRulesChecker()
	c.fixIndex = true
	if err := c.fix(root); err != nil {
		t.Fatalf("fix on absent lessons dir: %v", err)
	}
}

func TestLessonRulesChecker_Fix_ReadDirError(t *testing.T) {
	root := t.TempDir()
	lessonsDir := filepath.Join(root, "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() == 0 {
		t.Skip("unreadable directory is not enforced for root")
	}
	if err := os.Chmod(lessonsDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(lessonsDir, 0o755) })
	c := newLessonRulesChecker()
	c.fixIndex = true
	if err := c.fix(root); err == nil {
		t.Fatal("expected error reading an unreadable lessons dir during fix")
	}
}

func TestLessonRulesChecker_Fix_ParseError(t *testing.T) {
	e := newLessonRulesEnv(t)
	link := filepath.Join(e.specRoot, "lessons", "dangling.md")
	if err := os.Symlink(filepath.Join(e.specRoot, "nonexistent-target"), link); err != nil {
		t.Skipf("symlinks unsupported on this platform: %v", err)
	}
	c := newLessonRulesChecker()
	c.fixIndex = true
	if err := c.fix(e.specRoot); err == nil {
		t.Fatal("expected Parse error for dangling symlink candidate during fix")
	}
}

func TestLessonRulesChecker_Fix_SkipsSubdirectories(t *testing.T) {
	e := newLessonRulesEnv(t)
	e.writeLesson(t, "kinder-fake", fullLesson("Stated"))
	idx := e.writeIndex(t, "# Lessons\n\n## Index\n\n"+
		"| Lesson | Status | Recurred | Date | Owner |\n|---|---|---|---|---|\n\n"+
		"## Open Questions\n\nNone at this time.\n")
	// A subdirectory under lessons/ (not a recognized artifact shape) must be
	// skipped by the fix pass rather than causing an error.
	if err := os.MkdirAll(filepath.Join(e.specRoot, "lessons", "dirform"), 0o755); err != nil {
		t.Fatal(err)
	}

	c := newLessonRulesChecker()
	c.fixIndex = true
	if err := c.fix(e.specRoot); err != nil {
		t.Fatalf("fix: %v", err)
	}
	body, _ := os.ReadFile(idx)
	if !strings.Contains(string(body), "kinder-fake") {
		t.Errorf("fix should still index kinder-fake alongside the skipped subdir:\n%s", body)
	}
}

func TestLessonRulesChecker_Fix_SkipsReadmeAndNonLessonFiles(t *testing.T) {
	e := newLessonRulesEnv(t)
	e.writeLesson(t, "notes", "# Notes\n\nNot a lesson.\n")
	idx := e.writeIndex(t, "# Lessons\n\n## Index\n\n"+
		"| Lesson | Status | Recurred | Date | Owner |\n|---|---|---|---|---|\n\n"+
		"## Open Questions\n\nNone at this time.\n")
	before, _ := os.ReadFile(idx)

	c := newLessonRulesChecker()
	c.fixIndex = true
	if err := c.fix(e.specRoot); err != nil {
		t.Fatalf("fix: %v", err)
	}
	after, _ := os.ReadFile(idx)
	if string(before) != string(after) {
		t.Errorf("fix should not touch the index when nothing needs a row:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// rewriteLessonIndex's malformed-index branch: no "## Index" heading means
// the rewrite itself fails, so lessonIndexRules must fall back to reporting
// the violation instead of silently losing it.
func TestLessonIndexRules_FixFailsWhenIndexMalformed(t *testing.T) {
	e := newLessonRulesEnv(t)
	e.writeLesson(t, "kinder-fake", fullLesson("Recorded"))
	e.writeIndex(t, "# Lessons\n\nNo Index heading here.\n")

	parsed := map[string]*lesson.Lesson{}
	l, err := lesson.Parse(filepath.Join(e.specRoot, "lessons", "kinder-fake.md"))
	if err != nil {
		t.Fatal(err)
	}
	parsed["kinder-fake"] = l

	vs, fixed := lessonIndexRules(e.specRoot, parsed, true)
	if fixed {
		t.Error("expected fixed = false when the index cannot be rewritten")
	}
	if got := lessonViolation(vs, "L-003"); got == nil {
		t.Fatal("expected an L-003 violation to survive the failed fix")
	}
}

// ----- direct coverage of small helpers whose defensive branches are not
// reachable through the checker's own call sites -----

func TestExpectedLessonIndexRow_NilLesson(t *testing.T) {
	got, err := expectedLessonIndexRow("ghost", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := lessonIndexRow{slug: "ghost", link: "ghost/README.md", occurrences: "0", enforcement: "—"}
	if got != want {
		t.Errorf("expectedLessonIndexRow(nil) = %+v, want %+v", got, want)
	}
}

func TestRewriteLessonIndex_EmptySlugsWritesPlaceholder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")
	body := "# Lessons\n\n## Index\n\n" +
		"| Lesson | Status | Recurred | Date | Owner |\n|---|---|---|---|---|\n\n" +
		"## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := rewriteLessonIndex(path, nil, map[string]*lesson.Lesson{}); err != nil {
		t.Fatalf("rewriteLessonIndex: %v", err)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "_No lessons recorded yet._") {
		t.Errorf("expected placeholder row, got:\n%s", got)
	}
}

func TestRewriteLessonIndex_ReadFileError(t *testing.T) {
	if err := rewriteLessonIndex(filepath.Join(t.TempDir(), "missing.md"), nil, nil); err == nil {
		t.Fatal("expected error reading a nonexistent index file")
	}
}

func TestReadLessonIndexRows_OpenError(t *testing.T) {
	if _, _, _, err := readLessonIndexRows(filepath.Join(t.TempDir(), "missing.md")); err == nil {
		t.Fatal("expected os.Open error for a nonexistent index file")
	}
}
