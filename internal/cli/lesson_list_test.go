package cli

import (
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

func TestLessonList_DefaultListingPipeable(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)

	writeLessonInDir(t, lessonsDir, "stale-idea-check", "Recorded")
	writeLessonInDir(t, lessonsDir, "check-tags-before-tagging", "Stated")
	writeLessonInDir(t, lessonsDir, "kinder-fake", "Enforced")

	stdout, _, err := runLesson(t, "list")
	if err != nil {
		t.Fatalf("lesson list: %v", err)
	}
	want := "check-tags-before-tagging\nkinder-fake\nstale-idea-check\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// TestLessonList_StatusFilterRecorded asserts --status=recorded is a strict
// single-status match: a Stated lesson (also unenforced, but a different
// rung) must NOT appear. Without a Stated fixture here, this test cannot
// distinguish "exact match" from "matches anything not Enforced" — the
// distinction the --not-enforced tests below depend on.
func TestLessonList_StatusFilterRecorded(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)

	writeLessonInDir(t, lessonsDir, "recorded-one", "Recorded")
	writeLessonInDir(t, lessonsDir, "stated-one", "Stated")
	writeLessonInDir(t, lessonsDir, "enforced-one", "Enforced")

	stdout, _, err := runLesson(t, "list", "--status", "recorded")
	if err != nil {
		t.Fatalf("lesson list --status recorded: %v", err)
	}
	lines := nonEmptyLines(stdout)
	if len(lines) != 1 || lines[0] != "recorded-one" {
		t.Errorf("expected [recorded-one] only (stated-one must not match an exact --status=recorded filter), got %v", lines)
	}
}

// AC: the single most-valuable query — everything below Enforced, regardless
// of which of the two advisory rungs it sits on.
func TestLessonList_NotEnforced_UnionsRecordedAndStated(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)

	writeLessonInDir(t, lessonsDir, "recorded-one", "Recorded")
	writeLessonInDir(t, lessonsDir, "stated-one", "Stated")
	writeLessonInDir(t, lessonsDir, "enforced-one", "Enforced")
	writeLessonInDir(t, lessonsDir, "withdrawn-one", "Withdrawn")

	stdout, _, err := runLesson(t, "list", "--not-enforced")
	if err != nil {
		t.Fatalf("lesson list --not-enforced: %v", err)
	}
	lines := nonEmptyLines(stdout)
	want := map[string]bool{"recorded-one": true, "stated-one": true}
	if len(lines) != len(want) {
		t.Fatalf("expected %d slugs, got %v", len(want), lines)
	}
	for _, l := range lines {
		if !want[l] {
			t.Errorf("unexpected slug %q in --not-enforced output", l)
		}
	}
}

// TestLessonList_StatusCommaListUnions covers the comma-separated --status
// form, which --not-enforced is documented as shorthand for.
func TestLessonList_StatusCommaListUnions(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)

	writeLessonInDir(t, lessonsDir, "recorded-one", "Recorded")
	writeLessonInDir(t, lessonsDir, "stated-one", "Stated")
	writeLessonInDir(t, lessonsDir, "enforced-one", "Enforced")

	// A stray empty part (trailing/doubled comma) must be skipped silently,
	// mirroring parseLessonFields.
	stdout, _, err := runLesson(t, "list", "--status", "recorded,, stated,")
	if err != nil {
		t.Fatalf("lesson list --status recorded,,stated,: %v", err)
	}
	lines := nonEmptyLines(stdout)
	want := map[string]bool{"recorded-one": true, "stated-one": true}
	if len(lines) != len(want) {
		t.Fatalf("expected %d slugs, got %v", len(want), lines)
	}
	for _, l := range lines {
		if !want[l] {
			t.Errorf("unexpected slug %q", l)
		}
	}
}

// AC: an unrecognized --status value must exit 2 naming it, never silently
// resolve to an empty (and misleadingly reassuring) result.
func TestLessonList_StatusUnrecognizedValueExits2(t *testing.T) {
	setupLessonsSpec(t)

	_, _, err := runLesson(t, "list", "--status", "recorded,bogus")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidArgs, err)
	}
	if err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Errorf("error should name the offending value: %v", err)
	}
}

