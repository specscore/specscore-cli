package cli

import (
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

func receiptStateForTest(t *testing.T, path string) lesson.ReceiptState {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return lesson.ReadReceiptState(body)
}

func requireReEnforceRefused(t *testing.T, root string) {
	t.Helper()
	before := treeDigest(t, root)
	_, _, err := runLesson(t, "change-status", "kinder-fake", "--to=enforced")
	if got := exitCodeOfErr(err); got != exitcode.InvalidState {
		t.Fatalf("exit = %d, want %d; err=%v", got, exitcode.InvalidState, err)
	}
	if !reflect.DeepEqual(treeDigest(t, root), before) {
		t.Fatal("refused re-enforcement changed the tree or ledger")
	}
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
	state := receiptStateForTest(t, path)
	if state.Current == "" || !reflect.DeepEqual(state.Recorded, []string{state.Current}) {
		t.Fatalf("demotion must record the current receipt: %+v", state)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "claimed control does not bind\n\n"+lesson.CorrectionReceiptPrefix+state.Current+"\n") {
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
		if !strings.Contains(err.Error(), "replace the disproved receipt") || !strings.Contains(err.Error(), "already disproved") {
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
	root, path := demoteForReceiptTest(t, "Enforced")
	// Intra-line runs and a re-wrap: only receipt normalisation makes these
	// equal (the parser alone keeps "go  test\t./pkg/lint").
	editLessonForReceiptTest(t, path, "**Verification:** go test ./pkg/lint", "**Verification:** go  test\t./pkg/lint")
	editLessonForReceiptTest(t, path, "**Control:** none-yet: just recorded, mechanism", "**Control:** none-yet:   just recorded,\n  mechanism")
	requireReEnforceRefused(t, root)
}

// AC: re-enforce-non-enforcement-edit-refused — editing any other section
// leaves the receipt unchanged.
func TestLessonChangeStatus_ReEnforceNonEnforcementEditRefused_CLI(t *testing.T) {
	root, path := demoteForReceiptTest(t, "Enforced")
	editLessonForReceiptTest(t, path, "## Open Questions\n", "## Open Questions\n\nReworded after the correction.\n")
	requireReEnforceRefused(t, root)
}

// AC: re-enforce-any-disproved-receipt-refused — after a second correction,
// reverting to the first disproved receipt is still refused (R1 -> R2 -> R1).
func TestLessonChangeStatus_ReEnforceAnyDisprovedReceiptRefused_CLI(t *testing.T) {
	root, path := demoteForReceiptTest(t, "Enforced")
	r1 := receiptStateForTest(t, path).Current
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "fresh-evidence.txt"), []byte("re-verified\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	editLessonForReceiptTest(t, path, "spec/lessons/kinder-fake/control-evidence.txt", "spec/lessons/kinder-fake/fresh-evidence.txt")
	if _, stderr, err := runLesson(t, "change-status", "kinder-fake", "--to=enforced"); err != nil {
		t.Fatalf("enforce R2: %v (stderr=%s)", err, stderr)
	}
	if _, stderr, err := runLesson(t, "change-status", "kinder-fake", "--to=recorded", "--note", "R2 does not bind either"); err != nil {
		t.Fatalf("demote R2: %v (stderr=%s)", err, stderr)
	}
	editLessonForReceiptTest(t, path, "**Evidence:** spec/lessons/kinder-fake/fresh-evidence.txt", "**Evidence:** spec/lessons/kinder-fake/control-evidence.txt")
	if state := receiptStateForTest(t, path); state.Current != r1 || len(state.Recorded) != 2 {
		t.Fatalf("fixture must be back on R1 with two recorded receipts: %+v", state)
	}
	requireReEnforceRefused(t, root)
}

// AC: note-cannot-forge-receipt-state — --note is refused (exit 2) on any
// transition when it carries a receipt-like line, an Enforcement field line,
// a `## ` heading, or an open code fence / HTML comment.
func TestLessonChangeStatus_NoteCannotForgeReceiptState_CLI(t *testing.T) {
	root, _ := demoteForReceiptTest(t, "Enforced")
	before := treeDigest(t, root)
	for _, tc := range []struct{ to, note string }{
		{"stated", lesson.CorrectionReceiptPrefix + strings.Repeat("0", 64)},
		{"stated", "**Control:** wb-hook"},
		{"withdrawn", "Does not bind.\n\n## Details\nSee CI log."},
		{"stated", "```\nunclosed"},
	} {
		_, _, err := runLesson(t, "change-status", "kinder-fake", "--to="+tc.to, "--note", tc.note)
		if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
			t.Fatalf("--to=%s --note %q: exit = %d, want %d; err=%v", tc.to, tc.note, got, exitcode.InvalidArgs, err)
		}
	}
	if !reflect.DeepEqual(treeDigest(t, root), before) {
		t.Fatal("a refused note changed the tree or ledger")
	}
	// A realistic demotion note quoting the bad Control line is refused too.
	stageLesson(t, "kinder-fake", "Enforced")
	_, _, err := runLesson(t, "change-status", "kinder-fake", "--to=recorded", "--note", "Does not bind:\n\n**Control:** none-yet: just recorded (never runs on PRs)")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Fatalf("demotion quoting Control: exit = %d, want %d; err=%v", got, exitcode.InvalidArgs, err)
	}
}

// AC: re-enforce-refused-under-lock — a refusal raced past the CLI fast path is still enforced under the
// per-Lesson lock, and its prepared event is aborted cleanly (exit 4, not a
// recovery-required exit 10).
func TestLessonChangeStatus_ReEnforceRefusedUnderLock(t *testing.T) {
	root, path := demoteForReceiptTest(t, "Enforced")
	before, _ := os.ReadFile(path)
	ledger := filepath.Join(root, ".specscore", "events.jsonl")
	ledgerBefore, _ := os.ReadFile(ledger)
	deps := defaultLessonCLIDeps()
	deps.checkReEnforcement = func(string, string) error { return nil }
	cmd := lessonChangeStatusCommand()
	if err := cmd.Flags().Set("to", "enforced"); err != nil {
		t.Fatal(err)
	}
	err := runLessonChangeStatusWithDeps(cmd, []string{"kinder-fake"}, deps)
	if got := exitCodeOfErr(err); got != exitcode.InvalidState || strings.Contains(err.Error(), "recovery required") {
		t.Fatalf("exit = %d err = %v, want a clean InvalidState refusal", got, err)
	}
	after, _ := os.ReadFile(path)
	ledgerAfter, _ := os.ReadFile(ledger)
	if string(after) != string(before) || string(ledgerAfter) != string(ledgerBefore) {
		t.Fatal("the under-lock refusal changed the lesson or the ledger")
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
	// Strip the receipt line, leaving the note exactly as a pre-receipt
	// correction wrote it.
	editLessonForReceiptTest(t, path, "\n\n"+lesson.CorrectionReceiptPrefix+receiptStateForTest(t, path).Current, "")
	if _, stderr, err := runLesson(t, "change-status", "kinder-fake", "--to=enforced"); err != nil {
		t.Fatalf("pre-receipt correction must not block: %v (stderr=%s)", err, stderr)
	}
}

func TestLessonChangeStatus_ReEnforceCheckFailure(t *testing.T) {
	stageLesson(t, "kinder-fake", "Stated")
	deps := defaultLessonCLIDeps()
	deps.checkReEnforcement = func(string, string) error { return exitcode.UnexpectedErrorf("receipt read failed") }
	cmd := lessonChangeStatusCommand()
	if err := cmd.Flags().Set("to", "enforced"); err != nil {
		t.Fatal(err)
	}
	err := runLessonChangeStatusWithDeps(cmd, []string{"kinder-fake"}, deps)
	if got := exitCodeOfErr(err); got != exitcode.Unexpected || !strings.Contains(err.Error(), "receipt read failed") {
		t.Fatalf("exit = %d err = %v, want Unexpected receipt failure", got, err)
	}
}
