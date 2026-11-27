package cli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
)

// demoteForReceiptTest stages a lesson at status, demotes it to Recorded via
// the audited correction, and returns the project root and lesson path.
func demoteForReceiptTest(t *testing.T, status string) (string, string) {
	t.Helper()
	root := stageLesson(t, "kinder-fake", status)
	if _, stderr, err := runLesson(t, "change-status", "kinder-fake", "--to=recorded", "--note", "claimed control does not bind"); err != nil {
		t.Fatalf("demote: %v (stderr=%s)", err, stderr)
	}
	return root, filepath.Join(root, "spec", "lessons", "kinder-fake", "README.md")
}

func editLessonForReceiptTest(t *testing.T, path, old, replacement string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), old) {
		t.Fatalf("fixture lacks %q:\n%s", old, body)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(body), old, replacement, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

// AC: enforced-to-recorded-with-reason-demotes — the correction note carries
// the fingerprint of the receipt it judged not to bind.
func TestLessonChangeStatus_DemotionRecordsReceipt_CLI(t *testing.T) {
	_, path := demoteForReceiptTest(t, "Enforced")
	parsed, err := lesson.Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.CorrectionReceipt == "" || parsed.CorrectionReceipt != lesson.EnforcementReceipt(parsed) {
		t.Fatalf("demotion must record the current receipt; got %q want %q", parsed.CorrectionReceipt, lesson.EnforcementReceipt(parsed))
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "claimed control does not bind\n\n"+lesson.CorrectionReceiptPrefix+parsed.CorrectionReceipt+"\n") {
		t.Fatalf("receipt line must follow the note as its own paragraph:\n%s", body)
	}
}

// AC: re-enforce-unchanged-receipt-refused — real run and --dry-run refuse
// identically, and neither touches the tree or the event ledger.
func TestLessonChangeStatus_ReEnforceUnchangedReceiptRefused_CLI(t *testing.T) {
	root, _ := demoteForReceiptTest(t, "Enforced")
	if _, err := os.Stat(filepath.Join(root, ".specscore", "events.jsonl")); err != nil {
		t.Fatalf("fixture must have an event ledger to compare: %v", err)
	}
	before := treeDigest(t, root)
	for _, args := range [][]string{
		{"change-status", "kinder-fake", "--to=enforced"},
		{"change-status", "kinder-fake", "--to=enforced", "--dry-run"},
	} {
		stdout, _, err := runLesson(t, args...)
		if got := exitCodeOfErr(err); got != exitcode.InvalidState {
			t.Fatalf("%v: exit = %d, want %d; err=%v", args, got, exitcode.InvalidState, err)
		}
		if !strings.Contains(err.Error(), "replace the disproved receipt") || !strings.Contains(err.Error(), "unchanged since the audited correction") {
			t.Fatalf("%v: error must name the fix: %v", args, err)
		}
		if stdout != "" {
			t.Fatalf("%v: refused run printed %q", args, stdout)
		}
		if !reflect.DeepEqual(treeDigest(t, root), before) {
			t.Fatalf("%v: refused re-enforcement changed the tree or ledger", args)
		}
	}
}

// AC: re-enforce-whitespace-only-edit-refused — re-wrapping the receipt is
// not a fresh one.
func TestLessonChangeStatus_ReEnforceWhitespaceOnlyEditRefused_CLI(t *testing.T) {
	_, path := demoteForReceiptTest(t, "Enforced")
	editLessonForReceiptTest(t, path, "**Verification:** go test ./pkg/lint", "**Verification:**   go test\n  ./pkg/lint")
	_, _, err := runLesson(t, "change-status", "kinder-fake", "--to=enforced")
	if got := exitCodeOfErr(err); got != exitcode.InvalidState {
		t.Fatalf("exit = %d, want %d; err=%v", got, exitcode.InvalidState, err)
	}
}

// AC: re-enforce-with-fresh-receipt-allowed — editing Evidence is a fresh
// receipt, and the earlier correction history is kept.
func TestLessonChangeStatus_ReEnforceWithFreshReceiptAllowed_CLI(t *testing.T) {
	_, path := demoteForReceiptTest(t, "Enforced")
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "fresh-evidence.txt"), []byte("re-verified\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	editLessonForReceiptTest(t, path, "**Evidence:** spec/lessons/kinder-fake/control-evidence.txt", "**Evidence:** spec/lessons/kinder-fake/fresh-evidence.txt")
	stdout, stderr, err := runLesson(t, "change-status", "kinder-fake", "--to=enforced")
	if err != nil {
		t.Fatalf("re-enforce: %v (stderr=%s)", err, stderr)
	}
	if want := "kinder-fake: Recorded → Enforced\n"; stdout != want {
		t.Errorf("stdout = %q; want %q", stdout, want)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), lesson.CorrectionReceiptPrefix) || !strings.Contains(string(body), "claimed control does not bind") {
		t.Errorf("correction history must be kept:\n%s", body)
	}
}

