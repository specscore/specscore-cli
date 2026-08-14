package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// amendBoardImplementedBy unit: a missing file surfaces the ReadFile error
// (defensive; the verb reads status first, so this is unreachable via the CLI).
func TestAmendBoardImplementedBy_ReadError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.md")
	if err := amendBoardImplementedBy(missing, "x@y"); err == nil {
		t.Fatal("expected ReadFile error, got nil")
	}
}

// amendBoardImplementedBy unit: a file with no **Status:** line is an error.
func TestAmendBoardImplementedBy_NoStatusLine(t *testing.T) {
	p := filepath.Join(t.TempDir(), "README.md")
	if err := os.WriteFile(p, []byte("# Auth\n\nNo status.\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := amendBoardImplementedBy(p, "x@y"); err == nil {
		t.Fatal("expected error for missing status line, got nil")
	}
}

// amendPlanImplementedBy unit: a missing file surfaces the ReadFile error.
func TestAmendPlanImplementedBy_ReadError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.md")
	if err := amendPlanImplementedBy(missing, 1, "x@y"); err == nil {
		t.Fatal("expected ReadFile error, got nil")
	}
}

// implementedByCount returns how many `**Implemented-by:**` lines a file carries.
func implementedByCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	n := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "**Implemented-by:**") {
			n++
		}
	}
	return n
}

// completeTaskPlanBody is a plan whose "setup" block is already complete and
// carries a (wrong) provenance ref; "deploy" is planning.
const completeTaskPlanBody = `# Plan: Auth

**Status:** Executing
**Source Feature:** auth

## Tasks

### Task 1: Setup

**Id:** setup
**Status:** complete
**Implemented-by:** backstage@wrongsha
**Depends-On:** —

Setup body.

### Task 2: Deploy

**Id:** deploy
**Status:** planning
**Depends-On:** 1

Deploy body.
`

// AC corrective-restamp (board): amend overwrites a wrong ref on an already-
// complete task without changing the status, and prints "<task>: provenance
// amended".
func TestTaskChangeStatus_Board_AmendOverwrites(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "complete")
	if err := writeBoardImplementedBy(taskFile, "backstage@wrongsha"); err != nil {
		t.Fatalf("seed provenance: %v", err)
	}
	stdout, stderr, err := runTask(t, "change-status", "auth", "--amend-provenance",
		"--repo", "backstage", "--commit", "a1b2c3d")
	if err != nil {
		t.Fatalf("change-status: %v (stderr=%s)", err, stderr)
	}
	if want := "auth: provenance amended\n"; stdout != want {
		t.Errorf("stdout = %q; want %q", stdout, want)
	}
	if got := taskFileStatus(t, taskFile); got != "complete" {
		t.Errorf("status = %q; want complete (unchanged)", got)
	}
	if got := implementedByField(t, taskFile); got != "backstage@a1b2c3d" {
		t.Errorf("implemented-by = %q; want backstage@a1b2c3d", got)
	}
	if n := implementedByCount(t, taskFile); n != 1 {
		t.Errorf("implemented-by lines = %d; want exactly 1", n)
	}
}

// AC corrective-restamp ("or clears it"): amend with no provenance flags removes
// the existing **Implemented-by:** field.
func TestTaskChangeStatus_Board_AmendClears(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "complete")
	if err := writeBoardImplementedBy(taskFile, "backstage@wrongsha"); err != nil {
		t.Fatalf("seed provenance: %v", err)
	}
	stdout, _, err := runTask(t, "change-status", "auth", "--amend-provenance")
	if err != nil {
		t.Fatalf("change-status: %v", err)
	}
	if want := "auth: provenance amended\n"; stdout != want {
		t.Errorf("stdout = %q; want %q", stdout, want)
	}
	if got := taskFileStatus(t, taskFile); got != "complete" {
		t.Errorf("status = %q; want complete (unchanged)", got)
	}
	if got := implementedByField(t, taskFile); got != "" {
		t.Errorf("implemented-by = %q; want cleared", got)
	}
}

// Amend inserts a field on a complete task that has none.
func TestTaskChangeStatus_Board_AmendInsertsWhenAbsent(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "complete")
	_, _, err := runTask(t, "change-status", "auth", "--amend-provenance", "--commit", "deadbee")
	if err != nil {
		t.Fatalf("change-status: %v", err)
	}
	if got := implementedByField(t, taskFile); got != "deadbee" {
		t.Errorf("implemented-by = %q; want deadbee", got)
	}
}

// AC corrective-restamp: --amend-provenance with --to set exits 2 (InvalidArgs).
func TestTaskChangeStatus_Board_AmendWithToRejected(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "complete")
	if err := writeBoardImplementedBy(taskFile, "backstage@wrongsha"); err != nil {
		t.Fatalf("seed provenance: %v", err)
	}
	_, _, err := runTask(t, "change-status", "auth", "--amend-provenance",
		"--to=failed", "--commit", "a1b2c3d")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d (InvalidArgs); err=%v", got, exitcode.InvalidArgs, err)
	}
	if got := implementedByField(t, taskFile); got != "backstage@wrongsha" {
		t.Errorf("implemented-by = %q; want backstage@wrongsha (unchanged)", got)
	}
}

