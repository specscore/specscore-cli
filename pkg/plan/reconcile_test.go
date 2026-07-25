package plan

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// reconcilePlanBody returns a minimal lint-shaped flat Plan body in the given
// status, with one `### Task N:` block per entry in taskStatuses. A task
// status of "" omits the **Status:** line entirely (an unset task).
func reconcilePlanBody(status string, taskStatuses ...string) string {
	var b strings.Builder
	b.WriteString("---\nformat: " + FormatURL + "\nstatus: " + status + "\n---\n\n")
	b.WriteString("# Plan: Auth\n\n")
	b.WriteString("**Status:** " + status + "\n")
	b.WriteString("**Source:** none\n")
	b.WriteString("**Date:** 2026-06-17\n")
	b.WriteString("**Owner:** alex\n")
	b.WriteString("**Supersedes:** —\n\n")
	b.WriteString("## Summary\n\nAuth.\n\n")
	b.WriteString("## Approach\n\nOne task per step.\n\n")
	b.WriteString("## Tasks\n\n")
	for i, s := range taskStatuses {
		fmt.Fprintf(&b, "### Task %d: Step %d\n\n", i+1, i+1)
		if s != "" {
			fmt.Fprintf(&b, "**Status:** %s\n\n", s)
		} else {
			b.WriteString("No status field on this one.\n\n")
		}
	}
	b.WriteString("## Open Questions\n\nNone at this time.\n\n")
	b.WriteString("---\n*This document follows the " + FormatURL + "*\n")
	return b.String()
}

