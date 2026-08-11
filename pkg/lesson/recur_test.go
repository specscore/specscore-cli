package lesson

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecur_IncrementsExistingCount(t *testing.T) {
	dir := t.TempDir()
	path := writeLesson(t, dir, "kinder-fake", lessonBody("Stated"))

	n, err := Recur(path, "happened again in a different bot")
	if err != nil {
		t.Fatalf("Recur: %v", err)
	}
	if n != 1 {
		t.Fatalf("Recur count = %d, want 1", n)
	}
	body, _ := os.ReadFile(path)
	s := string(body)
	if !strings.Contains(s, "**Recurred:** 1") {
		t.Errorf("Recurred count not incremented:\n%s", s)
	}
	if !strings.Contains(s, "## Recurrences") || !strings.Contains(s, "happened again in a different bot") {
		t.Errorf("recurrence entry not recorded:\n%s", s)
	}

	// A second call increments again and appends a second entry.
	n2, err := Recur(path, "")
	if err != nil {
		t.Fatalf("Recur (2nd): %v", err)
	}
	if n2 != 2 {
		t.Fatalf("Recur count (2nd) = %d, want 2", n2)
	}
	body2, _ := os.ReadFile(path)
	s2 := string(body2)
	if !strings.Contains(s2, "**Recurred:** 2") {
		t.Errorf("Recurred count not incremented on 2nd call:\n%s", s2)
	}
	if strings.Count(s2, "## Recurrences") != 1 {
		t.Errorf("expected exactly one ## Recurrences heading, got body:\n%s", s2)
	}
}

func TestRecur_InsertsMissingRecurredFieldAfterStatus(t *testing.T) {
	dir := t.TempDir()
	body := "---\nformat: https://specscore.md/lesson-specification\nstatus: Recorded\n---\n\n" +
		"# Lesson: Legacy\n\n**Status:** Recorded\n**Date:** 2026-07-01\n**Owner:** alex\n\n" +
		"## Incident\n\nx\n\n## Process gap\n\nx\n\n## Check\n\nx\n\n## Enforcement\n\nx\n\n" +
		"---\n*This document follows the https://specscore.md/lesson-specification*\n"
	path := writeLesson(t, dir, "legacy", body)

	n, err := Recur(path, "")
	if err != nil {
		t.Fatalf("Recur: %v", err)
	}
	if n != 1 {
		t.Fatalf("Recur count = %d, want 1", n)
	}
	got, _ := os.ReadFile(path)
	s := string(got)
	if !strings.Contains(s, "**Status:** Recorded\n**Recurred:** 1\n") {
		t.Errorf("Recurred field not inserted right after Status:\n%s", s)
	}
}

func TestRecur_ClampsExistingNegativeCountToZeroBeforeIncrementing(t *testing.T) {
	dir := t.TempDir()
	body := "# Lesson: Bad Count\n\n**Status:** Recorded\n**Recurred:** -3\n\n## Incident\n\nx\n"
	path := writeLesson(t, dir, "bad-count", body)

	n, err := Recur(path, "")
	if err != nil {
		t.Fatalf("Recur: %v", err)
	}
	if n != 1 {
		t.Fatalf("Recur count = %d, want 1 (a corrupt negative count self-heals to 0 before incrementing)", n)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "**Recurred:** 1") {
		t.Errorf("Recurred not self-healed to 1:\n%s", got)
	}
}

func TestRecur_NoStatusLineErrors(t *testing.T) {
	dir := t.TempDir()
	path := writeLesson(t, dir, "no-status", "# Lesson: No Status\n\n## Incident\n\nx\n")
	if _, err := Recur(path, ""); err == nil {
		t.Fatal("expected error when neither **Recurred:** nor **Status:** is present")
	}
}

func TestRecur_ReadFileError(t *testing.T) {
	if _, err := Recur(filepath.Join(t.TempDir(), "missing.md"), ""); err == nil {
		t.Fatal("expected error reading a nonexistent lesson file")
	}
}

