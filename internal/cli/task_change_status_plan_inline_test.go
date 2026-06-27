package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// Covers rewritePlanTaskStatusLine's ReadFile error branch directly (a missing
// file cannot arise post-parse, so it is exercised as a unit).
func TestRewritePlanTaskStatusLine_ReadError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.md")
	if err := rewritePlanTaskStatusLine(missing, 1, lifecycle.TaskComplete, ""); err == nil {
		t.Fatal("expected ReadFile error for missing file, got nil")
	}
}

// stagePlanWithTasks writes a SpecScore project with a single-file plan at
// spec/plans/<slug>.md containing the given body, sets cwd to the root, and
// returns the root and the plan-file path.
func stagePlanWithTasks(t *testing.T, slug, body string) (string, string) {
	t.Helper()
	root := t.TempDir()
	withCwd(t, root)

	_ = os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte("name: test\n"), 0o644)
	plansDir := filepath.Join(root, "spec", "plans")
	_ = os.MkdirAll(plansDir, 0o755)
	planPath := filepath.Join(plansDir, slug+".md")
	_ = os.WriteFile(planPath, []byte(body), 0o644)
	return root, planPath
}

// twoTaskPlanBody is a plan with two **Id:**-addressed task blocks: "setup"
// (in_progress) and "deploy" (planning).
const twoTaskPlanBody = `# Plan: Auth

**Status:** Executing
**Source Feature:** auth

## Tasks

### Task 1: Setup

**Id:** setup
**Status:** in_progress
**Depends-On:** —

Setup body.

### Task 2: Deploy

**Id:** deploy
**Status:** planning
**Depends-On:** 1

Deploy body.
`

// planTaskStatus returns the **Status:** value of the block whose **Id:** equals
// id, by re-reading the plan file.
func planTaskStatus(t *testing.T, path, id string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read plan: %v", err)
	}
	lines := strings.Split(string(data), "\n")
	foundID := false
	for _, line := range lines {
		s := strings.TrimSpace(line)
		if s == "**Id:** "+id {
			foundID = true
			continue
		}
		if foundID && strings.HasPrefix(s, "**Status:**") {
			return strings.TrimSpace(strings.TrimPrefix(s, "**Status:**"))
		}
	}
	t.Fatalf("no status after **Id:** %s in %s", id, path)
	return ""
}

// AC plan-inline-target-resolves (happy path): resolve by **Id:**, set status on
// the right block, leave the other block untouched, print the success line.
func TestTaskChangeStatus_PlanInline_Resolves(t *testing.T) {
	_, planPath := stagePlanWithTasks(t, "auth", twoTaskPlanBody)

	stdout, stderr, err := runTask(t, "change-status", "setup",
		"--plan", "auth", "--to=complete", "--commit", "a1b2c3d")
	if err != nil {
		t.Fatalf("change-status: %v (stderr=%s)", err, stderr)
	}
	if want := "setup: in_progress → complete\n"; stdout != want {
		t.Errorf("stdout = %q; want %q", stdout, want)
	}
	if got := planTaskStatus(t, planPath, "setup"); got != "complete" {
		t.Errorf("setup status = %q; want complete", got)
	}
	// The sibling block must be byte-untouched.
	if got := planTaskStatus(t, planPath, "deploy"); got != "planning" {
		t.Errorf("deploy status = %q; want planning (untouched)", got)
	}
}

// No task block carries a matching **Id:** → exit 3 (NotFound).
func TestTaskChangeStatus_PlanInline_NoMatchingId(t *testing.T) {
	_, planPath := stagePlanWithTasks(t, "auth", twoTaskPlanBody)
	_, _, err := runTask(t, "change-status", "ghost", "--plan", "auth", "--to=queued")
	if got := exitCodeOfErr(err); got != exitcode.NotFound {
		t.Errorf("exit = %d, want %d (NotFound); err=%v", got, exitcode.NotFound, err)
	}
	if got := planTaskStatus(t, planPath, "setup"); got != "in_progress" {
		t.Errorf("setup changed: %q", got)
	}
}

// Missing plan file → exit 3 (NotFound).
func TestTaskChangeStatus_PlanInline_MissingPlan(t *testing.T) {
	stagePlanWithTasks(t, "auth", twoTaskPlanBody)
	_, _, err := runTask(t, "change-status", "setup", "--plan", "ghost", "--to=complete")
	if got := exitCodeOfErr(err); got != exitcode.NotFound {
		t.Errorf("exit = %d, want %d (NotFound); err=%v", got, exitcode.NotFound, err)
	}
}

// Illegal transition on a plan-inline task → exit 4 (InvalidState), block unchanged.
func TestTaskChangeStatus_PlanInline_IllegalTransition(t *testing.T) {
	_, planPath := stagePlanWithTasks(t, "auth", twoTaskPlanBody)
	// "deploy" is planning; planning → complete is illegal.
	_, _, err := runTask(t, "change-status", "deploy", "--plan", "auth", "--to=complete")
	if got := exitCodeOfErr(err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d (InvalidState); err=%v", got, exitcode.InvalidState, err)
	}
	if got := planTaskStatus(t, planPath, "deploy"); got != "planning" {
		t.Errorf("deploy changed on rejection: %q", got)
	}
}

// A resolved block with no **Status:** line surfaces an Unexpected (10) error.
func TestTaskChangeStatus_PlanInline_NoStatusLine(t *testing.T) {
	body := `# Plan: Auth

**Status:** Executing
**Source Feature:** auth

## Tasks

### Task 1: Setup

**Id:** setup
**Depends-On:** —

No status field here.
`
	stagePlanWithTasks(t, "auth", body)
	// from defaults to planning (no status line); planning → queued is legal, so
	// validation passes and the missing-status-line guard fires.
	_, _, err := runTask(t, "change-status", "setup", "--plan", "auth", "--to=queued")
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected); err=%v", got, exitcode.Unexpected, err)
	}
}

// A non-not-exist plan-parse failure (unreadable file) surfaces Unexpected (10).
func TestTaskChangeStatus_PlanInline_UnreadablePlan(t *testing.T) {
	_, planPath := stagePlanWithTasks(t, "auth", twoTaskPlanBody)
	if err := os.Chmod(planPath, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(planPath, 0o644) })

	_, _, err := runTask(t, "change-status", "setup", "--plan", "auth", "--to=complete")
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected); err=%v", got, exitcode.Unexpected, err)
	}
}

// A post-validation rewrite I/O failure surfaces Unexpected (10). The plan file
// is made read-only so the parse (a read) succeeds while the rewrite fails.
func TestTaskChangeStatus_PlanInline_RewriteFailure(t *testing.T) {
	_, planPath := stagePlanWithTasks(t, "auth", twoTaskPlanBody)
	if err := os.Chmod(planPath, 0o400); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(planPath, 0o644) })

	_, _, err := runTask(t, "change-status", "setup", "--plan", "auth", "--to=complete")
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected); err=%v", got, exitcode.Unexpected, err)
	}
}

// A --plan invocation under a project that does not resolve to a spec repo
// surfaces the resolve error.
func TestTaskChangeStatus_PlanInline_ProjectResolveError(t *testing.T) {
	stagePlanWithTasks(t, "auth", twoTaskPlanBody)
	bare := t.TempDir() // no specscore.yaml
	_, _, err := runTask(t, "change-status", "setup",
		"--plan", "auth", "--to=complete", "--project", bare)
	if err == nil {
		t.Fatal("expected resolve error, got nil")
	}
}
