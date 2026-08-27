package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/specscore/specscore-cli/pkg/plan"
)

// planTasksPlaceholder is the exact TODO comment `plan new` scaffolds inside
// an otherwise-empty `## Tasks` section (pkg/plan/scaffold.go). Test fixtures
// that need real `### Task N:` blocks replace it verbatim.
const planTasksPlaceholder = "<!-- TODO: add `### Task N:` blocks, each with a **Verifies:** line naming a source-Feature AC (feature-slug#ac:<ac-slug>) and a **Status:** line set to a canonical task status (planning, queued, in_progress, blocked, complete, failed, aborted); a new task starts at planning. -->"

// stageReconcilablePlan bootstraps a lint-clean spec repo with a scaffolded
// plan at the given status, carrying one `### Task N:` block per entry in
// taskStatuses (all sourceless, so P-001/P-002 AC-coverage rules don't apply).
// Mirrors stagePlan (plan_change_status_test.go) but injects real task
// blocks in place of the scaffold's TODO placeholder, and verifies the
// fixture is lint-clean before returning.
func stageReconcilablePlan(t *testing.T, slug, status string, taskStatuses ...string) string {
	t.Helper()
	root := setupSpecRoot(t)
	withCwd(t, root)
	if _, _, err := runPlan(t, "new", slug, "--owner", "tester"); err != nil {
		t.Fatalf("plan new: %v", err)
	}
	featuresReadme := "# Features\n\n## Index\n\n| Feature | Status |\n|---------|--------|\n\n_No features yet._\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(root, "spec", "features", "README.md"), []byte(featuresReadme), 0o644); err != nil {
		t.Fatalf("write features README: %v", err)
	}

	path := filepath.Join(root, "spec", "plans", slug+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read plan: %v", err)
	}
	patched := strings.Replace(string(raw), "**Status:** Draft", "**Status:** "+status, 1)
	patched = strings.Replace(patched, "status: Draft", "status: "+status, 1)

	if len(taskStatuses) > 0 {
		if !strings.Contains(patched, planTasksPlaceholder) {
			t.Fatalf("tasks placeholder not found in plan body:\n%s", patched)
		}
		var b strings.Builder
		for i, s := range taskStatuses {
			fmt.Fprintf(&b, "### Task %d: Step %d\n\n**Status:** %s\n\n", i+1, i+1, s)
		}
		patched = strings.Replace(patched, planTasksPlaceholder+"\n\n", b.String(), 1)
	}

	if err := os.WriteFile(path, []byte(patched), 0o644); err != nil {
		t.Fatalf("write patched plan: %v", err)
	}

	migrateTree(t, root)

	if _, err := lint.Lint(lint.Options{SpecRoot: filepath.Join(root, "spec"), Fix: true}); err != nil {
		t.Fatalf("initial lint --fix: %v", err)
	}
	vs, err := lint.Lint(lint.Options{SpecRoot: filepath.Join(root, "spec")})
	if err != nil {
		t.Fatalf("verify lint: %v", err)
	}
	for _, v := range vs {
		if v.Severity == "error" {
			t.Fatalf("pre-existing lint error in fixture: %s:%d [%s] %s", v.File, v.Line, v.Rule, v.Message)
		}
	}
	return root
}

func eightPlanningTasks() []string {
	out := make([]string, 8)
	for i := range out {
		out[i] = "planning"
	}
	return out
}

