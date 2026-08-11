package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/event"
	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

func canonicalLessonProject(t *testing.T) string {
	t.Helper()
	root := setupSpecRoot(t)
	if err := projectdef.WriteSpecConfig(root, lessonTestConfig()); err != nil {
		t.Fatal(err)
	}
	withCwd(t, root)
	if _, stderr, err := runLesson(t, "new", "review-before-merge", "--owner", "tester"); err != nil {
		t.Fatalf("lesson new: %v stderr=%s", err, stderr)
	}
	return root
}

func TestLessonOccurrenceJourney_PreservesContextAndRecurCompatibility(t *testing.T) {
	root := canonicalLessonProject(t)
	out, stderr, err := runLesson(t, "occurrence", "add", "review-before-merge", "--summary", "stale branch", "--context-json", `{"git":{"commit":"abcdef1","branch":"codex/test"},"execution":{"kind":"automation","id":"run-42"}}`)
	if err != nil {
		t.Fatalf("occurrence add: %v stderr=%s", err, stderr)
	}
	fields := strings.Fields(out)
	if len(fields) != 3 {
		t.Fatalf("unexpected add output %q", out)
	}
	id := fields[2]
	path := filepath.Join(root, "spec", "lessons", "review-before-merge", "occurrences", id+".json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]any
	if err := json.Unmarshal(b, &stored); err != nil {
		t.Fatal(err)
	}
	if stored["summary"] != "stale branch" {
		t.Fatalf("summary = %#v", stored["summary"])
	}
	ctx := stored["context"].(map[string]any)
	if ctx["git"].(map[string]any)["branch"] != "codex/test" {
		t.Fatalf("context was not preserved: %#v", ctx)
	}
	index, err := os.ReadFile(filepath.Join(root, "spec", "lessons", "README.md"))
	if err != nil || !strings.Contains(string(index), "| [review-before-merge](review-before-merge/README.md) | Recorded |") || !strings.Contains(string(index), "| 1 |") {
		t.Fatalf("occurrence add did not refresh its derived index row: err=%v\\n%s", err, index)
	}
	readmeBefore, _ := os.ReadFile(filepath.Join(root, "spec", "lessons", "review-before-merge", "README.md"))
	out, _, err = runLesson(t, "recur", "review-before-merge", "--note", "seen again")
	if err != nil || out != "review-before-merge: recurred 2\n" {
		t.Fatalf("recur out=%q err=%v", out, err)
	}
	readmeAfter, _ := os.ReadFile(filepath.Join(root, "spec", "lessons", "review-before-merge", "README.md"))
	if string(readmeBefore) != string(readmeAfter) {
		t.Fatal("recur rewrote canonical README")
	}
	index, err = os.ReadFile(filepath.Join(root, "spec", "lessons", "README.md"))
	if err != nil || !strings.Contains(string(index), "| 2 |") {
		t.Fatalf("canonical recur did not refresh its derived index row: err=%v\\n%s", err, index)
	}
	info, _, err := runLesson(t, "info", "review-before-merge", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(info, `"recurred": 2`) {
		t.Fatalf("derived recurrence missing from info: %s", info)
	}
}

func TestLessonOccurrenceRejectsBadContextBeforeWrite(t *testing.T) {
	root := canonicalLessonProject(t)
	_, _, err := runLesson(t, "occurrence", "add", "review-before-merge", "--context-json", `[]`)
	if exitCodeOfErr(err) != 2 {
		t.Fatalf("exit=%d err=%v", exitCodeOfErr(err), err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "spec", "lessons", "review-before-merge", "occurrences"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("invalid input wrote occurrences: %v", entries)
	}
}

func TestLessonReadCommandsRejectTraversalThroughCentralResolver(t *testing.T) {
	canonicalLessonProject(t)
	for name, args := range map[string][]string{
		"lesson-info":     {"info", "../outside"},
		"occurrence-list": {"occurrence", "list", "../outside"},
		"occurrence-info": {"occurrence", "info", "../outside", "01234567-89ab-4def-8123-456789abcdef"},
		"relation-list":   {"relation", "list", "../outside"},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := runLesson(t, args...)
			if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
				t.Fatalf("exit=%d want=%d err=%v", got, exitcode.InvalidArgs, err)
			}
		})
	}
}

func TestLessonOccurrence_UnknownPostPublicationFailureRetainsPreparedEvent(t *testing.T) {
	root := canonicalLessonProject(t)
	deps := defaultLessonCLIDeps()
	deps.addOccurrence = func(lesson.AddOccurrenceOptions) (lesson.Occurrence, error) {
		return lesson.Occurrence{}, errors.New("injected publication/fsync boundary uncertainty")
	}
	cmd := lessonOccurrenceAddCommand()
	var stderr strings.Builder
	cmd.SetErr(&stderr)
	err := runLessonOccurrenceAddWithDeps(cmd, []string{"review-before-merge"}, deps)
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Fatalf("exit=%d want=%d err=%v", got, exitcode.Unexpected, err)
	}
	if !strings.Contains(stderr.String()+err.Error(), "recovery required: prepared event") {
		t.Fatalf("missing visible recovery instruction: stderr=%q err=%v", stderr.String(), err)
	}
	prepared, readErr := event.NewOutbox(root).Prepared()
	if readErr != nil || len(prepared) != 1 || prepared[0].EventName != "lesson.occurrence-recorded" {
		t.Fatalf("serialized outbox must retain prepared recovery state: %#v err=%v", prepared, readErr)
	}
}

func TestCaptureOccurrenceContext_DetachedHeadOmitsEmptyBranch(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("tracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.name", "SpecScore Test"},
		{"config", "user.email", "test@example.invalid"},
		{"add", "tracked.txt"},
		{"commit", "-q", "-m", "fixture"},
		{"checkout", "-q", "--detach", "HEAD"},
	} {
		runGitForFlatMigration(t, root, args...)
	}
	ctx := captureOccurrenceContext(root, "")
	gitContext, ok := ctx["git"].(map[string]any)
	if !ok || gitContext["commit"] == "" {
		t.Fatalf("detached capture lost commit: %#v", ctx)
	}
	if _, exists := gitContext["branch"]; exists {
		t.Fatalf("detached capture persisted an unavailable empty branch: %#v", gitContext)
	}
}

func TestLessonCheckFailsOnlyOverBaseline(t *testing.T) {
	canonicalLessonProject(t)
	if _, _, err := runLesson(t, "recur", "review-before-merge"); err != nil {
		t.Fatal(err)
	}
	out, _, err := runLesson(t, "check", "--not-enforced", "--min-recurred", "1")
	if !strings.Contains(out, "review-before-merge") || exitCodeOfErr(err) != 1 {
		t.Fatalf("out=%q exit=%d err=%v", out, exitCodeOfErr(err), err)
	}
	_, _, err = runLesson(t, "check", "--not-enforced", "--min-recurred", "1", "--max", "1")
	if err != nil {
		t.Fatalf("baseline allowance should pass: %v", err)
	}
}
