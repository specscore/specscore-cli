package cli

import (
	"errors"
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
	if err := rewritePlanTaskStatusLine(missing, 1, lifecycle.TaskComplete, nil); err == nil {
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

// A plan-inline task mutation is valid only inside a real Plan artifact. This
// holds even when an arbitrary Markdown file contains a complete-looking task
// example; no status or provenance field may be rewritten on refusal.
func TestTaskChangeStatus_PlanInline_RefusesNonPlanWithoutMutation(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"backtick fence", "```markdown\n# Plan: Auth\n## Tasks\n### Task 1: Example\n**Id:** setup\n**Status:** planning\n```\n# Notes\n"},
		{"tilde fence", "~~~markdown\n# Plan: Auth\n## Tasks\n### Task 1: Example\n**Id:** setup\n**Status:** planning\n~~~\n# Notes\n"},
		{"indented code", "    # Plan: Auth\n    ## Tasks\n    ### Task 1: Example\n    **Id:** setup\n    **Status:** planning\n# Notes\n"},
		{"HTML comment", "<!--\n# Plan: Auth\n## Tasks\n### Task 1: Example\n**Id:** setup\n**Status:** planning\n-->\n# Notes\n"},
		{"frontmatter", "---\n# Plan: Auth\n## Tasks\n### Task 1: Example\n**Id:** setup\n**Status:** planning\n---\n# Notes\n"},
		{"earlier Setext H1", "Notes\n=====\n# Plan: Auth\n## Tasks\n### Task 1: Example\n**Id:** setup\n**Status:** planning\n"},
		{"earlier tab-separated ATX H1", "#\tNotes\n# Plan: Auth\n## Tasks\n### Task 1: Example\n**Id:** setup\n**Status:** planning\n"},
		{"earlier three-space ATX H1", "   # Notes\n# Plan: Auth\n## Tasks\n### Task 1: Example\n**Id:** setup\n**Status:** planning\n"},
		{"earlier bare ATX H1", "#\n# Plan: Auth\n## Tasks\n### Task 1: Example\n**Id:** setup\n**Status:** planning\n"},
		{"earlier one-character Setext H1", "Notes\n=\n# Plan: Auth\n## Tasks\n### Task 1: Example\n**Id:** setup\n**Status:** planning\n"},
		{"BOM-prefixed frontmatter", "\ufeff---\n# Plan: Metadata fake\n<!-- comment -->\n---\n# Notes\n## Tasks\n### Task 1: Example\n**Id:** setup\n**Status:** planning\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, planPath := stagePlanWithTasks(t, "auth", tc.body)
			before, err := os.ReadFile(planPath)
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = runTask(t, "change-status", "setup", "--plan", "auth", "--to=queued")
			if got := exitCodeOfErr(err); got != exitcode.InvalidState {
				t.Fatalf("exit = %d, want %d; err=%v", got, exitcode.InvalidState, err)
			}
			after, readErr := os.ReadFile(planPath)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(after) != string(before) {
				t.Fatalf("non-Plan file changed despite refusal:\n%s", after)
			}
		})
	}
}

// A traversal-shaped --plan must be rejected as usage before either
// plan-inline operation constructs its path. The target outside spec/plans is
// deliberately a complete-looking Plan: byte equality proves neither status
// nor provenance can be written through the selector.
func TestTaskChangeStatus_PlanInline_InvalidPlanSlugNeverTouchesExternalFile(t *testing.T) {
	root, _ := stagePlanWithTasks(t, "auth", twoTaskPlanBody)
	externalPath := filepath.Join(root, "outside.md")
	for _, tc := range []struct {
		name     string
		args     []string
		external []byte
	}{
		{
			name: "status transition",
			args: []string{"change-status", "setup", "--plan", "../../outside", "--to=queued"},
			external: []byte(`# Plan: Outside

## Tasks

### Task 1: Setup

**Id:** setup
**Status:** planning
`),
		},
		{
			name: "provenance amend",
			args: []string{"change-status", "setup", "--plan", "../../outside", "--amend-provenance", "--commit", "a1b2c3d"},
			external: []byte(`# Plan: Outside

## Tasks

### Task 1: Setup

**Id:** setup
**Status:** complete
**Implemented-by:** old@deadbee
`),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(externalPath, tc.external, 0o644); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(externalPath)
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = runTask(t, tc.args...)
			if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
				t.Fatalf("exit = %d, want %d (InvalidArgs); err=%v", got, exitcode.InvalidArgs, err)
			}
			after, readErr := os.ReadFile(externalPath)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(after) != string(before) {
				t.Fatalf("external file changed after invalid --plan refusal:\n%s", after)
			}
		})
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