// --status and --not-enforced together must be rejected rather than one
// silently winning.
func TestLessonList_StatusAndNotEnforcedMutuallyExclusive(t *testing.T) {
	setupLessonsSpec(t)

	_, _, err := runLesson(t, "list", "--status", "recorded", "--not-enforced")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidArgs, err)
	}
}

// TestLessonList_MinRecurredFilters covers the recurrence filter, and its
// combination with --not-enforced — "which lessons have recurred and are
// still not enforced?" as one command.
func TestLessonList_MinRecurredFilters(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)

	writeLessonRaw(t, lessonsDir, "recurring-stated",
		"# Lesson: Recurring Stated\n\n**Status:** Stated\n**Recurred:** 2\n\n"+
			"## Incident\n\nx\n\n## Process gap\n\nx\n\n## Check\n\nx\n\n## Enforcement\n\nx\n")
	writeLessonInDir(t, lessonsDir, "quiet-stated", "Stated")
	writeLessonRaw(t, lessonsDir, "recurring-enforced",
		"# Lesson: Recurring Enforced\n\n**Status:** Enforced\n**Recurred:** 1\n\n"+
			"## Incident\n\nx\n\n## Process gap\n\nx\n\n## Check\n\nx\n\n## Enforcement\n\nx\n")

	stdout, _, err := runLesson(t, "list", "--min-recurred", "1")
	if err != nil {
		t.Fatalf("lesson list --min-recurred 1: %v", err)
	}
	lines := nonEmptyLines(stdout)
	if len(lines) != 2 {
		t.Fatalf("expected 2 recurring lessons, got %v", lines)
	}

	stdout2, _, err := runLesson(t, "list", "--not-enforced", "--min-recurred", "1")
	if err != nil {
		t.Fatalf("lesson list --not-enforced --min-recurred 1: %v", err)
	}
	lines2 := nonEmptyLines(stdout2)
	if len(lines2) != 1 || !strings.Contains(lines2[0], "recurring-stated") {
		t.Errorf("expected only recurring-stated, got %v", lines2)
	}
}

// TestLessonList_MinRecurredNegativeExits2 covers the negative-value guard.
func TestLessonList_MinRecurredNegativeExits2(t *testing.T) {
	setupLessonsSpec(t)

	_, _, err := runLesson(t, "list", "--min-recurred", "-1")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidArgs, err)
	}
}

func TestLessonList_EmptyMatchExitsZero(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)
	writeLessonInDir(t, lessonsDir, "recorded-one", "Recorded")

	stdout, _, err := runLesson(t, "list", "--status", "Withdrawn")
	if err != nil {
		t.Fatalf("lesson list --status Withdrawn: %v (expected exit 0)", err)
	}
	if stdout != "" {
		t.Errorf("expected empty stdout, got %q", stdout)
	}
}

func TestLessonList_EmptyProject(t *testing.T) {
	setupLessonsSpec(t)

	stdout, _, err := runLesson(t, "list")
	if err != nil {
		t.Fatalf("lesson list: %v", err)
	}
	if stdout != "" {
		t.Errorf("expected empty stdout, got %q", stdout)
	}
}