// AC: happy-path-eight-tasks — the founder's motivating scenario end-to-end
// through the CLI: a Draft plan with 8 embedded tasks stuck at planning gets
// reconciled to Implemented, and `spec lint` passes afterward.
func TestPlanReconcile_HappyPath_CLI(t *testing.T) {
	root := stageReconcilablePlan(t, "auth", "Draft", eightPlanningTasks()...)

	stdout, stderr, err := runPlan(t, "reconcile", "auth", "--tasks=complete",
		"--note", "implemented directly on main during the outage; tracked flow was skipped",
		"--evidence", "a1b2c3d,https://github.com/org/repo/pull/42")
	if err != nil {
		t.Fatalf("reconcile: %v (stderr=%s)", err, stderr)
	}
	if want := "auth: Draft → Implemented (reconciled, 8 task(s) marked complete)\n"; stdout != want {
		t.Errorf("stdout = %q; want %q", stdout, want)
	}

	body, _ := os.ReadFile(filepath.Join(root, "spec", "plans", "auth.md"))
	s := string(body)
	if !strings.Contains(s, "**Status:** Implemented") {
		t.Errorf("status not rewritten:\n%s", s)
	}
	if !strings.Contains(s, "status: Implemented") || strings.Contains(s, "status: Draft") {
		t.Errorf("frontmatter status mirror was not atomically reconciled with the body status:\n%s", s)
	}
	if strings.Count(s, "**Status:** complete") != 8 {
		t.Errorf("expected 8 completed tasks:\n%s", s)
	}
	if !strings.Contains(s, "**Reconciled:**") {
		t.Errorf("missing **Reconciled:** marker:\n%s", s)
	}
	if !strings.Contains(s, "Evidence: a1b2c3d, https://github.com/org/repo/pull/42") {
		t.Errorf("missing evidence:\n%s", s)
	}

	// REQ: spec lint must pass on the reconciled artifact, and the derived
	// plan status must agree with the reconciled task rollup.
	vs, err := lint.Lint(lint.Options{SpecRoot: filepath.Join(root, "spec")})
	if err != nil {
		t.Fatalf("lint: %v", err)
	}
	for _, v := range vs {
		if v.Severity == "error" {
			t.Errorf("unexpected lint error after reconcile: %s:%d [%s] %s", v.File, v.Line, v.Rule, v.Message)
		}
	}
	if locks := transactionLockFiles(t, filepath.Join(root, "spec")); len(locks) != 0 {
		t.Fatalf("successful plan reconcile left transaction-lock artifacts: %v", locks)
	}
}

func TestPlanReconcile_TreeTransactionPublishesDeclaredPlanChanges(t *testing.T) {
	root := stageReconcilablePlan(t, "auth", "Draft", "planning", "planning")
	stdout, stderr, err := runPlan(t, "reconcile", "auth", "--tasks=complete",
		"--note", "delivered outside the tracked flow", "--tree-transaction")
	if err != nil {
		t.Fatalf("tree transaction reconcile: %v (stderr=%s)", err, stderr)
	}
	if !strings.Contains(stdout, "auth: Draft → Implemented") {
		t.Fatalf("stdout = %q", stdout)
	}
	receipts, err := readLifecycleReceipts(root)
	if err != nil || len(receipts) != 1 {
		t.Fatalf("tree transaction receipts = %#v, %v", receipts, err)
	}
	receipt := receipts[0]
	if receipt.State != "committed" || !slices.Equal(receipt.DeclaredWriteSet, []string{"plans/README.md", "plans/auth.md"}) {
		t.Fatalf("tree transaction receipt = %#v", receipt)
	}
	live, err := os.ReadFile(filepath.Join(root, "spec", "plans", "auth.md"))
	if err != nil || !strings.Contains(string(live), "**Status:** Implemented") {
		t.Fatalf("published Plan = %v\n%s", err, live)
	}
	predecessor, err := os.ReadFile(filepath.Join(receipt.RecoveryRoot, "spec", "plans", "auth.md"))
	if err != nil || !strings.Contains(string(predecessor), "**Status:** Draft") {
		t.Fatalf("retained predecessor = %v\n%s", err, predecessor)
	}
	index, err := os.ReadFile(filepath.Join(root, "spec", "plans", "README.md"))
	if err != nil || !strings.Contains(string(index), "| [auth](auth.md) | Implemented |") {
		t.Fatalf("published plans index = %v\n%s", err, index)
	}
}