// A plan-inline task enters execution only after every declared prerequisite
// derives Implemented. The refusal is before the rewrite, so the target plan
// stays byte-identical and the diagnostic lists the prerequisite and status.
func TestTaskChangeStatus_PlanInline_InProgressRequiresImplementedPrerequisites(t *testing.T) {
	body := strings.Replace(twoTaskPlanBody, "**Status:** Executing\n**Source Feature:** auth", "**Status:** Executing\n**Prerequisite Plans:** foundation\n**Source Feature:** auth", 1)
	body = strings.Replace(body, "**Status:** planning\n**Depends-On:** 1", "**Status:** queued\n**Depends-On:** 1", 1)
	root, planPath := stagePlanWithTasks(t, "auth", body)
	plansDir := filepath.Join(root, "spec", "plans")
	foundation := `# Plan: Foundation

**Status:** Approved

## Tasks

### Task 1: Work

**Status:** queued
`
	if err := os.WriteFile(filepath.Join(plansDir, "foundation.md"), []byte(foundation), 0o644); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = runTask(t, "change-status", "deploy", "--plan", "auth", "--to=in_progress")
	if got := exitCodeOfErr(err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d (InvalidState); err=%v", got, exitcode.InvalidState, err)
	}
	if !strings.Contains(err.Error(), "foundation") || !strings.Contains(err.Error(), "Approved") {
		t.Errorf("diagnostic must name unmet prerequisite/status: %v", err)
	}
	after, readErr := os.ReadFile(planPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != string(original) {
		t.Errorf("task plan changed despite readiness refusal:\n%s", after)
	}
}

// A reachable prerequisite cycle is invalid even if the directly named plan's
// own task rollup derives Implemented. The execution entrypoint must not use
// that direct status as a bypass, and it must leave the task unchanged.
func TestTaskChangeStatus_PlanInline_InProgressRejectsPrerequisiteCycle(t *testing.T) {
	body := strings.Replace(twoTaskPlanBody, "**Status:** Executing\n**Source Feature:** auth", "**Status:** Executing\n**Prerequisite Plans:** foundation\n**Source Feature:** auth", 1)
	body = strings.Replace(body, "**Status:** planning\n**Depends-On:** 1", "**Status:** queued\n**Depends-On:** 1", 1)
	root, planPath := stagePlanWithTasks(t, "auth", body)
	foundation := `# Plan: Foundation

**Status:** Implemented
**Prerequisite Plans:** auth

## Tasks

### Task 1: Work

**Status:** complete
`
	if err := os.WriteFile(filepath.Join(root, "spec", "plans", "foundation.md"), []byte(foundation), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = runTask(t, "change-status", "deploy", "--plan", "auth", "--to=in_progress")
	if got := exitCodeOfErr(err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidState, err)
	}
	if !strings.Contains(err.Error(), "prerequisite cycle: auth -> foundation -> auth") {
		t.Errorf("cycle diagnostic = %v", err)
	}
	after, readErr := os.ReadFile(planPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != string(before) {
		t.Error("task plan changed despite prerequisite-cycle refusal")
	}
}

// A malformed prerequisite artifact is a lifecycle refusal (4), not an
// operational failure (10), and the task status remains untouched.
func TestTaskChangeStatus_PlanInline_PreservesInvalidStateFakeHeadingReadinessError(t *testing.T) {
	body := strings.Replace(twoTaskPlanBody, "**Status:** Executing\n**Source Feature:** auth", "**Status:** Executing\n**Prerequisite Plans:** foundation\n**Source Feature:** auth", 1)
	body = strings.Replace(body, "**Status:** planning\n**Depends-On:** 1", "**Status:** queued\n**Depends-On:** 1", 1)
	root, planPath := stagePlanWithTasks(t, "auth", body)
	if err := os.WriteFile(filepath.Join(root, "spec", "plans", "foundation.md"), []byte("```markdown\n# Plan: Foundation\n```\n# Notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = runTask(t, "change-status", "deploy", "--plan", "auth", "--to=in_progress")
	if got := exitCodeOfErr(err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidState, err)
	}
	after, readErr := os.ReadFile(planPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != string(before) {
		t.Error("task plan changed despite invalid-state readiness refusal")
	}
}

// A duplicate prerequisite header is malformed even if a later header says
// none. Readiness must refuse before the target task rewrite and retain the
// first declaration for the authoring diagnostic.
func TestTaskChangeStatus_PlanInline_DuplicatePrerequisiteHeaderRefusesWithoutMutation(t *testing.T) {
	body := strings.Replace(twoTaskPlanBody, "**Status:** Executing\n**Source Feature:** auth", "**Status:** Executing\n**Prerequisite Plans:** foundation\n**Prerequisite Plans:** —\n**Source Feature:** auth", 1)
	body = strings.Replace(body, "**Status:** planning\n**Depends-On:** 1", "**Status:** queued\n**Depends-On:** 1", 1)
	root, planPath := stagePlanWithTasks(t, "auth", body)
	foundation := `# Plan: Foundation

**Status:** Implemented

## Tasks

### Task 1: Work

**Status:** complete
`
	if err := os.WriteFile(filepath.Join(root, "spec", "plans", "foundation.md"), []byte(foundation), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = runTask(t, "change-status", "deploy", "--plan", "auth", "--to=in_progress")
	if got := exitCodeOfErr(err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidState, err)
	}
	if !strings.Contains(err.Error(), "duplicate field") {
		t.Errorf("diagnostic = %v, want duplicate declaration", err)
	}
	after, readErr := os.ReadFile(planPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != string(before) {
		t.Error("task plan changed despite duplicate prerequisite refusal")
	}
}

func TestTaskChangeStatus_PlanInline_ReadinessReadFailureIsAtomic(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission denial is not enforceable for root")
	}
	body := strings.Replace(twoTaskPlanBody, "**Status:** Executing\n**Source Feature:** auth", "**Status:** Executing\n**Prerequisite Plans:** foundation\n**Source Feature:** auth", 1)
	body = strings.Replace(body, "**Status:** planning\n**Depends-On:** 1", "**Status:** queued\n**Depends-On:** 1", 1)
	root, planPath := stagePlanWithTasks(t, "auth", body)
	foundationPath := filepath.Join(root, "spec", "plans", "foundation.md")
	if err := os.WriteFile(foundationPath, []byte("# Plan: Foundation\n\n**Status:** Approved\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(foundationPath, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(foundationPath, 0o644) })
	original, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = runTask(t, "change-status", "deploy", "--plan", "auth", "--to=in_progress")
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
	after, readErr := os.ReadFile(planPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != string(original) {
		t.Errorf("task plan changed despite readiness read failure")
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
	stagePlanWithTasks(t, "auth", twoTaskPlanBody)
	boom := errors.New("atomic transaction boom")

	_, _, err := runTaskWithMutationDeps(t, taskMutationDeps{transformArtifact: func(string, func([]byte) ([]byte, error)) error { return boom }}, "change-status", "setup", "--plan", "auth", "--to=complete")
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected); err=%v", got, exitcode.Unexpected, err)
	}
	if !errors.Is(err, boom) {
		t.Fatalf("error lost transaction cause: %v", err)
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
