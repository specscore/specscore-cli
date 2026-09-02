package lesson

import (
	"os"
	"path/filepath"
	"testing"
)

// writeLesson writes a Lesson-shaped file at lessonsDir/<slug>.md and returns
// its path.
func writeLesson(t *testing.T, lessonsDir, slug, content string) string {
	t.Helper()
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(lessonsDir, slug+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParse_NotOpenable(t *testing.T) {
	if _, err := Parse(filepath.Join(t.TempDir(), "does-not-exist.md")); err == nil {
		t.Fatal("expected error opening a nonexistent file")
	}
}

func TestParse_NotALesson(t *testing.T) {
	dir := t.TempDir()
	path := writeLesson(t, filepath.Join(dir, "lessons"), "notes", "# Notes\n\nNot a lesson.\n")
	l, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if l.HasLessonTitle {
		t.Fatal("expected HasLessonTitle = false for a non-Lesson file")
	}
}

func TestParse_FullHeaderAndSections(t *testing.T) {
	dir := t.TempDir()
	body := `# Lesson: Kinder Fake

**Status:** Stated
**Date:** 2026-07-25
**Owner:** alex
**Recurred:** 2
**Superseded By:** newer-lesson

## Incident

It broke.

## Process gap

No check caught it.

## Check

Add a conformance suite.

## Enforcement

Stated in AGENTS.md.
`
	l, err := Parse(writeLesson(t, filepath.Join(dir, "lessons"), "kinder-fake", body))
	if err != nil {
		t.Fatal(err)
	}
	if !l.HasLessonTitle || l.Title != "Kinder Fake" {
		t.Fatalf("title parse: %+v", l)
	}
	if l.Status != "Stated" || l.StatusLine == 0 {
		t.Errorf("Status = %q line=%d", l.Status, l.StatusLine)
	}
	if l.Date != "2026-07-25" || l.DateLine == 0 {
		t.Errorf("Date = %q line=%d", l.Date, l.DateLine)
	}
	if l.Owner != "alex" || l.OwnerLine == 0 {
		t.Errorf("Owner = %q line=%d", l.Owner, l.OwnerLine)
	}
	if l.Recurred != 2 || !l.RecurredValid || l.RecurredLine == 0 {
		t.Errorf("Recurred = %d valid=%v line=%d", l.Recurred, l.RecurredValid, l.RecurredLine)
	}
	if l.SupersededBy != "newer-lesson" || l.SupersededByLine == 0 {
		t.Errorf("SupersededBy = %q line=%d", l.SupersededBy, l.SupersededByLine)
	}
	for _, s := range RequiredSections {
		if !l.HasSection(s) {
			t.Errorf("missing section %q", s)
		}
	}
	if missing := l.MissingRequiredSections(); len(missing) != 0 {
		t.Errorf("MissingRequiredSections = %v, want none", missing)
	}
	if l.HasSection("Nonexistent") {
		t.Error("HasSection should be false for an absent title")
	}
}

func TestParse_MissingSections(t *testing.T) {
	dir := t.TempDir()
	body := "# Lesson: Partial\n\n**Status:** Recorded\n\n## Incident\n\nSomething happened.\n"
	l, err := Parse(writeLesson(t, filepath.Join(dir, "lessons"), "partial", body))
	if err != nil {
		t.Fatal(err)
	}
	missing := l.MissingRequiredSections()
	want := []string{"Process gap", "Check", "Enforcement"}
	if len(missing) != len(want) {
		t.Fatalf("missing = %v, want %v", missing, want)
	}
	for i, w := range want {
		if missing[i] != w {
			t.Errorf("missing[%d] = %q, want %q", i, missing[i], w)
		}
	}
}

func TestParse_DuplicateSectionHeadingKeepsFirstLine(t *testing.T) {
	dir := t.TempDir()
	body := "# Lesson: Dup\n\n**Status:** Recorded\n\n## Incident\n\nFirst.\n\n## Incident\n\nSecond (duplicate heading).\n"
	l, err := Parse(writeLesson(t, filepath.Join(dir, "lessons"), "dup", body))
	if err != nil {
		t.Fatal(err)
	}
	if line, ok := l.SectionLines["Incident"]; !ok || line != 5 {
		t.Errorf("SectionLines[Incident] = %d, ok=%v; want first occurrence at line 5", line, ok)
	}
}

func TestParse_RecurredInvalidValueDefaultsToZero(t *testing.T) {
	dir := t.TempDir()
	body := "# Lesson: Bad Count\n\n**Status:** Recorded\n**Recurred:** not-a-number\n"
	l, err := Parse(writeLesson(t, filepath.Join(dir, "lessons"), "bad-count", body))
	if err != nil {
		t.Fatal(err)
	}
	if l.Recurred != 0 || l.RecurredValid {
		t.Errorf("Recurred = %d valid=%v, want 0/false for an unparsable value", l.Recurred, l.RecurredValid)
	}
	if l.RecurredRaw != "not-a-number" {
		t.Errorf("RecurredRaw = %q", l.RecurredRaw)
	}
}

func TestParse_RecurredNegativeValueDefaultsToZero(t *testing.T) {
	dir := t.TempDir()
	body := "# Lesson: Negative\n\n**Status:** Recorded\n**Recurred:** -1\n"
	l, err := Parse(writeLesson(t, filepath.Join(dir, "lessons"), "negative", body))
	if err != nil {
		t.Fatal(err)
	}
	if l.Recurred != 0 || l.RecurredValid {
		t.Errorf("Recurred = %d valid=%v, want 0/false for a negative value", l.Recurred, l.RecurredValid)
	}
}

// writeCanonicalLesson writes a directory-form Lesson at
// lessonsDir/<slug>/README.md and returns its path.
func writeCanonicalLesson(t *testing.T, lessonsDir, slug, content string) string {
	t.Helper()
	dir := filepath.Join(lessonsDir, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "README.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestParse_EnforcementFieldsJoinWrappedContinuationLines guards against
// REQ regression: a hand-wrapped **Control:**/**Verification:**/**Evidence:**
// value (soft-wrapped across multiple source lines, as this prose commonly
// is — the same shape that motivated the feature-Summary paragraph-join fix
// in c910c04) must be parsed as one complete value, not truncated at the
// first physical line. Before the fix, matchBoldField only ever matched the
// field's own line, so every continuation line was silently dropped.
func TestParse_EnforcementFieldsJoinWrappedContinuationLines(t *testing.T) {
	dir := t.TempDir()
	body := "# Lesson: Clean Clone Guard\n\n" +
		"**Status:** Recorded\n**Date:** 2026-08-27\n**Owner:** alex\n" +
		"**Classifications:** tooling-determinism\n" +
		"**Legacy Provenance:** —\n**Duplicate Of:** —\n**Supersedes:** —\n**Superseded By:** —\n\n" +
		"## Lesson\n\nx\n\n## Process Gap\n\nx\n\n## Tracking\n\n" +
		"- **Occurrence store:** `occurrences/`\n" +
		"- **Recurrence metadata:** derived from child JSON; never hand-maintained here.\n" +
		"- **Occurrence schema:** `https://specscore.md/new/lesson-occurrence.schema.json`\n\n" +
		"## Enforcement\n\n" +
		"**Control:** the clean-clone guard resolves each candidate write to an absolute\n" +
		"path and refuses only when that path lies inside a canonical clone and outside\n" +
		"any linked worktree cut from it — never on command name or shell cwd.\n" +
		"**Verification:** a regression suite asserting the three observed false\n" +
		"positives are permitted.\n" +
		"**Evidence:** not yet implemented — this lesson is Recorded, not\n" +
		"Enforced.\n\n" +
		"## Open Questions\n\nNone at this time.\n"
	l, err := Parse(writeCanonicalLesson(t, filepath.Join(dir, "lessons"), "clean-clone-guard", body))
	if err != nil {
		t.Fatal(err)
	}
	wantControl := "the clean-clone guard resolves each candidate write to an absolute " +
		"path and refuses only when that path lies inside a canonical clone and outside " +
		"any linked worktree cut from it — never on command name or shell cwd."
	if l.Control != wantControl {
		t.Fatalf("Control = %q, want %q (must not truncate mid-sentence)", l.Control, wantControl)
	}
	wantVerification := "a regression suite asserting the three observed false positives are permitted."
	if l.Verification != wantVerification {
		t.Fatalf("Verification = %q, want %q", l.Verification, wantVerification)
	}
	wantEvidence := "not yet implemented — this lesson is Recorded, not Enforced."
	if l.Evidence != wantEvidence {
		t.Fatalf("Evidence = %q, want %q", l.Evidence, wantEvidence)
	}
}

// TestParse_EnforcementFieldStopsAtBlankLine asserts that a field's
// continuation is only ever the immediately following contiguous non-blank
// lines — a blank line still ends the value, matching the existing
// "single paragraph" contract instead of slurping unrelated content that
// follows later in the section (e.g. a blockquoted rule after the field
// block).
func TestParse_EnforcementFieldStopsAtBlankLine(t *testing.T) {
	dir := t.TempDir()
	body := "# Lesson: Stated Rule\n\n" +
		"**Status:** Stated\n**Date:** 2026-08-27\n**Owner:** alex\n" +
		"**Classifications:** tooling-determinism\n" +
		"**Legacy Provenance:** —\n**Duplicate Of:** —\n**Supersedes:** —\n**Superseded By:** —\n\n" +
		"## Lesson\n\nx\n\n## Process Gap\n\nx\n\n## Tracking\n\n" +
		"- **Occurrence store:** `occurrences/`\n" +
		"- **Recurrence metadata:** derived from child JSON; never hand-maintained here.\n" +
		"- **Occurrence schema:** `https://specscore.md/new/lesson-occurrence.schema.json`\n\n" +
		"## Enforcement\n\n" +
		"**Control:** Added to CLAUDE.md, verbatim:\n\n" +
		"> Must not be appended to Control.\n\n" +
		"**Verification:** the rule text is present.\n" +
		"**Evidence:** —\n\n" +
		"## Open Questions\n\nNone at this time.\n"
	l, err := Parse(writeCanonicalLesson(t, filepath.Join(dir, "lessons"), "stated-rule", body))
	if err != nil {
		t.Fatal(err)
	}
	if want := "Added to CLAUDE.md, verbatim:"; l.Control != want {
		t.Fatalf("Control = %q, want %q (must stop at blank line)", l.Control, want)
	}
	if want := "the rule text is present."; l.Verification != want {
		t.Fatalf("Verification = %q, want %q", l.Verification, want)
	}
}

func TestIsSingleFileLessonPath(t *testing.T) {
	lessonsDir := filepath.Join("spec", "lessons")
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"not-md", filepath.Join(lessonsDir, "notes.txt"), false},
		{"readme", filepath.Join(lessonsDir, "README.md"), false},
		{"bare-readme", "README.md", false},
		{"nested", filepath.Join(lessonsDir, "sub", "child.md"), false},
		{"direct-child", filepath.Join(lessonsDir, "kinder-fake.md"), true},
		{"outside-dir", filepath.Join("other", "kinder-fake.md"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsSingleFileLessonPath(lessonsDir, tc.path); got != tc.want {
				t.Errorf("IsSingleFileLessonPath(%q, %q) = %v, want %v", lessonsDir, tc.path, got, tc.want)
			}
		})
	}
}

func TestIsSingleFileLessonPath_DirItself(t *testing.T) {
	// filePath == lessonsDir exactly (both ending in ".md", an artificial but
	// reachable shape): filepathRel returns an error (empty rel), so the
	// function must return false rather than panicking.
	if IsSingleFileLessonPath("spec/lessons.md", "spec/lessons.md") {
		t.Error("expected false when filePath equals lessonsDir")
	}
}