func TestPlanReconcile_TreeTransactionValidationCreatesNoRecoveryState(t *testing.T) {
	root := stageReconcilablePlan(t, "auth", "Draft", "failed")
	_, _, err := runPlan(t, "reconcile", "auth", "--tasks=complete", "--note", "unsupported claim", "--tree-transaction")
	if got := exitCodeOfErr(err); got != exitcode.InvalidState {
		t.Fatalf("exit = %d, want %d: %v", got, exitcode.InvalidState, err)
	}
	entries, readErr := os.ReadDir(root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, entry := range entries {
		if entry.Name() == ".specscore-recovery" || strings.HasPrefix(entry.Name(), ".specscore-txn-") {
			t.Fatalf("validation refusal created recovery state: %s", entry.Name())
		}
	}
	if locks := transactionLockFiles(t, filepath.Join(root, "spec")); len(locks) != 0 {
		t.Fatalf("failed plan reconcile left transaction-lock artifacts: %v", locks)
	}
}

func TestPlanReconcile_TreeTransactionStagedFailures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		arrange func(t *testing.T)
	}{
		{
			name: "staged snapshot parse",
			arrange: func(t *testing.T) {
				original := planReconcileFn
				t.Cleanup(func() { planReconcileFn = original })
				planReconcileFn = func(opts plan.ReconcileOptions) (plan.ReconcileResult, error) {
					return plan.ReconcileResult{}, opts.ValidateSnapshot("invalid.md", []byte(strings.Repeat("x", (1<<20)+1)))
				}
			},
		},
		{
			name: "staged reconcile",
			arrange: func(t *testing.T) {
				original := planReconcileFn
				t.Cleanup(func() { planReconcileFn = original })
				planReconcileFn = func(plan.ReconcileOptions) (plan.ReconcileResult, error) {
					return plan.ReconcileResult{}, errors.New("staged reconcile failed")
				}
			},
		},
		{
			name: "staged index sync",
			arrange: func(t *testing.T) {
				original := planSyncIndexFn
				t.Cleanup(func() { planSyncIndexFn = original })
				planSyncIndexFn = func(string) (bool, error) { return false, errors.New("staged sync failed") }
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stageReconcilablePlan(t, "auth", "Draft", "planning")
			tc.arrange(t)
			if _, _, err := runPlan(t, "reconcile", "auth", "--tasks=complete", "--note", "reason", "--tree-transaction"); err == nil {
				t.Fatal("tree transaction unexpectedly succeeded")
			}
		})
	}
}

// AC: missing-note-refused
func TestPlanReconcile_MissingNote_CLI(t *testing.T) {
	stageReconcilablePlan(t, "auth", "Draft", "planning")
	_, _, err := runPlan(t, "reconcile", "auth", "--tasks=complete")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidArgs, err)
	}
}

// AC: unknown-slug
func TestPlanReconcile_UnknownSlug_CLI(t *testing.T) {
	stageReconcilablePlan(t, "auth", "Draft", "planning")
	_, _, err := runPlan(t, "reconcile", "ghost", "--tasks=complete", "--note", "x")
	if got := exitCodeOfErr(err); got != exitcode.NotFound {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.NotFound, err)
	}
}

// AC: re-run-refused — a second reconcile on an already-reconciled plan is a
// no-op refusal, not a silent success.
func TestPlanReconcile_ReRun_CLI(t *testing.T) {
	stageReconcilablePlan(t, "auth", "Draft", "planning", "planning")
	if _, _, err := runPlan(t, "reconcile", "auth", "--tasks=complete", "--note", "first pass"); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	_, _, err := runPlan(t, "reconcile", "auth", "--tasks=complete", "--note", "second pass")
	if got := exitCodeOfErr(err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidState, err)
	}
}

func TestPlanReconcile_ArgErrors_CLI(t *testing.T) {
	stageReconcilablePlan(t, "auth", "Draft", "planning")
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"missing-slug", []string{"reconcile", "--tasks=complete", "--note", "x"}, exitcode.InvalidArgs},
		{"too-many-args", []string{"reconcile", "auth", "extra", "--tasks=complete", "--note", "x"}, exitcode.InvalidArgs},
		{"bad-slug", []string{"reconcile", "Bad_Slug", "--tasks=complete", "--note", "x"}, exitcode.InvalidArgs},
		{"missing-tasks", []string{"reconcile", "auth", "--note", "x"}, exitcode.InvalidArgs},
		{"unrecognized-tasks", []string{"reconcile", "auth", "--tasks=some", "--note", "x"}, exitcode.InvalidArgs},
		{"missing-note", []string{"reconcile", "auth", "--tasks=complete"}, exitcode.InvalidArgs},
		{"bad-force-tasks", []string{"reconcile", "auth", "--tasks=complete", "--note", "x", "--force-tasks=abc"}, exitcode.InvalidArgs},
		{"negative-force-tasks", []string{"reconcile", "auth", "--tasks=complete", "--note", "x", "--force-tasks=-1"}, exitcode.InvalidArgs},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := runPlan(t, tc.args...)
			if got := exitCodeOfErr(err); got != tc.want {
				t.Errorf("exit = %d, want %d; err=%v", got, tc.want, err)
			}
		})
	}
}

// AC: --tasks value is matched case-insensitively.
func TestPlanReconcile_TasksFlagCaseInsensitive_CLI(t *testing.T) {
	stageReconcilablePlan(t, "auth", "Draft", "planning")
	_, stderr, err := runPlan(t, "reconcile", "auth", "--tasks=COMPLETE", "--note", "x")
	if err != nil {
		t.Fatalf("reconcile: %v (stderr=%s)", err, stderr)
	}
}