// AC corrective-restamp: --amend-provenance on a non-complete task exits 4.
func TestTaskChangeStatus_Board_AmendNonCompleteRejected(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "in_progress")
	_, _, err := runTask(t, "change-status", "auth", "--amend-provenance", "--commit", "a1b2c3d")
	if got := exitCodeOfErr(err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d (InvalidState); err=%v", got, exitcode.InvalidState, err)
	}
	if got := taskFileStatus(t, taskFile); got != "in_progress" {
		t.Errorf("status = %q; want in_progress (unchanged)", got)
	}
	if got := implementedByField(t, taskFile); got != "" {
		t.Errorf("implemented-by = %q; want none (unchanged)", got)
	}
}

// Amend with provenance flags but no --commit exits 2, leaving the task unchanged.
func TestTaskChangeStatus_Board_AmendWithoutCommitRejected(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "complete")
	if err := writeBoardImplementedBy(taskFile, "backstage@wrongsha"); err != nil {
		t.Fatalf("seed provenance: %v", err)
	}
	_, _, err := runTask(t, "change-status", "auth", "--amend-provenance", "--repo", "backstage")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d (InvalidArgs); err=%v", got, exitcode.InvalidArgs, err)
	}
	if !strings.Contains(err.Error(), "--commit") {
		t.Errorf("error should name --commit: %v", err)
	}
	if got := implementedByField(t, taskFile); got != "backstage@wrongsha" {
		t.Errorf("implemented-by = %q; want backstage@wrongsha (unchanged)", got)
	}
}

// Amend on a missing board task surfaces NotFound (3).
func TestTaskChangeStatus_Board_AmendTaskNotFound(t *testing.T) {
	stageTaskWithStatus(t, "auth", "complete")
	_, _, err := runTask(t, "change-status", "ghost", "--amend-provenance", "--commit", "a1b2c3d")
	if got := exitCodeOfErr(err); got != exitcode.NotFound {
		t.Errorf("exit = %d, want %d (NotFound); err=%v", got, exitcode.NotFound, err)
	}
}

// Amend on a board task whose file has no **Status:** line surfaces Unexpected (10).
func TestTaskChangeStatus_Board_AmendNoStatusLine(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "complete")
	noStatus := "# Auth\n\nBody only.\n"
	if err := os.WriteFile(taskFile, []byte(noStatus), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _, err := runTask(t, "change-status", "auth", "--amend-provenance", "--commit", "a1b2c3d")
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected); err=%v", got, exitcode.Unexpected, err)
	}
}

// A board amend write failure surfaces Unexpected (10).
func TestTaskChangeStatus_Board_AmendWriteFailure(t *testing.T) {
	stageTaskWithStatus(t, "auth", "complete")
	boom := errors.New("atomic transaction boom")

	_, _, err := runTaskWithMutationDeps(t, taskMutationDeps{transformArtifact: func(string, func([]byte) ([]byte, error)) error { return boom }}, "change-status", "auth", "--amend-provenance", "--commit", "a1b2c3d")
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected); err=%v", got, exitcode.Unexpected, err)
	}
	if !errors.Is(err, boom) {
		t.Fatalf("error lost transaction cause: %v", err)
	}
}

// A board amend under a project that does not resolve surfaces the resolve error.
func TestTaskChangeStatus_Board_AmendProjectResolveError(t *testing.T) {
	stageTaskWithStatus(t, "auth", "complete")
	bare := t.TempDir()
	_, _, err := runTask(t, "change-status", "auth", "--amend-provenance",
		"--commit", "a1b2c3d", "--project", bare)
	if err == nil {
		t.Fatal("expected resolve error, got nil")
	}
}

// --- plan-inline amend ---

// AC corrective-restamp (plan-inline): amend overwrites a wrong ref on a complete
// block without changing status; the sibling block is untouched.
func TestTaskChangeStatus_PlanInline_AmendOverwrites(t *testing.T) {
	_, planPath := stagePlanWithTasks(t, "auth", completeTaskPlanBody)
	stdout, _, err := runTask(t, "change-status", "setup", "--plan", "auth",
		"--amend-provenance", "--repo", "backstage", "--commit", "a1b2c3d")
	if err != nil {
		t.Fatalf("change-status: %v", err)
	}
	if want := "setup: provenance amended\n"; stdout != want {
		t.Errorf("stdout = %q; want %q", stdout, want)
	}
	if got := planTaskStatus(t, planPath, "setup"); got != "complete" {
		t.Errorf("setup status = %q; want complete (unchanged)", got)
	}
	if got := implementedByField(t, planPath); got != "backstage@a1b2c3d" {
		t.Errorf("implemented-by = %q; want backstage@a1b2c3d", got)
	}
	if n := implementedByCount(t, planPath); n != 1 {
		t.Errorf("implemented-by lines = %d; want exactly 1", n)
	}
	if got := planTaskStatus(t, planPath, "deploy"); got != "planning" {
		t.Errorf("deploy changed: %q", got)
	}
}