// stageReconcilePlan writes a flat plan at spec/plans/<slug>.md under a fresh
// SpecRoot and returns (specRoot, planPath).
func stageReconcilePlan(t *testing.T, slug, status string, taskStatuses ...string) (string, string) {
	t.Helper()
	root := t.TempDir()
	plansDir := filepath.Join(root, "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(plansDir, slug+".md")
	if err := os.WriteFile(path, []byte(reconcilePlanBody(status, taskStatuses...)), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	return root, path
}

func eightPlanning() []string {
	out := make([]string, 8)
	for i := range out {
		out[i] = "planning"
	}
	return out
}

// AC: happy-path-eight-tasks — the founder's motivating scenario: a Draft
// plan whose 8 embedded tasks are all still "planning" gets reconciled to
// Implemented, every task flips to complete, and the artifact records why.
func TestReconcile_HappyPath_EightTasksDraftToImplemented(t *testing.T) {
	pinReconcileDate(t, "2026-07-25")
	root, path := stageReconcilePlan(t, "auth", "Draft", eightPlanning()...)

	res, err := Reconcile(ReconcileOptions{
		SpecRoot: root, Slug: "auth",
		Note:         "implemented directly on main during the outage; tracked flow was skipped",
		Evidence:     []string{"a1b2c3d", "https://github.com/org/repo/pull/42"},
		PostMutation: okHook,
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.From != lifecycle.PlanDraft || res.To != lifecycle.PlanImplemented || res.TasksReconciled != 8 {
		t.Fatalf("result = %+v", res)
	}

	body, _ := os.ReadFile(path)
	s := string(body)
	if !strings.Contains(s, "**Status:** Implemented\n") {
		t.Errorf("plan status not rewritten:\n%s", s)
	}
	if strings.Count(s, "**Status:** complete") != 8 {
		t.Errorf("expected 8 task lines rewritten to complete:\n%s", s)
	}
	if strings.Count(s, "**Status:** planning") != 0 {
		t.Errorf("no task should remain planning:\n%s", s)
	}
	if !strings.Contains(s, "**Reconciled:** 2026-07-25") {
		t.Errorf("missing **Reconciled:** marker:\n%s", s)
	}
	if !strings.Contains(s, "## Resolution") {
		t.Errorf("missing ## Resolution section:\n%s", s)
	}
	if !strings.Contains(s, "Reconciled Draft → Implemented outside the tracked `change-status` flow") {
		t.Errorf("resolution paragraph missing reconciliation preamble:\n%s", s)
	}
	if !strings.Contains(s, "tracked flow was skipped") {
		t.Errorf("resolution paragraph missing caller note:\n%s", s)
	}
	if !strings.Contains(s, "Evidence: a1b2c3d, https://github.com/org/repo/pull/42") {
		t.Errorf("resolution paragraph missing evidence:\n%s", s)
	}
	// The marker is inserted immediately after the **Status:** line.
	idx := strings.Index(s, "**Status:** Implemented\n")
	markerIdx := strings.Index(s, "**Reconciled:**")
	if markerIdx <= idx {
		t.Errorf("marker must follow the Status line: statusIdx=%d markerIdx=%d", idx, markerIdx)
	}
}

// AC: no-evidence — Evidence is genuinely optional; when omitted, no
// "Evidence:" line is written.
func TestReconcile_NoEvidence_NoEvidenceLineWritten(t *testing.T) {
	root, path := stageReconcilePlan(t, "auth", "Approved", "planning")
	_, err := Reconcile(ReconcileOptions{
		SpecRoot: root, Slug: "auth", Note: "shipped ahead of the record", PostMutation: okHook,
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	body, _ := os.ReadFile(path)
	if strings.Contains(string(body), "Evidence:") {
		t.Errorf("no Evidence line expected:\n%s", body)
	}
}

// AC: partial-completion — some tasks already complete; only the remaining
// ones are rewritten, and TasksReconciled counts only the changed ones.
func TestReconcile_PartialCompletion(t *testing.T) {
	root, path := stageReconcilePlan(t, "auth", "Approved", "complete", "planning", "in_progress")
	res, err := Reconcile(ReconcileOptions{
		SpecRoot: root, Slug: "auth", Note: "the rest landed too", PostMutation: okHook,
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.TasksReconciled != 2 {
		t.Errorf("TasksReconciled = %d, want 2", res.TasksReconciled)
	}
	body, _ := os.ReadFile(path)
	if strings.Count(string(body), "**Status:** complete") != 3 {
		t.Errorf("expected 3 complete task lines:\n%s", body)
	}
}

// AC: tasks-already-complete-status-still-updates — every task is already
// complete, but the plan's own Status field lagged (e.g. a prior reconcile's
// PostMutation rolled back). Reconcile still fixes the plan-level line and
// writes the record, with TasksReconciled == 0.
func TestReconcile_TasksAlreadyComplete_OnlyPlanStatusChanges(t *testing.T) {
	root, path := stageReconcilePlan(t, "auth", "Draft", "complete", "complete")
	res, err := Reconcile(ReconcileOptions{
		SpecRoot: root, Slug: "auth", Note: "status field never caught up", PostMutation: okHook,
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.TasksReconciled != 0 {
		t.Errorf("TasksReconciled = %d, want 0", res.TasksReconciled)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "**Status:** Implemented\n") {
		t.Errorf("plan status not corrected:\n%s", body)
	}
}

// AC: missing-note-refused — Reconcile refuses without a justification.
func TestReconcile_MissingNote_Refused(t *testing.T) {
	root, _ := stageReconcilePlan(t, "auth", "Draft", "planning")
	_, err := Reconcile(ReconcileOptions{SpecRoot: root, Slug: "auth", PostMutation: okHook})
	if got := codeOf(t, err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidArgs, err)
	}
}

// AC: unknown-slug — a slug that resolves to no plan at all exits 3.
func TestReconcile_UnknownSlug_NotFound(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "spec", "plans"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := Reconcile(ReconcileOptions{SpecRoot: root, Slug: "ghost", Note: "x", PostMutation: okHook})
	if got := codeOf(t, err); got != exitcode.NotFound {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.NotFound, err)
	}
}

// AC: directory-form-unsupported — reconcile only supports the flat
// single-file form; a plan that resolves to the directory form is refused.
func TestReconcile_DirectoryForm_Refused(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "spec", "plans", "auth")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "# Plan: Auth\n\n**Status:** Draft\n"
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := Reconcile(ReconcileOptions{SpecRoot: root, Slug: "auth", Note: "x", PostMutation: okHook})
	if got := codeOf(t, err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidState, err)
	}
}

// AC: no-status-line — a plan that parses (has a title) but carries no
// **Status:** line at all.
func TestReconcile_NoStatusLine(t *testing.T) {
	root := t.TempDir()
	plansDir := filepath.Join(root, "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(plansDir, "auth.md"), []byte("# Plan: Auth\n\nNo status here.\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := Reconcile(ReconcileOptions{SpecRoot: root, Slug: "auth", Note: "x", PostMutation: okHook})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
}

// AC: disposition-not-resurrected — a plan in any terminal disposition status
// is refused; reconcile does not resurrect dispositions.
func TestReconcile_DispositionStatus_Refused(t *testing.T) {
	for _, st := range []string{"Rejected", "Withdrawn", "Superseded", "Deprecated"} {
		t.Run(st, func(t *testing.T) {
			root, _ := stageReconcilePlan(t, "auth", st, "complete")
			_, err := Reconcile(ReconcileOptions{SpecRoot: root, Slug: "auth", Note: "x", PostMutation: okHook})
			if got := codeOf(t, err); got != exitcode.InvalidState {
				t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidState, err)
			}
		})
	}
}

// AC: no-embedded-tasks — a plan with no `## Tasks` entries has nothing for
// reconcile to derive a status from.
func TestReconcile_NoEmbeddedTasks_Refused(t *testing.T) {
	root, _ := stageReconcilePlan(t, "auth", "Draft")
	_, err := Reconcile(ReconcileOptions{SpecRoot: root, Slug: "auth", Note: "x", PostMutation: okHook})
	if got := codeOf(t, err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidState, err)
	}
	if !strings.Contains(err.Error(), "no embedded tasks") {
		t.Errorf("expected 'no embedded tasks' message, got: %q", err.Error())
	}
}

// AC: task-missing-status-line — a task block with no **Status:** field
// cannot be reconciled; the error names the offending task number.
func TestReconcile_TaskMissingStatusLine_Refused(t *testing.T) {
	root, _ := stageReconcilePlan(t, "auth", "Draft", "planning", "")
	_, err := Reconcile(ReconcileOptions{SpecRoot: root, Slug: "auth", Note: "x", PostMutation: okHook})
	if got := codeOf(t, err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidState, err)
	}
	if !strings.Contains(err.Error(), "Task 2") {
		t.Errorf("expected message to name Task 2, got: %q", err.Error())
	}
}

// AC: failed-task-refused — --tasks=complete must never silently overwrite a
// task recorded as failed; the error names the task number and its status.
func TestReconcile_FailedTask_RefusedWithoutForceTasks(t *testing.T) {
	root, path := stageReconcilePlan(t, "auth", "Approved", "complete", "failed")
	before, _ := os.ReadFile(path)

	_, err := Reconcile(ReconcileOptions{SpecRoot: root, Slug: "auth", Note: "x", PostMutation: okHook})
	if got := codeOf(t, err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidState, err)
	}
	if !strings.Contains(err.Error(), "Task 2 (failed)") {
		t.Errorf("expected message to name Task 2 (failed), got: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "--force-tasks=2") {
		t.Errorf("expected message to suggest --force-tasks=2, got: %q", err.Error())
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Errorf("plan must be unchanged on refusal:\nbefore=%s\nafter=%s", before, after)
	}
}

// AC: aborted-task-refused — same guard for the aborted terminal status.
func TestReconcile_AbortedTask_RefusedWithoutForceTasks(t *testing.T) {
	root, _ := stageReconcilePlan(t, "auth", "Approved", "aborted")
	_, err := Reconcile(ReconcileOptions{SpecRoot: root, Slug: "auth", Note: "x", PostMutation: okHook})
	if got := codeOf(t, err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidState, err)
	}
	if !strings.Contains(err.Error(), "Task 1 (aborted)") {
		t.Errorf("expected message to name Task 1 (aborted), got: %q", err.Error())
	}
}

// AC: multiple-terminal-tasks-all-named — the refusal names every offending
// task, not just the first.
func TestReconcile_MultipleTerminalTasks_AllNamed(t *testing.T) {
	root, _ := stageReconcilePlan(t, "auth", "Approved", "failed", "planning", "aborted")
	_, err := Reconcile(ReconcileOptions{SpecRoot: root, Slug: "auth", Note: "x", PostMutation: okHook})
	if got := codeOf(t, err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidState, err)
	}
	for _, want := range []string{"Task 1 (failed)", "Task 3 (aborted)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected message to name %q, got: %q", want, err.Error())
		}
	}
	if strings.Contains(err.Error(), "Task 2") {
		t.Errorf("planning task must not be named as a blocker: %q", err.Error())
	}
}

