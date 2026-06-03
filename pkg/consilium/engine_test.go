package consilium

import (
	"reflect"
	"testing"
)

// defaultRosterSnapshot returns the 9-role default roster in declared order,
// matching REQ:default-roster-9-roles: 3 builders, 3 customers, 3 adversaries.
func defaultRosterSnapshot() []RosterEntry {
	return []RosterEntry{
		{Name: "engineer", Group: GroupBuilders},
		{Name: "architect", Group: GroupBuilders},
		{Name: "qa", Group: GroupBuilders},
		{Name: "pm", Group: GroupCustomers},
		{Name: "ux", Group: GroupCustomers},
		{Name: "marketing", Group: GroupCustomers},
		{Name: "yagni-cop", Group: GroupAdversaries},
		{Name: "skeptic", Group: GroupAdversaries},
		{Name: "security-ops", Group: GroupAdversaries},
	}
}

// approveVote is a helper for a should-implement vote at the given confidence,
// cost, and complexity for a role.
func approveVote(role, confidence, cost, complexity string) Vote {
	return Vote{
		Verdict:    "should-implement",
		Confidence: confidence,
		Cost:       cost,
		Complexity: complexity,
		Argument:   "ok",
		Role:       role,
	}
}

// AC: gate-engine-applies-rules-in-order — the strict-baseline happy path.
// All builders and customers approve at medium+ confidence with cost/complexity
// at or under the ceilings; no abstains or adversary vetoes. The verdict MUST be
// should-implement with the rule_trace ending in the step-11 rule, empty
// excluded_votes, and {builders:3, customers:3, adversaries:3} denominators.
func TestEvaluate_AppliesRulesInOrder_ShouldImplement(t *testing.T) {
	roster := defaultRosterSnapshot()
	votes := []Vote{
		approveVote("engineer", "high", "🟢", "🟢"),
		approveVote("architect", "medium", "🟡", "🟡"),
		approveVote("qa", "high", "🟢", "🟡"),
		approveVote("pm", "medium", "🟡", "🟢"),
		approveVote("ux", "high", "🟢", "🟢"),
		approveVote("marketing", "medium", "🟡", "🟡"),
	}

	got := Evaluate(votes, roster, StrictBaseline())

	if got.Verdict != VerdictShouldImplement {
		t.Fatalf("Verdict = %q, want %q", got.Verdict, VerdictShouldImplement)
	}
	if len(got.ExcludedVotes) != 0 {
		t.Errorf("ExcludedVotes = %v, want empty", got.ExcludedVotes)
	}
	wantDen := Denominators{Builders: 3, Customers: 3, Adversaries: 3}
	if got.Denominators != wantDen {
		t.Errorf("Denominators = %+v, want %+v", got.Denominators, wantDen)
	}
	if len(got.RuleTrace) == 0 {
		t.Fatalf("RuleTrace is empty, want it to end in the should-implement rule")
	}
	last := got.RuleTrace[len(got.RuleTrace)-1]
	if last != ruleShouldImplement {
		t.Errorf("RuleTrace ends in %q, want %q; full trace = %v", last, ruleShouldImplement, got.RuleTrace)
	}
	// The five gates must have fired (be named) before the terminal rule.
	wantTrace := []string{
		ruleBuilderGate,
		ruleCustomerGate,
		ruleConfidenceGate,
		ruleCostGate,
		ruleComplexityGate,
		ruleShouldImplement,
	}
	if !reflect.DeepEqual(got.RuleTrace, wantTrace) {
		t.Errorf("RuleTrace = %v, want %v", got.RuleTrace, wantTrace)
	}
}

