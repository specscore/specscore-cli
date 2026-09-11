package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stalePlanWithBadSourceForRouteError is a Plan artifact carrying a P-002
// violation (unrecognized **Source:** value) planted directly under
// spec/plans — the exact stale-local-tree shape finding 1's reviewer
// reproduction describes (a committed plans_repo, no repo_checkouts, and a
// stale local spec/plans file with a bad **Source:**). It must never surface
// once Options.PlanRouteError is set: every Plan-owned checker is disabled
// for that run.
const stalePlanWithBadSourceForRouteError = "# Plan: Stale\n\n**Status:** Draft\n**Source:** not-a-real-source\n\n## Tasks\n\n### Task 1: Do\n\n**Status:** planning\n\n## Open Questions\n\nNone at this time.\n"

// TestLint_PlanRouteError_SkipsEveryPlanOwnedCheckerAndTouchesNoFile drives
// LintWithResult end to end (both the fix pass and the check pass, in one
// call) with PlanRouteError set, over a spec tree whose local spec/plans
// carries a real, otherwise-detectable violation. It proves: (a) every
// Plan-owned rule — readme-exists' plans walk, plan-hierarchy,
// plan-roi-metadata, plan-index-sync, P-001..P-010, and the plan-owned
// adherence-footer/status-mirror targets — is silent, (b) the sole
// plan-route-unresolved finding names the configured error, and (c) not one
// byte under spec/plans changes even with Fix:true (the CLI is the layer
// that actually refuses --fix outright before calling into pkg/lint at all;
// this proves pkg/lint itself never writes there either, as defense in
// depth for any other caller).
func TestLint_PlanRouteError_SkipsEveryPlanOwnedCheckerAndTouchesNoFile(t *testing.T) {
	root := t.TempDir()
	planDir := filepath.Join(root, "plans", "stale")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(planDir, "README.md")
	if err := os.WriteFile(planPath, []byte(stalePlanWithBadSourceForRouteError), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}

	res, err := LintWithResult(Options{
		SpecRoot:       root,
		Fix:            true,
		PlanRouteError: "plans repository acme/plans-hub has no local checkout; set repo_checkouts.acme/plans-hub",
	})
	if err != nil {
		t.Fatalf("LintWithResult error: %v", err)
	}

	var routeFindings int
	for _, v := range res.Violations {
		if v.Rule == "plan-route-unresolved" {
			routeFindings++
			if v.Severity != "error" {
				t.Errorf("plan-route-unresolved severity = %q, want error", v.Severity)
			}
			if !strings.Contains(v.Message, "acme/plans-hub") {
				t.Errorf("plan-route-unresolved message should name the broken route; got %q", v.Message)
			}
			continue
		}
		if strings.HasPrefix(v.Rule, "P-") || v.Rule == "plan-hierarchy" || v.Rule == "plan-roi-metadata" || v.Rule == "plan-index-sync" {
			t.Errorf("unexpected Plan-owned violation surfaced under a broken route: %+v", v)
		}
		if v.Rule == "readme-exists" && (v.File == "plans" || strings.HasPrefix(filepath.ToSlash(v.File), "plans/")) {
			t.Errorf("readme-exists must not report on spec/plans under a broken route: %+v", v)
		}
	}
	if routeFindings != 1 {
		t.Fatalf("expected exactly one plan-route-unresolved finding, got %d (violations=%+v)", routeFindings, res.Violations)
	}

	if len(res.Fixed) != 0 {
		t.Errorf("Fix must not touch any file under a broken Plan route; Fixed = %v", res.Fixed)
	}
	after, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("the stale local plan was rewritten despite the broken route")
	}
}

// TestLint_PlanRouteError_EmptyStringNeverEmits proves the sentinel: an empty
// PlanRouteError (the default — both the "no route configured" and "route
// resolved successfully" shapes leave it empty) never registers the
// plan-route-unresolved finding, and every Plan-owned checker runs exactly
// as it did before this option existed.
func TestLint_PlanRouteError_EmptyStringNeverEmits(t *testing.T) {
	root := t.TempDir()
	res, err := LintWithResult(Options{SpecRoot: root})
	if err != nil {
		t.Fatalf("LintWithResult error: %v", err)
	}
	for _, v := range res.Violations {
		if v.Rule == "plan-route-unresolved" {
			t.Fatalf("unexpected plan-route-unresolved finding with empty PlanRouteError: %+v", v)
		}
	}
}