// AC: blocked-task-not-guarded — a blocked task needs no acknowledgement; it
// is not a terminal claim (REQ: leave blocked as-is).
func TestReconcile_BlockedTask_NoGuardNeeded(t *testing.T) {
	root, _ := stageReconcilePlan(t, "auth", "Approved", "blocked")
	res, err := Reconcile(ReconcileOptions{SpecRoot: root, Slug: "auth", Note: "x", PostMutation: okHook})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(res.Overrides) != 0 {
		t.Errorf("blocked task must not be treated as an override: %+v", res.Overrides)
	}
}

// AC: force-tasks-override-succeeds — naming the failed task's number via
// ForceTasks lets reconcile proceed, and the override is reported both in the
// result and, itemized by number and prior status, in the Resolution
// paragraph — never folded silently into the aggregate count alone.
func TestReconcile_ForceTasks_OverrideSucceeds(t *testing.T) {
	root, path := stageReconcilePlan(t, "auth", "Approved", "complete", "failed", "aborted")
	res, err := Reconcile(ReconcileOptions{
		SpecRoot: root, Slug: "auth", Note: "both actually landed on retry",
		ForceTasks: []int{2, 3}, PostMutation: okHook,
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.TasksReconciled != 2 {
		t.Errorf("TasksReconciled = %d, want 2", res.TasksReconciled)
	}
	if len(res.Overrides) != 2 {
		t.Fatalf("Overrides = %+v, want 2 entries", res.Overrides)
	}
	if res.Overrides[0].Number != 2 || res.Overrides[0].From != StatusFailed {
		t.Errorf("Overrides[0] = %+v, want {2 failed}", res.Overrides[0])
	}
	if res.Overrides[1].Number != 3 || res.Overrides[1].From != StatusAborted {
		t.Errorf("Overrides[1] = %+v, want {3 aborted}", res.Overrides[1])
	}

	body, _ := os.ReadFile(path)
	s := string(body)
	if strings.Count(s, "**Status:** complete") != 3 {
		t.Errorf("expected all 3 tasks complete:\n%s", s)
	}
	if !strings.Contains(s, "**Overridden from a terminal failure state via --force-tasks:** Task 2 (was failed), Task 3 (was aborted).") {
		t.Errorf("Resolution must itemize the overridden tasks by number and prior status:\n%s", s)
	}
}

// AC: force-tasks-partial-ack — naming only SOME of the failed/aborted tasks
// still blocks on the un-acknowledged one.
func TestReconcile_ForceTasks_PartialAcknowledgement_StillBlocked(t *testing.T) {
	root, _ := stageReconcilePlan(t, "auth", "Approved", "failed", "aborted")
	_, err := Reconcile(ReconcileOptions{
		SpecRoot: root, Slug: "auth", Note: "only task 1 actually landed",
		ForceTasks: []int{1}, PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidState, err)
	}
	if !strings.Contains(err.Error(), "Task 2 (aborted)") {
		t.Errorf("expected message to still name Task 2 (aborted), got: %q", err.Error())
	}
	if strings.Contains(err.Error(), "Task 1") {
		t.Errorf("acknowledged Task 1 must not be named as a blocker: %q", err.Error())
	}
}

// AC: force-tasks-irrelevant-numbers-are-harmless — a --force-tasks entry
// that does not correspond to a failed/aborted task is a no-op, not an error.
func TestReconcile_ForceTasks_IrrelevantNumberIsHarmless(t *testing.T) {
	root, _ := stageReconcilePlan(t, "auth", "Approved", "planning")
	res, err := Reconcile(ReconcileOptions{
		SpecRoot: root, Slug: "auth", Note: "x", ForceTasks: []int{99}, PostMutation: okHook,
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(res.Overrides) != 0 {
		t.Errorf("expected no overrides for an irrelevant --force-tasks number, got %+v", res.Overrides)
	}
}

// AC: re-run-refused — running reconcile again on an already-reconciled plan
// is a no-op refusal (exit 4), not a silent success.
func TestReconcile_ReRun_Refused(t *testing.T) {
	root, _ := stageReconcilePlan(t, "auth", "Draft", "planning", "planning")
	if _, err := Reconcile(ReconcileOptions{SpecRoot: root, Slug: "auth", Note: "first pass", PostMutation: okHook}); err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}
	_, err := Reconcile(ReconcileOptions{SpecRoot: root, Slug: "auth", Note: "second pass", PostMutation: okHook})
	if got := codeOf(t, err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidState, err)
	}
	if !strings.Contains(err.Error(), "already reconciled") {
		t.Errorf("expected 'already reconciled' message, got: %q", err.Error())
	}
}

// AC: second-reconcile-after-new-work — a plan reconciled once, then a new
// task is appended and left at planning, is reconciled again successfully.
// The **Reconciled:** marker is NOT duplicated; the new Resolution paragraph
// is appended alongside the first.
func TestReconcile_SecondReconcile_AfterNewTaskAdded(t *testing.T) {
	root, path := stageReconcilePlan(t, "auth", "Draft", "planning")
	if _, err := Reconcile(ReconcileOptions{SpecRoot: root, Slug: "auth", Note: "first pass", PostMutation: okHook}); err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}

	// Append a second task, left at planning, simulating more work landing
	// after the first reconciliation.
	body, _ := os.ReadFile(path)
	patched := strings.Replace(string(body), "## Open Questions",
		"### Task 2: Follow-up\n\n**Status:** planning\n\n## Open Questions", 1)
	if err := os.WriteFile(path, []byte(patched), 0o644); err != nil {
		t.Fatalf("append task: %v", err)
	}

	res, err := Reconcile(ReconcileOptions{SpecRoot: root, Slug: "auth", Note: "second pass", PostMutation: okHook})
	if err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}
	if res.TasksReconciled != 1 {
		t.Errorf("TasksReconciled = %d, want 1", res.TasksReconciled)
	}
	final, _ := os.ReadFile(path)
	if strings.Count(string(final), "**Reconciled:**") != 1 {
		t.Errorf("marker must not be duplicated:\n%s", final)
	}
	if !strings.Contains(string(final), "first pass") || !strings.Contains(string(final), "second pass") {
		t.Errorf("both reconciliation notes must be preserved:\n%s", final)
	}
}