func TestRecur_AppendsToExistingRecurrencesSectionBeforeNextHeading(t *testing.T) {
	dir := t.TempDir()
	body := "# Lesson: Has Section\n\n**Status:** Stated\n**Recurred:** 1\n\n" +
		"## Incident\n\nx\n\n## Process gap\n\nx\n\n## Check\n\nx\n\n## Enforcement\n\nx\n\n" +
		"## Recurrences\n\n- 2026-07-01 — first occurrence\n\n" +
		"---\n*This document follows the https://specscore.md/lesson-specification*\n"
	path := writeLesson(t, dir, "has-section", body)

	if _, err := Recur(path, "second occurrence"); err != nil {
		t.Fatalf("Recur: %v", err)
	}
	got, _ := os.ReadFile(path)
	s := string(got)
	if !strings.Contains(s, "- 2026-07-01 — first occurrence") {
		t.Errorf("existing entry lost:\n%s", s)
	}
	if !strings.Contains(s, "second occurrence") {
		t.Errorf("new entry not appended:\n%s", s)
	}
	// The new entry must land after the existing one and before the
	// adherence-footer text line (mirroring pkg/lifecycle's own
	// isFooterLine convention: only the "*This document follows…" line is
	// recognized as the footer anchor).
	firstIdx := strings.Index(s, "first occurrence")
	secondIdx := strings.Index(s, "second occurrence")
	footerIdx := strings.Index(s, "*This document follows")
	if firstIdx == -1 || secondIdx == -1 || footerIdx == -1 || firstIdx >= secondIdx || secondIdx >= footerIdx {
		t.Errorf("new entry not placed after the existing one and before the footer:\n%s", s)
	}
}

func TestRecur_CreatesSectionAtEOFWhenNoFooter(t *testing.T) {
	dir := t.TempDir()
	body := "# Lesson: No Footer\n\n**Status:** Recorded\n**Recurred:** 0\n\n## Incident\n\nx\n"
	path := writeLesson(t, dir, "no-footer", body)

	if _, err := Recur(path, "occurred again"); err != nil {
		t.Fatalf("Recur: %v", err)
	}
	got, _ := os.ReadFile(path)
	s := string(got)
	if !strings.Contains(s, "## Recurrences") || !strings.Contains(s, "occurred again") {
		t.Errorf("recurrence section not created at EOF:\n%s", s)
	}
}

func TestRecur_WriteFileError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("read-only file is not enforced for root")
	}
	dir := t.TempDir()
	path := writeLesson(t, dir, "kinder-fake", lessonBody("Stated"))
	// Recurrence publication is same-directory temp+rename, so make the
	// containing directory read-only to fail before publication.
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	if _, err := Recur(path, "note"); err == nil {
		t.Fatal("expected write error in a read-only directory")
	}
	if _, err := recurUnlocked(path, "note"); err == nil {
		t.Fatal("expected atomic writer error in a read-only directory")
	}
}

func TestRecurWithPostMutationCanonicalPathAndHook(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "spec", "lessons", "rule")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "README.md")
	if err := os.WriteFile(path, []byte(lessonBody("Recorded")), 0o644); err != nil {
		t.Fatal(err)
	}
	called := 0
	count, err := RecurWithPostMutation(path, "again", func(got int) error {
		called++
		if got != 1 {
			t.Fatalf("hook count=%d want 1", got)
		}
		return nil
	})
	if err != nil || count != 1 || called != 1 {
		t.Fatalf("count=%d called=%d err=%v", count, called, err)
	}
	boom := fmt.Errorf("post mutation")
	count, err = RecurWithPostMutation(path, "again", func(int) error { return boom })
	if !errors.Is(err, boom) || count != 2 {
		t.Fatalf("hook failure count=%d err=%v", count, err)
	}
}

func TestRecurAtomicWriterFilesystemBoundariesRetainPublishedState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lesson.md")
	original := []byte("original\n")
	updated := []byte("updated\n")
	if err := os.WriteFile(path, original, 0o640); err != nil {
		t.Fatal(err)
	}
	baseline := &faultMatrixFS{lessonFS: osLessonFS{}}
	if err := writeRecurFileAtomicWithFS(path, updated, baseline); err != nil {
		t.Fatal(err)
	}
	for failAt, op := range baseline.trace {
		failAt, op := failAt+1, op
		t.Run(fmt.Sprintf("%03d-%s", failAt, op), func(t *testing.T) {
			if err := os.WriteFile(path, original, 0o640); err != nil {
				t.Fatal(err)
			}
			fs := &faultMatrixFS{lessonFS: osLessonFS{}, failAt: failAt}
			err := writeRecurFileAtomicWithFS(path, updated, fs)
			got, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if err == nil {
				// Best-effort cleanup errors occur only on the now-unlinked temp.
				if op != "Remove" {
					t.Fatalf("injected %s failure was ignored", op)
				}
				return
			}
			if bytes.Equal(got, original) {
				return
			}
			if !bytes.Equal(got, updated) || MutationOutcomeOf(err) != MutationUncertain {
				t.Fatalf("post-publication %s: bytes=%q outcome=%v err=%v", op, got, MutationOutcomeOf(err), err)
			}
		})
	}
}