// Plan-inline amend with no provenance flags clears the field.
func TestTaskChangeStatus_PlanInline_AmendClears(t *testing.T) {
	_, planPath := stagePlanWithTasks(t, "auth", completeTaskPlanBody)
	_, _, err := runTask(t, "change-status", "setup", "--plan", "auth", "--amend-provenance")
	if err != nil {
		t.Fatalf("change-status: %v", err)
	}
	if got := planTaskStatus(t, planPath, "setup"); got != "complete" {
		t.Errorf("setup status = %q; want complete", got)
	}
	if got := implementedByField(t, planPath); got != "" {
		t.Errorf("implemented-by = %q; want cleared", got)
	}
}

// Plan-inline amend on a non-complete block exits 4.
func TestTaskChangeStatus_PlanInline_AmendNonCompleteRejected(t *testing.T) {
	_, planPath := stagePlanWithTasks(t, "auth", twoTaskPlanBody)
	_, _, err := runTask(t, "change-status", "setup", "--plan", "auth",
		"--amend-provenance", "--commit", "a1b2c3d")
	if got := exitCodeOfErr(err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d (InvalidState); err=%v", got, exitcode.InvalidState, err)
	}
	if got := planTaskStatus(t, planPath, "setup"); got != "in_progress" {
		t.Errorf("setup changed: %q; want in_progress (unchanged)", got)
	}
}

// Plan-inline amend on a missing plan exits 3 (NotFound).
func TestTaskChangeStatus_PlanInline_AmendMissingPlan(t *testing.T) {
	stagePlanWithTasks(t, "auth", completeTaskPlanBody)
	_, _, err := runTask(t, "change-status", "setup", "--plan", "ghost",
		"--amend-provenance", "--commit", "a1b2c3d")
	if got := exitCodeOfErr(err); got != exitcode.NotFound {
		t.Errorf("exit = %d, want %d (NotFound); err=%v", got, exitcode.NotFound, err)
	}
}

// Plan-inline amend with no matching **Id:** exits 3 (NotFound).
func TestTaskChangeStatus_PlanInline_AmendNoMatchingId(t *testing.T) {
	stagePlanWithTasks(t, "auth", completeTaskPlanBody)
	_, _, err := runTask(t, "change-status", "ghost", "--plan", "auth",
		"--amend-provenance", "--commit", "a1b2c3d")
	if got := exitCodeOfErr(err); got != exitcode.NotFound {
		t.Errorf("exit = %d, want %d (NotFound); err=%v", got, exitcode.NotFound, err)
	}
}

// Plan-inline amend with an unreadable plan surfaces Unexpected (10).
func TestTaskChangeStatus_PlanInline_AmendUnreadablePlan(t *testing.T) {
	_, planPath := stagePlanWithTasks(t, "auth", completeTaskPlanBody)
	if err := os.Chmod(planPath, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(planPath, 0o644) })

	_, _, err := runTask(t, "change-status", "setup", "--plan", "auth",
		"--amend-provenance", "--commit", "a1b2c3d")
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected); err=%v", got, exitcode.Unexpected, err)
	}
}

// Plan-inline amend whose post-validation write fails surfaces Unexpected (10).
func TestTaskChangeStatus_PlanInline_AmendWriteFailure(t *testing.T) {
	stagePlanWithTasks(t, "auth", completeTaskPlanBody)
	boom := errors.New("atomic transaction boom")

	_, _, err := runTaskWithMutationDeps(t, taskMutationDeps{transformArtifact: func(string, func([]byte) ([]byte, error)) error { return boom }}, "change-status", "setup", "--plan", "auth",
		"--amend-provenance", "--commit", "a1b2c3d")
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected); err=%v", got, exitcode.Unexpected, err)
	}
	if !errors.Is(err, boom) {
		t.Fatalf("error lost transaction cause: %v", err)
	}
}

// Plan-inline amend under a project that does not resolve surfaces the error.
func TestTaskChangeStatus_PlanInline_AmendProjectResolveError(t *testing.T) {
	stagePlanWithTasks(t, "auth", completeTaskPlanBody)
	bare := t.TempDir()
	_, _, err := runTask(t, "change-status", "setup", "--plan", "auth",
		"--amend-provenance", "--commit", "a1b2c3d", "--project", bare)
	if err == nil {
		t.Fatal("expected resolve error, got nil")
	}
}