// AC: post-mutation-failure-rolls-back — a failing PostMutation hook (e.g.
// spec lint) restores the pre-invocation file bytes exactly.
func TestReconcile_PostMutationFails_RollsBack(t *testing.T) {
	root, path := stageReconcilePlan(t, "auth", "Draft", "planning", "planning")
	before, _ := os.ReadFile(path)
	boom := errors.New("lint failed")
	_, err := Reconcile(ReconcileOptions{
		SpecRoot: root, Slug: "auth", Note: "x", PostMutation: func() error { return boom },
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom error, got %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Errorf("file not rolled back:\nbefore=%s\nafter=%s", before, after)
	}
}

// AC: note-write-failure-rolls-back — injects a failure into the shared
// appendNoteFn seam (also used by ChangeStatus) to cover the resolution-note
// rollback branch.
func TestReconcile_NoteWriteFails_RollsBack(t *testing.T) {
	root, path := stageReconcilePlan(t, "auth", "Draft", "planning")
	before, _ := os.ReadFile(path)
	orig := appendNoteFn
	appendNoteFn = func(string, string) ([]byte, bool, error) {
		return nil, false, errors.New("note write boom")
	}
	t.Cleanup(func() { appendNoteFn = orig })

	_, err := Reconcile(ReconcileOptions{SpecRoot: root, Slug: "auth", Note: "x", PostMutation: okHook})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Errorf("file not rolled back after note-write failure:\nbefore=%s\nafter=%s", before, after)
	}
}