// AC: a task recorded as failed is never silently force-completed; the
// refusal names the task number and its status. Staged at Draft (not
// Approved) because Approved is P-007 derivation-eligible: the fixture
// helper's own lint --fix pass would otherwise re-derive the body status to
// Failed before reconcile even runs, given a task already at failed.
func TestPlanReconcile_FailedTask_Refused_CLI(t *testing.T) {
	stageReconcilablePlan(t, "auth", "Draft", "complete", "failed")
	_, stderr, err := runPlan(t, "reconcile", "auth", "--tasks=complete", "--note", "x")
	if got := exitCodeOfErr(err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidState, err)
	}
	if !strings.Contains(err.Error(), "Task 2 (failed)") && !strings.Contains(stderr, "Task 2 (failed)") {
		t.Errorf("expected error to name Task 2 (failed), got err=%q stderr=%q", err, stderr)
	}
}

// AC: --force-tasks acknowledges the override end-to-end through the CLI:
// the command succeeds, the stdout line discloses the forced task, and
// spec lint still passes on the result. Staged at Draft for the same reason
// as FailedTask_Refused_CLI above.
func TestPlanReconcile_ForceTasks_OverrideSucceeds_CLI(t *testing.T) {
	root := stageReconcilablePlan(t, "auth", "Draft", "complete", "failed")
	stdout, stderr, err := runPlan(t, "reconcile", "auth", "--tasks=complete",
		"--note", "task 2 actually landed on retry", "--force-tasks=2")
	if err != nil {
		t.Fatalf("reconcile: %v (stderr=%s)", err, stderr)
	}
	if want := "auth: Draft → Implemented (reconciled, 1 task(s) marked complete, including 1 forced from a terminal state: Task 2 was failed)\n"; stdout != want {
		t.Errorf("stdout = %q; want %q", stdout, want)
	}

	body, _ := os.ReadFile(filepath.Join(root, "spec", "plans", "auth.md"))
	if !strings.Contains(string(body), "Overridden from a terminal failure state via --force-tasks:** Task 2 (was failed)") {
		t.Errorf("Resolution must itemize the override:\n%s", body)
	}

	vs, err := lint.Lint(lint.Options{SpecRoot: filepath.Join(root, "spec")})
	if err != nil {
		t.Fatalf("lint: %v", err)
	}
	for _, v := range vs {
		if v.Severity == "error" {
			t.Errorf("unexpected lint error after reconcile with override: %s:%d [%s] %s", v.File, v.Line, v.Rule, v.Message)
		}
	}
}

// AC: lint-failure-rolls-back
func TestPlanReconcile_LintFailureRollsBack_CLI(t *testing.T) {
	root := stageReconcilablePlan(t, "auth", "Draft", "planning")
	orig := lintLintFn
	lintLintFn = func(opts lint.Options) ([]lint.Violation, error) {
		if opts.Fix {
			return nil, nil
		}
		return []lint.Violation{{File: "x", Line: 1, Rule: "P-001", Severity: "error", Message: "boom"}}, nil
	}
	t.Cleanup(func() { lintLintFn = orig })

	_, _, err := runPlan(t, "reconcile", "auth", "--tasks=complete", "--note", "x")
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
	body, _ := os.ReadFile(filepath.Join(root, "spec", "plans", "auth.md"))
	if !strings.Contains(string(body), "**Status:** Implemented") {
		t.Errorf("committed status missing after lint failure:\n%s", body)
	}
	if !strings.Contains(string(body), "**Reconciled:**") {
		t.Errorf("reconciled marker missing from committed transaction:\n%s", body)
	}
}

// Reconcile is an out-of-band record correction, not a way around a Plan's
// execution prerequisites. Its readiness refusal happens before any rewrite.
func TestPlanReconcile_UnmetPrerequisiteRefusesWithoutMutation_CLI(t *testing.T) {
	root := stageReconcilablePlan(t, "delivery", "Draft", "planning")
	deliveryPath := filepath.Join(root, "spec", "plans", "delivery.md")
	body, err := os.ReadFile(deliveryPath)
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.Replace(string(body), "**Status:** Draft", "**Status:** Draft\n**Prerequisite Plans:** foundation", 1))
	if err := os.WriteFile(deliveryPath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	foundation := `# Plan: Foundation

**Status:** Approved

## Tasks

### Task 1: Work

**Status:** queued
`
	if err := os.WriteFile(filepath.Join(root, "spec", "plans", "foundation.md"), []byte(foundation), 0o644); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(deliveryPath)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = runPlan(t, "reconcile", "delivery", "--tasks=complete", "--note", "x")
	if got := exitCodeOfErr(err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidState, err)
	}
	if !strings.Contains(err.Error(), "foundation") || !strings.Contains(err.Error(), "Approved") {
		t.Errorf("diagnostic must name unmet prerequisite/status: %v", err)
	}
	after, readErr := os.ReadFile(deliveryPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != string(original) {
		t.Errorf("reconcile changed plan despite readiness refusal:\n%s", after)
	}
}