func TestLessonList_TextShowsRecurredCount(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)
	writeLessonRaw(t, lessonsDir, "recurring",
		"# Lesson: Recurring\n\n**Status:** Stated\n**Recurred:** 2\n\n"+
			"## Incident\n\nx\n\n## Process gap\n\nx\n\n## Check\n\nx\n\n## Enforcement\n\nx\n")

	stdout, _, err := runLesson(t, "list")
	if err != nil {
		t.Fatalf("lesson list: %v", err)
	}
	if stdout != "recurring (recurred 2)\n" {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestLessonList_InvalidFormat(t *testing.T) {
	setupLessonsSpec(t)

	_, _, err := runLesson(t, "list", "--format", "csv")
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	if !strings.Contains(err.Error(), "invalid --format") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLessonList_FormatJSON(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)

	writeLessonInDir(t, lessonsDir, "recorded-one", "Recorded")
	writeLessonInDir(t, lessonsDir, "stated-one", "Stated")

	stdout, _, err := runLesson(t, "list", "--format", "json", "--status", "Recorded")
	if err != nil {
		t.Fatalf("lesson list --format json: %v", err)
	}
	if !strings.Contains(stdout, `"slug": "recorded-one"`) {
		t.Errorf("json missing recorded-one slug: %s", stdout)
	}
	if !strings.Contains(stdout, `"status": "Recorded"`) {
		t.Errorf("json missing status: %s", stdout)
	}
	if strings.Contains(stdout, "stated-one") {
		t.Errorf("json should not include filtered-out stated-one: %s", stdout)
	}
}

func TestLessonList_FormatYAML(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)

	writeLessonInDir(t, lessonsDir, "yaml-lesson", "Recorded")

	stdout, _, err := runLesson(t, "list", "--format", "yaml")
	if err != nil {
		t.Fatalf("lesson list --format yaml: %v", err)
	}
	if !strings.Contains(stdout, "slug: yaml-lesson") {
		t.Errorf("yaml missing slug: %s", stdout)
	}
	if !strings.Contains(stdout, "status: Recorded") {
		t.Errorf("yaml missing status: %s", stdout)
	}
	if !strings.Contains(stdout, "recurred: 0") {
		t.Errorf("yaml missing recurred: %s", stdout)
	}
}

func TestLessonList_FieldsReturnsYAML(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)
	writeLessonRaw(t, lessonsDir, "fielded",
		"# Lesson: Fielded\n\n**Status:** Stated\n**Date:** 2026-07-25\n**Owner:** alex\n**Recurred:** 3\n\n"+
			"## Incident\n\nx\n\n## Process gap\n\nx\n\n## Check\n\nx\n\n## Enforcement\n\nx\n")

	stdout, _, err := runLesson(t, "list", "--fields", "status,recurred,date,owner")
	if err != nil {
		t.Fatalf("lesson list --fields: %v", err)
	}
	for _, want := range []string{"slug: fielded", "status: Stated", `recurred: "3"`, `date: "2026-07-25"`, "owner: alex"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("yaml missing %q: %s", want, stdout)
		}
	}
}

func TestLessonList_FieldsUpgradesTextToYAML(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)
	writeLessonInDir(t, lessonsDir, "upgraded-lesson", "Recorded")

	stdout, _, err := runLesson(t, "list", "--format", "text", "--fields", "status")
	if err != nil {
		t.Fatalf("lesson list --format text --fields status: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(stdout), "- ") {
		t.Errorf("expected YAML list output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "status: Recorded") {
		t.Errorf("expected status key in upgraded output, got: %s", stdout)
	}
}

func TestLessonList_FieldsJSONFormat(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)
	writeLessonInDir(t, lessonsDir, "fielded-json", "Recorded")

	stdout, _, err := runLesson(t, "list", "--fields", "status", "--format", "json")
	if err != nil {
		t.Fatalf("lesson list --fields --format json: %v", err)
	}
	if !strings.Contains(stdout, `"slug": "fielded-json"`) || !strings.Contains(stdout, `"status": "Recorded"`) {
		t.Errorf("json fields output missing expected keys: %s", stdout)
	}
}

func TestLessonList_UnknownFieldExitsTwo(t *testing.T) {
	setupLessonsSpec(t)

	_, _, err := runLesson(t, "list", "--fields", "bogus")
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	if got := exitCodeOf(err); got != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("error should name offending field: %v", err)
	}
}

func TestLessonList_FieldsEmptyValueKeyPresent(t *testing.T) {
	lessonsDir := setupLessonsSpec(t)
	writeLessonInDir(t, lessonsDir, "no-status-lesson", "")

	stdout, _, err := runLesson(t, "list", "--fields", "status")
	if err != nil {
		t.Fatalf("lesson list --fields status: %v", err)
	}
	if !strings.Contains(stdout, "status:") {
		t.Errorf("expected status key present even when empty, got: %s", stdout)
	}
}