// AC: parse-error — os.Open fails (permission denied) after resolvePlanFile's
// Stat already confirmed the file exists.
func TestReconcile_ParseError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission bits are not enforced for root")
	}
	root, path := stageReconcilePlan(t, "auth", "Draft", "planning")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	_, err := Reconcile(ReconcileOptions{SpecRoot: root, Slug: "auth", Note: "x", PostMutation: okHook})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
	if !strings.Contains(err.Error(), "parsing plan") {
		t.Errorf("expected parsing error, got: %q", err.Error())
	}
}

// AC: read-original-snapshot-error — injects a failure into the
// reconcileReadOriginalFn seam, covering the branch a real filesystem race
// would otherwise be needed to reach.
func TestReconcile_ReadOriginalSnapshotError(t *testing.T) {
	root, _ := stageReconcilePlan(t, "auth", "Draft", "planning")
	orig := reconcileReadOriginalFn
	reconcileReadOriginalFn = func(string) ([]byte, error) {
		return nil, errors.New("read boom")
	}
	t.Cleanup(func() { reconcileReadOriginalFn = orig })

	_, err := Reconcile(ReconcileOptions{SpecRoot: root, Slug: "auth", Note: "x", PostMutation: okHook})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
	if !strings.Contains(err.Error(), "reading plan") {
		t.Errorf("expected reading-plan error, got: %q", err.Error())
	}
}

