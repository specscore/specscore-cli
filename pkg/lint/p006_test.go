package lint

import "testing"

// p006PlanBody builds a single-file plan (P-001..P-005 clean when the "foo"
// feature with AC "one" exists) carrying the supplied document **Status:** value.
func p006PlanBody(slug, status string) string {
	return "# Plan: " + slug + "\n\n" +
		"**Status:** " + status + "\n" +
		"**Source Feature:** foo\n" +
		"**Date:** 2026-06-08\n" +
		"**Owner:** o\n" +
		"**Supersedes:** —\n" +
		"\n## Summary\n\ns\n\n## Approach\n\na\n\n" +
		"## Tasks\n\n### Task 1: t\n\n**Status:** pending\n**Depends-On:** —\n**Verifies:** foo#ac:one\n\nbody\n\n" +
		"## Open Questions\n\nNone.\n"
}

// AC: plan-status-enum-flagged
func TestP006_NonCanonicalStatusFlagged(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "foo", "one")
	e.writePlan(t, "bad", p006PlanBody("bad", "Completed"))
	v := hasViolation(runRules(t, e), "P-006", "Completed")
	if v == nil {
		t.Fatal("expected P-006 violation for non-canonical status")
	}
	if v.File != "plans/bad.md" {
		t.Fatalf("File = %q, want plans/bad.md", v.File)
	}
	if v.Line != 3 {
		t.Fatalf("Line = %d, want 3 (the **Status:** line)", v.Line)
	}
	if v.Severity != "error" {
		t.Fatalf("Severity = %q, want error", v.Severity)
	}
}

// AC: plan-status-enum-accepts-canonical — every canonical Plan status is clean.
func TestP006_CanonicalStatusesAccepted(t *testing.T) {
	canonical := []string{
		"Draft", "In Review", "Approved", "Executing", "Blocked",
		"Implemented", "Failed", "Rejected", "Withdrawn", "Superseded", "Deprecated",
	}
	for _, s := range canonical {
		e := newPlanRulesEnv(t)
		e.writeFeature(t, "foo", "one")
		e.writePlan(t, "ok", p006PlanBody("ok", s))
		if v := hasViolation(runRules(t, e), "P-006", ""); v != nil {
			t.Fatalf("status %q should be accepted, got P-006: %q", s, v.Message)
		}
	}
}

// REQ:rule-p-006-plan-status-enum — comparison is case-sensitive.
func TestP006_WrongCaseFlagged(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "foo", "one")
	e.writePlan(t, "lc", p006PlanBody("lc", "draft"))
	if v := hasViolation(runRules(t, e), "P-006", "draft"); v == nil {
		t.Fatal("expected P-006 violation for wrong-case status")
	}
}

// REQ:rule-p-006-plan-status-enum — an absent **Status:** line emits nothing.
func TestP006_AbsentStatusNoViolation(t *testing.T) {
	e := newPlanRulesEnv(t)
	e.writeFeature(t, "foo", "one")
	e.writePlan(t, "nostatus", "# Plan: nostatus\n\n"+
		"**Source Feature:** foo\n\n## Summary\n\ns\n\n## Approach\n\na\n\n"+
		"## Tasks\n\n### Task 1: t\n\n**Status:** pending\n**Depends-On:** —\n**Verifies:** foo#ac:one\n\nbody\n\n"+
		"## Open Questions\n\nNone.\n")
	if v := hasViolation(runRules(t, e), "P-006", ""); v != nil {
		t.Fatalf("absent **Status:** should emit no P-006, got %q", v.Message)
	}
}
