package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/lesson"
)

// lessonIndexScaleCount is large enough to exercise the index writer and
// L-003/L-004 checker at a scale close to a real long-lived lessons store
// (sneat-co/backstage is in the hundreds), without committing any of the
// generated Lesson directories to this repository — they live only in each
// test's t.TempDir().
const lessonIndexScaleCount = 300

// build300LessonStore scaffolds lessonIndexScaleCount canonical Lessons on
// disk (each a real, parseable spec/lessons/<slug>/README.md — expected by
// expectedLessonIndexRow, which re-parses from disk) and returns the parsed
// set keyed by slug plus the sorted slug list. Occurrence directories are
// deliberately omitted: DiscoverOccurrences tolerates a missing
// occurrences/ dir (treats it as zero occurrences), and lessonIndexRules
// never calls the occurrence-child linter (L-009) that would require one.
func build300LessonStore(t *testing.T, specRoot string) (map[string]*lesson.Lesson, []string) {
	t.Helper()
	lessonsDir := filepath.Join(specRoot, "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	parsed := make(map[string]*lesson.Lesson, lessonIndexScaleCount)
	slugs := make([]string, 0, lessonIndexScaleCount)
	for i := 0; i < lessonIndexScaleCount; i++ {
		slug := fmt.Sprintf("scale-lesson-%03d", i)
		path := filepath.Join(lessonsDir, slug, "README.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		body, err := lesson.ScaffoldCanonical(lesson.ScaffoldOptions{Slug: slug, Owner: "codex", Date: "2026-08-10"}, []string{"process"})
		if err != nil {
			t.Fatalf("scaffold %s: %v", slug, err)
		}
		if err := os.WriteFile(path, body, 0o644); err != nil {
			t.Fatal(err)
		}
		l, err := lesson.Parse(path)
		if err != nil {
			t.Fatalf("parse %s: %v", slug, err)
		}
		parsed[slug] = l
		slugs = append(slugs, slug)
	}
	return parsed, slugs
}

// TestLessonIndex_300LessonStoreFixRoundTripsByteIdentical proves that
// regenerating the lessons index from the same 300-Lesson set is
// deterministic: a second full rewrite from an already-clean index produces
// byte-identical output, and a lint pass over the regenerated file reports
// no L-003/L-004 violations. This is the item-1 five-column projection
// (Enforcement dropped) exercised at a scale close to a real long-lived
// store, not the three-row fixtures the rest of this package uses.
func TestLessonIndex_300LessonStoreFixRoundTripsByteIdentical(t *testing.T) {
	specRoot := t.TempDir()
	parsed, slugs := build300LessonStore(t, specRoot)
	indexPath := filepath.Join(specRoot, "lessons", "README.md")
	empty := "# Lessons\n\n## Lessons\n\n| Lesson | Status | Classifications | Occurrences | Last Occurred |\n|---|---|---|---:|---|\n\n_No lessons recorded yet._\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(indexPath, []byte(empty), 0o644); err != nil {
		t.Fatal(err)
	}

	vs, fixed := lessonIndexRules(specRoot, parsed, true)
	if !fixed {
		t.Fatalf("expected the 300-row index to be written on first fix, violations: %+v", vs)
	}
	first, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, slug := range slugs {
		if !strings.Contains(string(first), "["+slug+"]("+slug+"/README.md)") {
			t.Fatalf("regenerated index missing row for %s", slug)
		}
	}
	if strings.Contains(string(first), "Enforcement") {
		t.Fatalf("regenerated index still carries the dropped Enforcement column:\n%.500s...", first)
	}

	// A clean lint pass over the just-regenerated index must report nothing:
	// the L-003/L-004 checker and the writer agree on the same projection.
	if vs, _ := lessonIndexRules(specRoot, parsed, false); len(vs) != 0 {
		t.Fatalf("regenerated 300-row index is not lint-clean: %+v", vs)
	}

	// Round trip: rewriting again from the identical parsed set must produce
	// byte-identical output — the projection is a pure function of the
	// discovered Lesson set, not of the file it replaces.
	if err := rewriteLessonIndexUnlocked(indexPath, slugs, parsed); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("--fix output is not round-trip stable across two rewrites of the same 300-Lesson set")
	}
}

// TestLessonIndex_300LessonStoreOldFormatIndexIsRewritten proves the
// migration path item 1 requires: a 300-row index still on the pre-slim
// six-column shape (canonical header/rows carrying a trailing Enforcement
// cell) is reported as drift by L-004 and, under --fix, rewritten in full to
// the five-column projection — with no leftover Enforcement cell anywhere
// — after which a fresh lint pass is clean.
func TestLessonIndex_300LessonStoreOldFormatIndexIsRewritten(t *testing.T) {
	specRoot := t.TempDir()
	parsed, slugs := build300LessonStore(t, specRoot)
	indexPath := filepath.Join(specRoot, "lessons", "README.md")

	var oldRows strings.Builder
	for _, slug := range slugs {
		l := parsed[slug]
		fmt.Fprintf(&oldRows, "| [%s](%s/README.md) | %s | %s | 0 |  | %s |\n",
			slug, slug, l.Status, strings.Join(l.Classifications, ", "), "—")
	}
	old := "# Lessons\n\n## Lessons\n\n" +
		"| Lesson | Status | Classifications | Occurrences | Last Occurred | Enforcement |\n" +
		"|---|---|---|---:|---|---|\n" +
		oldRows.String() +
		"\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(indexPath, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	// Lint-only: the old shape must be reported as drift (L-004), never
	// silently accepted.
	vs, fixed := lessonIndexRules(specRoot, parsed, false)
	if fixed {
		t.Fatal("lint-only pass must not rewrite the index")
	}
	if got := lessonViolation(vs, "L-004"); got == nil || !strings.Contains(got.Message, "table shape") {
		t.Fatalf("old six-column index must be flagged as a table-shape drift: %+v", vs)
	}

	vs, fixed = lessonIndexRules(specRoot, parsed, true)
	if !fixed {
		t.Fatalf("expected --fix to rewrite the old-format 300-row index, violations: %+v", vs)
	}
	got, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "Enforcement") {
		t.Fatalf("migrated index still carries the dropped Enforcement column:\n%.500s...", got)
	}
	if !strings.Contains(string(got), "| Lesson | Status | Classifications | Occurrences | Last Occurred |\n") {
		t.Fatalf("migrated index is missing the five-column header:\n%.500s...", got)
	}
	for _, slug := range slugs {
		if !strings.Contains(string(got), "["+slug+"]("+slug+"/README.md)") {
			t.Fatalf("migrated index missing row for %s", slug)
		}
	}

	if vs, _ := lessonIndexRules(specRoot, parsed, false); len(vs) != 0 {
		t.Fatalf("migrated 300-row index is not lint-clean: %+v", vs)
	}
}
