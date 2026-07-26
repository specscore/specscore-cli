package cli

import (
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

func TestLessonInfo_ReturnsMetadata(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)
	writeLessonRaw(t, lessonsDir, "kinder-fake",
		"# Lesson: Kinder Fake\n\n**Status:** Stated\n**Date:** 2026-07-25\n**Owner:** alex\n**Recurred:** 2\n\n"+
			"## Incident\n\nx\n\n## Process gap\n\nx\n\n## Check\n\nx\n\n## Enforcement\n\nx\n")

	stdout, _, err := runLesson(t, "info", "kinder-fake")
	if err != nil {
		t.Fatalf("lesson info kinder-fake: %v", err)
	}
	for _, want := range []string{
		"slug: kinder-fake",
		"status: Stated",
		"date: \"2026-07-25\"",
		"owner: alex",
		"recurred: 2",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("yaml missing %q: %s", want, stdout)
		}
	}
}

func TestLessonInfo_ReturnsSectionCoverage(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)
	writeLessonRaw(t, lessonsDir, "partial",
		"# Lesson: Partial\n\n**Status:** Recorded\n\n## Incident\n\nx\n\n## Process gap\n\nx\n")

	stdout, _, err := runLesson(t, "info", "partial")
	if err != nil {
		t.Fatalf("lesson info partial: %v", err)
	}
	if !strings.Contains(stdout, "Check") || !strings.Contains(stdout, "Enforcement") {
		t.Errorf("missing_sections should name the absent sections: %s", stdout)
	}
	if !strings.Contains(stdout, "Incident") {
		t.Errorf("sections should list the present ones: %s", stdout)
	}
}

func TestLessonInfo_SupersededByField(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)
	writeLessonRaw(t, lessonsDir, "old",
		"# Lesson: Old\n\n**Status:** Superseded\n**Superseded By:** new-lesson\n\n"+
			"## Incident\n\nx\n\n## Process gap\n\nx\n\n## Check\n\nx\n\n## Enforcement\n\nx\n")

	stdout, _, err := runLesson(t, "info", "old")
	if err != nil {
		t.Fatalf("lesson info old: %v", err)
	}
	if !strings.Contains(stdout, "superseded_by: new-lesson") {
		t.Errorf("yaml missing superseded_by: %s", stdout)
	}
}

func TestLessonInfo_NotFoundExits3(t *testing.T) {
	setupLessonsSpec(t)

	stdout, _, err := runLesson(t, "info", "does-not-exist")
	if err == nil {
		t.Fatal("expected error for missing lesson, got nil")
	}
	if got := exitCodeOfErr(err); got != exitcode.NotFound {
		t.Errorf("exit code = %d, want %d (NotFound)", got, exitcode.NotFound)
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("error should name the missing slug, got: %v", err)
	}
	if stdout != "" {
		t.Errorf("expected no partial stdout, got: %q", stdout)
	}
}

func TestLessonInfo_MissingSlugExits2(t *testing.T) {
	setupLessonsSpec(t)

	_, _, err := runLesson(t, "info")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
}

func TestLessonInfo_TooManyArgsExits2(t *testing.T) {
	setupLessonsSpec(t)

	_, _, err := runLesson(t, "info", "a", "b")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
}

func TestLessonInfo_InvalidFormatExits2(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)
	writeLessonInDir(t, lessonsDir, "any-lesson", "Recorded")

	_, _, err := runLesson(t, "info", "any-lesson", "--format", "csv")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
}

func TestLessonInfo_FormatJSON(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)
	writeLessonInDir(t, lessonsDir, "json-lesson", "Enforced")

	stdout, _, err := runLesson(t, "info", "json-lesson", "--format", "json")
	if err != nil {
		t.Fatalf("lesson info --format json: %v", err)
	}
	if !strings.Contains(stdout, `"status": "Enforced"`) {
		t.Errorf("json missing status: %s", stdout)
	}
}

func TestLessonInfo_FormatText_AllSectionsPresent(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)
	writeLessonInDir(t, lessonsDir, "text-lesson", "Stated")

	stdout, _, err := runLesson(t, "info", "text-lesson", "--format", "text")
	if err != nil {
		t.Fatalf("lesson info --format text: %v", err)
	}
	for _, want := range []string{"Slug:", "Status:", "Recurred:", "Sections:", "all present"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("text output missing %q: %s", want, stdout)
		}
	}
}

func TestLessonInfo_FormatText_MissingSections(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)
	writeLessonRaw(t, lessonsDir, "partial-text",
		"# Lesson: Partial Text\n\n**Status:** Recorded\n\n## Incident\n\nx\n")

	stdout, _, err := runLesson(t, "info", "partial-text", "--format", "text")
	if err != nil {
		t.Fatalf("lesson info --format text: %v", err)
	}
	if !strings.Contains(stdout, "missing:") {
		t.Errorf("text output should report missing sections: %s", stdout)
	}
}

func TestLessonInfo_FormatText_SupersededByOmittedWhenAbsent(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)
	writeLessonInDir(t, lessonsDir, "no-successor", "Recorded")

	stdout, _, err := runLesson(t, "info", "no-successor", "--format", "text")
	if err != nil {
		t.Fatalf("lesson info --format text: %v", err)
	}
	if strings.Contains(stdout, "Superseded By:") {
		t.Errorf("text output should omit Superseded By when absent: %s", stdout)
	}
}
