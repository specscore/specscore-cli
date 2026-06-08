package lint

import "testing"

// p005PlanBody builds a fully-valid single-file plan (P-001..P-004 clean when
// the "foo" feature with AC "one" exists) carrying the supplied parentLine
// (e.g. "**Parent:** foo\n", or "" for a root plan).
func p005PlanBody(slug, parentLine string) string {
	return "# Plan: " + slug + "\n\n" +
		"**Status:** Draft\n" +
		"**Source Feature:** foo\n" +
		"**Date:** 2026-06-08\n" +
		"**Owner:** o\n" +
		"**Supersedes:** —\n" +
		parentLine +
		"\n## Summary\n\ns\n\n## Approach\n\na\n\n" +
		"## Tasks\n\n### Task 1: t\n\n**Status:** pending\n**Depends-On:** —\n**Verifies:** foo#ac:one\n\nbody\n\n" +
		"## Open Questions\n\nNone.\n"
}

func TestP005_RootPlanNoViolation(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "foo", "one")
	e.writePlan(t, "root", p005PlanBody("root", ""))
	if v := hasViolation(runRules(t, e), "P-005", ""); v != nil {
		t.Fatalf("root plan should have no P-005 violation, got %q", v.Message)
	}
}

func TestP005_CrossRepoAccepted(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "foo", "one")
	e.writePlan(t, "sub", p005PlanBody("sub", "**Parent:** other-repo:some-master\n"))
	if v := hasViolation(runRules(t, e), "P-005", ""); v != nil {
		t.Fatalf("cross-repo parent should be accepted, got %q", v.Message)
	}
}

func TestP005_CrossRepoMalformed(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "foo", "one")
	e.writePlan(t, "badx", p005PlanBody("badx", "**Parent:** Bad_Repo:\n"))
	if v := hasViolation(runRules(t, e), "P-005", "malformed cross-repo"); v == nil {
		t.Fatal("expected malformed cross-repo P-005 violation")
	}
}

func TestP005_SameRepoResolves(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "foo", "one")
	e.writePlan(t, "master", p005PlanBody("master", ""))
	e.writePlan(t, "child", p005PlanBody("child", "**Parent:** master\n"))
	if v := hasViolation(runRules(t, e), "P-005", ""); v != nil {
		t.Fatalf("resolvable same-repo parent should pass, got %q", v.Message)
	}
}

func TestP005_DanglingParent(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "foo", "one")
	e.writePlan(t, "child", p005PlanBody("child", "**Parent:** ghost\n"))
	if v := hasViolation(runRules(t, e), "P-005", "does not resolve"); v == nil {
		t.Fatal("expected dangling-parent P-005 violation")
	}
}

func TestP005_SelfParent(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "foo", "one")
	e.writePlan(t, "loop", p005PlanBody("loop", "**Parent:** loop\n"))
	if v := hasViolation(runRules(t, e), "P-005", "references the plan itself"); v == nil {
		t.Fatal("expected self-parent P-005 violation")
	}
}

func TestP005_TwoNodeCycle(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "foo", "one")
	e.writePlan(t, "cyca", p005PlanBody("cyca", "**Parent:** cycb\n"))
	e.writePlan(t, "cycb", p005PlanBody("cycb", "**Parent:** cyca\n"))
	vs := runRules(t, e)
	cy := hasViolation(vs, "P-005", "forms a cycle")
	if cy == nil {
		t.Fatal("expected a cycle P-005 violation")
	}
	// Reported exactly once, on the lexicographically-smaller member.
	count := 0
	for _, v := range vs {
		if v.Rule == "P-005" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("cycle should report exactly one P-005 violation, got %d", count)
	}
	if cy.File != "plans/cyca.md" {
		t.Fatalf("cycle should be reported on cyca.md, got %s", cy.File)
	}
	if cy.Message != "**Parent:** chain forms a cycle: cyca → cycb → cyca" {
		t.Fatalf("unexpected cycle render: %q", cy.Message)
	}
}

func TestP005_ThreeNodeCycle(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "foo", "one")
	e.writePlan(t, "pa", p005PlanBody("pa", "**Parent:** pb\n"))
	e.writePlan(t, "pb", p005PlanBody("pb", "**Parent:** pc\n"))
	e.writePlan(t, "pc", p005PlanBody("pc", "**Parent:** pa\n"))
	cy := hasViolation(runRules(t, e), "P-005", "forms a cycle")
	if cy == nil {
		t.Fatal("expected a 3-node cycle violation")
	}
	if cy.Message != "**Parent:** chain forms a cycle: pa → pb → pc → pa" {
		t.Fatalf("unexpected 3-node cycle render: %q", cy.Message)
	}
}

// TestP005_TailIntoCycle exercises the cycle reported via a tail node whose
// entry point is not the smallest cycle member, covering the min-search
// branches in cycleFirst and renderCyclePath. Edges: aa→cc, cc→bb, bb→cc
// (cycle {bb,cc}, tail aa). The cycle is discovered from aa as [cc, bb].
func TestP005_TailIntoCycle(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "foo", "one")
	e.writePlan(t, "aa", p005PlanBody("aa", "**Parent:** cc\n"))
	e.writePlan(t, "bb", p005PlanBody("bb", "**Parent:** cc\n"))
	e.writePlan(t, "cc", p005PlanBody("cc", "**Parent:** bb\n"))
	cy := hasViolation(runRules(t, e), "P-005", "forms a cycle")
	if cy == nil {
		t.Fatal("expected a cycle violation via tail node")
	}
	if cy.File != "plans/bb.md" {
		t.Fatalf("cycle should be reported on smallest member bb.md, got %s", cy.File)
	}
	if cy.Message != "**Parent:** chain forms a cycle: bb → cc → bb" {
		t.Fatalf("unexpected tail-cycle render: %q", cy.Message)
	}
}