// A unanimous should-not-implement from builders and customers yields the
// step-12 terminal verdict.
func TestEvaluate_ShouldNotImplement(t *testing.T) {
	roster := defaultRosterSnapshot()
	mk := func(role string) Vote {
		return Vote{Verdict: "should-not-implement", Confidence: "high", Cost: "🟢", Complexity: "🟢", Argument: "no", Role: role}
	}
	votes := []Vote{
		mk("engineer"), mk("architect"), mk("qa"),
		mk("pm"), mk("ux"), mk("marketing"),
	}

	got := Evaluate(votes, roster, StrictBaseline())

	if got.Verdict != VerdictShouldNotImplement {
		t.Fatalf("Verdict = %q, want %q", got.Verdict, VerdictShouldNotImplement)
	}
	if last := got.RuleTrace[len(got.RuleTrace)-1]; last != ruleShouldNotImplement {
		t.Errorf("RuleTrace ends in %q, want %q", last, ruleShouldNotImplement)
	}
}

// A split that neither passes all gates nor reaches a unanimous rejection falls
// through to needs-human-review (step 13).
func TestEvaluate_NeedsHumanReview(t *testing.T) {
	roster := defaultRosterSnapshot()
	votes := []Vote{
		approveVote("engineer", "high", "🟢", "🟢"),
		approveVote("architect", "high", "🟢", "🟢"),
		// qa rejects -> builder supermajority still holds (require_all=true fails it though)
		{Verdict: "should-not-implement", Confidence: "high", Cost: "🟢", Complexity: "🟢", Argument: "no", Role: "qa"},
		approveVote("pm", "high", "🟢", "🟢"),
		approveVote("ux", "high", "🟢", "🟢"),
		approveVote("marketing", "high", "🟢", "🟢"),
	}

	got := Evaluate(votes, roster, StrictBaseline())

	if got.Verdict != VerdictNeedsHumanReview {
		t.Fatalf("Verdict = %q, want %q", got.Verdict, VerdictNeedsHumanReview)
	}
	if last := got.RuleTrace[len(got.RuleTrace)-1]; last != ruleNeedsHumanReview {
		t.Errorf("RuleTrace ends in %q, want %q", last, ruleNeedsHumanReview)
	}
}

// Confidence below the floor blocks should-implement even when approvals pass.
func TestEvaluate_ConfidenceFloorBlocks(t *testing.T) {
	roster := defaultRosterSnapshot()
	votes := []Vote{
		approveVote("engineer", "low", "🟢", "🟢"),
		approveVote("architect", "low", "🟢", "🟢"),
		approveVote("qa", "low", "🟢", "🟢"),
		approveVote("pm", "low", "🟢", "🟢"),
		approveVote("ux", "low", "🟢", "🟢"),
		approveVote("marketing", "low", "🟢", "🟢"),
	}

	got := Evaluate(votes, roster, StrictBaseline())

	if got.Verdict != VerdictNeedsHumanReview {
		t.Fatalf("Verdict = %q, want %q (median confidence below floor)", got.Verdict, VerdictNeedsHumanReview)
	}
}

// Cost above the ceiling blocks should-implement.
func TestEvaluate_CostCeilingBlocks(t *testing.T) {
	roster := defaultRosterSnapshot()
	votes := []Vote{
		approveVote("engineer", "high", "🔴", "🟢"),
		approveVote("architect", "high", "🔴", "🟢"),
		approveVote("qa", "high", "🔴", "🟢"),
		approveVote("pm", "high", "🔴", "🟢"),
		approveVote("ux", "high", "🔴", "🟢"),
		approveVote("marketing", "high", "🔴", "🟢"),
	}

	got := Evaluate(votes, roster, StrictBaseline())

	if got.Verdict != VerdictNeedsHumanReview {
		t.Fatalf("Verdict = %q, want %q (median cost above ceiling)", got.Verdict, VerdictNeedsHumanReview)
	}
}

func TestMedianOrdinal_LowerMiddleForEven(t *testing.T) {
	// Even count -> lower-middle element. [0,1,2,3] -> index 1 -> 1.
	if got := medianOrdinal([]int{0, 1, 2, 3}); got != 1 {
		t.Errorf("medianOrdinal([0,1,2,3]) = %d, want 1 (lower-middle)", got)
	}
	// Odd count -> true middle.
	if got := medianOrdinal([]int{0, 2, 2}); got != 2 {
		t.Errorf("medianOrdinal([0,2,2]) = %d, want 2", got)
	}
}