// AC: write-plan-error — the target file itself is read-only, so the
// combined status-line rewrite's os.WriteFile fails.
func TestReconcile_WritePlanError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission bits are not enforced for root")
	}
	root, path := stageReconcilePlan(t, "auth", "Draft", "planning")
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	_, err := Reconcile(ReconcileOptions{SpecRoot: root, Slug: "auth", Note: "x", PostMutation: okHook})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
	if !strings.Contains(err.Error(), "writing plan") {
		t.Errorf("expected writing-plan error, got: %q", err.Error())
	}
}

// TestReconcile_GuardErrors covers the defensive required-field checks —
// mirrors TestChangeStatus_GuardErrors in transitions_test.go.
func TestReconcile_GuardErrors(t *testing.T) {
	cases := []struct {
		name string
		opts ReconcileOptions
		want int
	}{
		{"no-specroot", ReconcileOptions{Slug: "a", Note: "x", PostMutation: okHook}, exitcode.Unexpected},
		{"no-slug", ReconcileOptions{SpecRoot: "/x", Note: "x", PostMutation: okHook}, exitcode.InvalidArgs},
		{"no-note", ReconcileOptions{SpecRoot: "/x", Slug: "a", PostMutation: okHook}, exitcode.InvalidArgs},
		{"blank-note", ReconcileOptions{SpecRoot: "/x", Slug: "a", Note: "   ", PostMutation: okHook}, exitcode.InvalidArgs},
		{"no-hook", ReconcileOptions{SpecRoot: "/x", Slug: "a", Note: "x"}, exitcode.Unexpected},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Reconcile(tc.opts)
			if got := codeOf(t, err); got != tc.want {
				t.Errorf("exit = %d, want %d", got, tc.want)
			}
		})
	}
}

// pinReconcileDate replaces reconcileTodayUTC for the duration of the test.
func pinReconcileDate(t *testing.T, date string) {
	t.Helper()
	orig := reconcileTodayUTC
	reconcileTodayUTC = func() string { return date }
	t.Cleanup(func() { reconcileTodayUTC = orig })
}