// A malformed prerequisite artifact is a lifecycle refusal (4), not an
// operational failure (10), and reconcile must not write any bytes first.
func TestPlanReconcile_PreservesInvalidStateReadinessErrorWithoutMutation_CLI(t *testing.T) {
	root := stageReconcilablePlan(t, "delivery", "Draft", "planning")
	deliveryPath := filepath.Join(root, "spec", "plans", "delivery.md")
	body, err := os.ReadFile(deliveryPath)
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.Replace(string(body), "**Status:** Draft", "**Status:** Draft\n**Prerequisite Plans:** foundation", 1))
	if err := os.WriteFile(deliveryPath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "spec", "plans", "foundation.md"), []byte("# Notes\n\n# Plan: Foundation\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(deliveryPath)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = runPlan(t, "reconcile", "delivery", "--tasks=complete", "--note", "x")
	if got := exitCodeOfErr(err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidState, err)
	}
	after, readErr := os.ReadFile(deliveryPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != string(before) {
		t.Error("reconcile changed plan despite invalid-state readiness refusal")
	}
}

// A --project that does not resolve to a spec repo surfaces the
// resolveSpecRoot error (covers the error-return branch after flag
// validation passes).
func TestPlanReconcile_ProjectResolveError_CLI(t *testing.T) {
	bare := t.TempDir() // no spec/ subtree
	_, _, err := runPlan(t, "reconcile", "auth", "--tasks=complete", "--note", "x", "--project", bare)
	if err == nil {
		t.Fatal("expected resolveSpecRoot error, got nil")
	}
}

// A false all-complete claim is corrected through the same public lifecycle
// surface, not by editing the embedded task status by hand.
func TestPlanReconcile_ReopenTasks_CLI(t *testing.T) {
	root := stageReconcilablePlan(t, "auth", "Implemented", "complete", "complete")
	stdout, stderr, err := runPlan(t, "reconcile", "auth", "--reopen-tasks=2", "--note", "audit found task 2 incomplete", "--evidence", "audit.md")
	if err != nil {
		t.Fatalf("reopen reconcile: %v (stderr=%s)", err, stderr)
	}
	if want := "auth: Implemented → Blocked (reconciled, 1 task(s) marked blocked)\n"; stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
	body, err := os.ReadFile(filepath.Join(root, "spec", "plans", "auth.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "**Status:** Blocked") || !strings.Contains(string(body), "**Status:** blocked") || !strings.Contains(string(body), "audit found task 2 incomplete") {
		t.Fatalf("reopen correction not recorded:\n%s", body)
	}
}

func TestPlanReconcile_ReopenFlagErrors_CLI(t *testing.T) {
	stageReconcilablePlan(t, "auth", "Implemented", "complete")
	for _, args := range [][]string{
		{"reconcile", "auth", "--tasks=complete", "--reopen-tasks=1", "--note", "x"},
		{"reconcile", "auth", "--reopen-tasks=nope", "--note", "x"},
		{"reconcile", "auth", "--reopen-tasks=1", "--force-tasks=1", "--note", "x"},
	} {
		_, _, err := runPlan(t, args...)
		if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
			t.Fatalf("args=%v exit=%d, want %d: %v", args, got, exitcode.InvalidArgs, err)
		}
	}
}

// AC: the execution-band-rejected error message on change-status points the
// user at `plan reconcile`.
func TestPlanChangeStatus_ExecutionBandMessagePointsAtReconcile_CLI(t *testing.T) {
	stagePlan(t, "auth", "Approved")
	_, stderr, err := runPlan(t, "change-status", "auth", "--to=implemented")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidArgs, err)
	}
	if !strings.Contains(err.Error(), "plan reconcile") && !strings.Contains(stderr, "plan reconcile") {
		t.Errorf("expected error to mention `plan reconcile`, got err=%q stderr=%q", err, stderr)
	}
}
