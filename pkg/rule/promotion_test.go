package rule

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const canonicalLesson = `---
format: https://specscore.md/lesson-specification
status: Recorded
---

# Lesson: Kinder Fake Hides Bug

**Status:** Recorded
**Date:** 2026-09-03
**Owner:** alex
**Classifications:** process
**Legacy Provenance:** —
**Duplicate Of:** —
**Supersedes:** —
**Superseded By:** —

## Lesson

The durable rule.

## Open Questions

None at this time.
`

func writeLesson(t *testing.T, root, slug, body string) string {
	t.Helper()
	dir := filepath.Join(root, "spec", "lessons", slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "README.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSetLessonPromotesToInsertsAfterRelationBlock(t *testing.T) {
	root := t.TempDir()
	path := writeLesson(t, root, "kinder-fake-hides-bug", canonicalLesson)
	if err := SetLessonPromotesTo(path, "no-hand-rolled-fakes"); err != nil {
		t.Fatalf("SetLessonPromotesTo: %v", err)
	}
	text := string(mustRead(t, path))
	if !strings.Contains(text, "**Superseded By:** —\n**Promotes To:** rule:no-hand-rolled-fakes\n") {
		t.Fatalf("pointer not placed after the relation block:\n%s", text)
	}
	// Everything else survives byte-for-byte.
	for _, want := range []string{"## Lesson", "The durable rule.", "## Open Questions"} {
		if !strings.Contains(text, want) {
			t.Errorf("lesson lost %q", want)
		}
	}
}

func TestSetLessonPromotesToRewritesExisting(t *testing.T) {
	root := t.TempDir()
	path := writeLesson(t, root, "x", canonicalLesson)
	if err := SetLessonPromotesTo(path, "first"); err != nil {
		t.Fatal(err)
	}
	if err := SetLessonPromotesTo(path, "second"); err != nil {
		t.Fatal(err)
	}
	text := string(mustRead(t, path))
	if strings.Count(text, "**Promotes To:**") != 1 {
		t.Fatalf("pointer duplicated:\n%s", text)
	}
	if !strings.Contains(text, "**Promotes To:** rule:second") {
		t.Fatalf("pointer not rewritten:\n%s", text)
	}
}

// Clearing removes the line entirely: an em-dash sentinel would read as a
// half-finished promotion rather than as "never promoted".
func TestClearLessonPromotesToRemovesTheLine(t *testing.T) {
	root := t.TempDir()
	path := writeLesson(t, root, "x", canonicalLesson)
	if err := SetLessonPromotesTo(path, "r"); err != nil {
		t.Fatal(err)
	}
	if err := ClearLessonPromotesTo(path); err != nil {
		t.Fatalf("ClearLessonPromotesTo: %v", err)
	}
	if strings.Contains(string(mustRead(t, path)), "Promotes To") {
		t.Fatalf("pointer not removed:\n%s", mustRead(t, path))
	}
	// Clearing an already-clear Lesson is a no-op, not an error.
	if err := ClearLessonPromotesTo(path); err != nil {
		t.Fatalf("ClearLessonPromotesTo (second): %v", err)
	}
}

func TestSetLessonPromotesToRejectsAnchorlessDocument(t *testing.T) {
	root := t.TempDir()
	path := writeLesson(t, root, "x", "# Lesson: X\n\nno metadata fields at all\n")
	if err := SetLessonPromotesTo(path, "r"); err == nil {
		t.Fatal("SetLessonPromotesTo should refuse a Lesson with no canonical metadata field")
	}
}

func TestSetLessonPromotesToMissingFile(t *testing.T) {
	if err := SetLessonPromotesTo(filepath.Join(t.TempDir(), "nope.md"), "r"); err == nil {
		t.Fatal("SetLessonPromotesTo on a missing file should error")
	}
}
