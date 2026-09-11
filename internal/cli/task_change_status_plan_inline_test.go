package cli

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
	"github.com/specscore/specscore-cli/pkg/lint"
)

// Covers rewritePlanTaskStatusLine's ReadFile error branch directly (a missing
// file cannot arise post-parse, so it is exercised as a unit).
func TestRewritePlanTaskStatusLine_ReadError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.md")
	if err := rewritePlanTaskStatusLine(missing, 1, lifecycle.TaskComplete, nil); err == nil {
		t.Fatal("expected ReadFile error for missing file, got nil")
	}
}

// sourceFeatureLineRe extracts the value of a plan body's **Source Feature:**
// header line, so stagePlanWithTasks can materialize a real, lint-clean
// Feature for it.
var sourceFeatureLineRe = regexp.MustCompile(`(?m)^\*\*Source Feature:\*\* (.+)$`)

// stagePlanWithTasks writes a SpecScore project with a single-file plan at
// spec/plans/<slug>.md containing the given body, sets cwd to the root, and
// returns the root and the plan-file path. The project is git-initialized
// and self-routes Plans to its own identity (configureSameRepoPlans) so
// resolvePlanStore succeeds exactly as it would for a real same-repo project
// (repo-config#req:plans-repo-project-selection: omission does not imply
// same-repository storage, so every fixture must route explicitly).
//
// runTaskChangeStatusPlanInline (and the amend/provenance equivalents) now
// run the same full post-mutation lint verification plan change-status
// already ran (deriving the containing Plan's execution-band **Status:**
// from the task rollup, per P-007) — so the fixture must be genuinely
// lint-clean, not just the bare plan file: an ancestor spec/README.md, a
// Features index, the referenced Source Feature's own README (when the body
// declares one), and a canonical spec/plans/README.md. migrateTree backfills
// the artifact-frontmatter-convention frontmatter the hand-written body
// constants in this file don't carry.
func stagePlanWithTasks(t *testing.T, slug, body string) (string, string) {
	t.Helper()
	root := t.TempDir()
	configureSameRepoPlans(t, root)
	withCwd(t, root)

	if err := os.MkdirAll(filepath.Join(root, "spec", "features"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFeature := func(featureSlug string) {
		t.Helper()
		featureDir := filepath.Join(root, "spec", "features", filepath.FromSlash(featureSlug))
		if err := os.MkdirAll(featureDir, 0o755); err != nil {
			t.Fatal(err)
		}
		toolbar := "> [SpecScore.**Studio**](https://specscore.studio): | " +
			"[Explore](https://specscore.studio/app/github.com/specscore/test-fixture/spec/features/" + featureSlug + "?op=explore) | " +
			"[Edit](https://specscore.studio/app/github.com/specscore/test-fixture/spec/features/" + featureSlug + "?op=edit) | " +
			"[Ask question](https://specscore.studio/app/github.com/specscore/test-fixture/spec/features/" + featureSlug + "?op=ask) | " +
			"[Request change](https://specscore.studio/app/github.com/specscore/test-fixture/spec/features/" + featureSlug + "?op=request-change) |"
		featureBody := "---\nformat: https://specscore.md/feature-specification\nstatus: Approved\n---\n\n" +
			"# Feature: " + featureSlug + "\n\n" + toolbar + "\n**Status:** Approved\n**Source Ideas:** —\n\n" +
			"## Summary\n\nTest fixture.\n\n## Open Questions\n\nNone at this time.\n\n" +
			"---\n*This document follows the https://specscore.md/feature-specification*\n"
		if err := os.WriteFile(filepath.Join(featureDir, "README.md"), []byte(featureBody), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	featuresIndexRows := ""
	if m := sourceFeatureLineRe.FindStringSubmatch(body); m != nil {
		featureSlug := strings.TrimSpace(m[1])
		topLevel := strings.SplitN(featureSlug, "/", 2)[0]
		featuresIndexRows = "| [" + topLevel + "](" + topLevel + "/README.md) | Approved |\n"
		// A nested slug (e.g. "rts-multilayer-chess/browser-play-surface")
		// names an umbrella Feature directory too (readme-exists requires a
		// README.md at every level, mirroring this repo's own cli/plan
		// nesting) — write one for every ancestor segment, not just the leaf.
		segments := strings.Split(featureSlug, "/")
		for i := range segments {
			writeFeature(strings.Join(segments[:i+1], "/"))
		}
	}
	featuresReadme := "# Features\n\n## Index\n\n| Feature | Status |\n|---------|--------|\n" + featuresIndexRows + "\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(root, "spec", "features", "README.md"), []byte(featuresReadme), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeMissingIndex(root, "spec/README.md", specReadmeContent(readSpecConfigBestEffort(root))); err != nil {
		t.Fatal(err)
	}
	if err := writeMissingIndex(root, "spec/plans/README.md", plansIndexContent(readSpecConfigBestEffort(root))); err != nil {
		t.Fatal(err)
	}

	plansDir := filepath.Join(root, "spec", "plans")
	planPath := filepath.Join(plansDir, slug+".md")
	_ = os.WriteFile(planPath, []byte(body), 0o644)
	migrateTree(t, root)

	// Best-effort structural backfill (the studio toolbar line, missing
	// Features-index rows, plans-index rows, ...) — NOT gated on the result
	// being fully lint-clean afterward, unlike stagePlan: many callers here
	// deliberately construct an imperfect plan body (a malformed
	// **Implemented-by:**, an unresolved **Prerequisite Plans:** entry, ...)
	// as the exact behavior under test, and such a fixture legitimately fails
	// full-tree lint. Those tests fail before runTaskChangeStatusPlanInline's
	// post-mutation lint hook is ever reached; only the happy-path tests that
	// actually get there need the mechanical backfill below.
	for i := 0; i < 2; i++ {
		if _, err := lintLintFn(lint.Options{SpecRoot: filepath.Join(root, "spec"), Fix: true}); err != nil {
			t.Logf("stagePlanWithTasks lint --fix (best-effort): %v", err)
		}
	}
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

// An explicitly supplied blank --plan must never fall back to a same-named
// board task. The matching board task is deliberately mutable by each command
// shape; byte equality proves the invalid selector stopped dispatch before any
// board or plan mutation.
func TestTaskChangeStatus_ExplicitBlankPlanNeverFallsBackToBoard(t *testing.T) {
	for _, tc := range []struct {
		name     string
		planFlag string
		amend    bool
	}{
		{name: "status with equals", planFlag: "--plan="},
		{name: "status with whitespace", planFlag: "--plan= \t"},
		{name: "amend with equals", planFlag: "--plan=", amend: true},
		{name: "amend with whitespace", planFlag: "--plan= \t", amend: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status := "in_progress"
			planBody := twoTaskPlanBody
			if tc.amend {
				status = "complete"
				planBody = completeTaskPlanBody
			}
			root, boardTaskPath := stageTaskWithStatus(t, "setup", status)
			if tc.amend {
				if err := writeBoardImplementedBy(boardTaskPath, "backstage@wrongsha"); err != nil {
					t.Fatalf("seed board provenance: %v", err)
				}
			}

			plansDir := filepath.Join(root, "spec", "plans")
			if err := os.MkdirAll(plansDir, 0o755); err != nil {
				t.Fatal(err)
			}
			planPath := filepath.Join(plansDir, "auth.md")
			if err := os.WriteFile(planPath, []byte(planBody), 0o644); err != nil {
				t.Fatal(err)
			}

			boardIndexPath := filepath.Join(root, "tasks", "README.md")
			boardIndexBefore, err := os.ReadFile(boardIndexPath)
			if err != nil {
				t.Fatal(err)
			}
			boardBefore, err := os.ReadFile(boardTaskPath)
			if err != nil {
				t.Fatal(err)
			}
			planBefore, err := os.ReadFile(planPath)
			if err != nil {
				t.Fatal(err)
			}

			args := []string{"change-status", "setup", tc.planFlag}
			if tc.amend {
				args = append(args, "--amend-provenance", "--commit", "a1b2c3d")
			} else {
				args = append(args, "--to=complete")
			}
			_, _, err = runTask(t, args...)
			if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
				t.Fatalf("exit = %d, want %d (InvalidArgs); err=%v", got, exitcode.InvalidArgs, err)
			}

			boardIndexAfter, readErr := os.ReadFile(boardIndexPath)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(boardIndexAfter) != string(boardIndexBefore) {
				t.Fatalf("board index changed despite invalid --plan refusal:\n%s", boardIndexAfter)
			}
			boardAfter, readErr := os.ReadFile(boardTaskPath)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(boardAfter) != string(boardBefore) {
				t.Fatalf("matching board task changed despite invalid --plan refusal:\n%s", boardAfter)
			}
			planAfter, readErr := os.ReadFile(planPath)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(planAfter) != string(planBefore) {
				t.Fatalf("plan changed despite invalid --plan refusal:\n%s", planAfter)
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