// AC: stated-correction-also-requires-fresh-receipt — the Stated -> Recorded
// correction binds too, including through the Recorded -> Stated -> Enforced
// detour.
func TestLessonChangeStatus_StatedCorrectionRequiresFreshReceipt_CLI(t *testing.T) {
	root, _ := demoteForReceiptTest(t, "Stated")
	before := treeDigest(t, root)
	_, _, err := runLesson(t, "change-status", "kinder-fake", "--to=enforced")
	if got := exitCodeOfErr(err); got != exitcode.InvalidState {
		t.Fatalf("Recorded -> Enforced exit = %d, want %d; err=%v", got, exitcode.InvalidState, err)
	}
	if !reflect.DeepEqual(treeDigest(t, root), before) {
		t.Fatal("refused re-enforcement changed the tree or ledger")
	}
	if _, stderr, err := runLesson(t, "change-status", "kinder-fake", "--to=stated"); err != nil {
		t.Fatalf("Recorded -> Stated must stay legal: %v (stderr=%s)", err, stderr)
	}
	_, _, err = runLesson(t, "change-status", "kinder-fake", "--to=enforced")
	if got := exitCodeOfErr(err); got != exitcode.InvalidState {
		t.Fatalf("Stated -> Enforced exit = %d, want %d; err=%v", got, exitcode.InvalidState, err)
	}
}

// AC: pre-receipt-correction-not-refused — a correction recorded before
// demotions carried a receipt has no baseline, so it does not block.
func TestLessonChangeStatus_PreReceiptCorrectionNotRefused_CLI(t *testing.T) {
	_, path := demoteForReceiptTest(t, "Stated")
	parsed, err := lesson.Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	// Strip the receipt line, leaving the note exactly as a pre-receipt
	// correction wrote it.
	editLessonForReceiptTest(t, path, "\n\n"+lesson.CorrectionReceiptPrefix+parsed.CorrectionReceipt, "")
	if _, stderr, err := runLesson(t, "change-status", "kinder-fake", "--to=enforced"); err != nil {
		t.Fatalf("pre-receipt correction must not block: %v (stderr=%s)", err, stderr)
	}
}

func TestLessonChangeStatus_ReEnforceParseFailure(t *testing.T) {
	stageLesson(t, "kinder-fake", "Stated")
	deps := defaultLessonCLIDeps()
	deps.parse = func(string) (*lesson.Lesson, error) { return nil, errors.New("parse failed") }
	cmd := lessonChangeStatusCommand()
	if err := cmd.Flags().Set("to", "enforced"); err != nil {
		t.Fatal(err)
	}
	err := runLessonChangeStatusWithDeps(cmd, []string{"kinder-fake"}, deps)
	if got := exitCodeOfErr(err); got != exitcode.Unexpected || !strings.Contains(err.Error(), "parsing lesson before enforcement") {
		t.Fatalf("exit = %d err = %v, want Unexpected parse failure", got, err)
	}
}
