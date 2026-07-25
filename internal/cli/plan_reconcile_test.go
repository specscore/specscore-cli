package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lint"
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
	if !strings.Contains(string(body), "**Status:** Draft") {
		t.Errorf("status not rolled back after lint failure:\n%s", body)
	}
	if strings.Contains(string(body), "**Reconciled:**") {
		t.Errorf("reconciled marker must not survive rollback:\n%s", body)
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
