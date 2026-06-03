package consilium

import (
	"reflect"
	"strings"
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

// abstainVote is a helper for an abstain vote at the given confidence for a role.
func abstainVote(role, confidence string) Vote {
	return Vote{
		Verdict:    "abstain",
		Confidence: confidence,
		Cost:       "🟢",
		Complexity: "🟢",
		Argument:   "n/a",
		Role:       role,
	}
}

// AC: high-confidence-abstain-excluded-from-denominator — a high-confidence
// abstain is removed from its group's denominator and listed in ExcludedVotes,
// while evaluation proceeds normally to a verdict.
func TestEvaluate_HighConfidenceAbstainExcludedFromDenominator(t *testing.T) {
	roster := defaultRosterSnapshot()
	votes := []Vote{
		approveVote("engineer", "high", "🟢", "🟢"),
		approveVote("architect", "high", "🟢", "🟢"),
		approveVote("qa", "high", "🟢", "🟢"),
		// One customer abstains at high confidence; the other two approve.
		abstainVote("pm", "high"),
		approveVote("ux", "high", "🟢", "🟢"),
		approveVote("marketing", "high", "🟢", "🟢"),
	}

	got := Evaluate(votes, roster, StrictBaseline())

	if got.Denominators.Customers != 2 {
		t.Errorf("Denominators.Customers = %d, want 2", got.Denominators.Customers)
	}
	if !contains(got.ExcludedVotes, "pm") {
		t.Errorf("ExcludedVotes = %v, want it to contain %q", got.ExcludedVotes, "pm")
	}
	// Both remaining customers approve, so the customer gate passes and the
	// panel reaches should-implement (high-conf abstain does not stop eval).
	if got.Verdict != VerdictShouldImplement {
		t.Errorf("Verdict = %q, want %q", got.Verdict, VerdictShouldImplement)
	}
}

// AC: low-confidence-abstain-caps-verdict — a low-confidence abstain caps the
// verdict at needs-human-review via low-abstain-veto, before the strict-gate
// path (steps 5–11) is evaluated.
func TestEvaluate_LowConfidenceAbstainCapsVerdict(t *testing.T) {
	roster := defaultRosterSnapshot()
	votes := []Vote{
		abstainVote("engineer", "low"),
		approveVote("architect", "high", "🟢", "🟢"),
		approveVote("qa", "high", "🟢", "🟢"),
		approveVote("pm", "high", "🟢", "🟢"),
		approveVote("ux", "high", "🟢", "🟢"),
		approveVote("marketing", "high", "🟢", "🟢"),
	}

	got := Evaluate(votes, roster, StrictBaseline())

	if got.Verdict != VerdictNeedsHumanReview {
		t.Fatalf("Verdict = %q, want %q", got.Verdict, VerdictNeedsHumanReview)
	}
	if !contains(got.RuleTrace, ruleLowAbstainVeto) {
		t.Errorf("RuleTrace = %v, want it to contain %q", got.RuleTrace, ruleLowAbstainVeto)
	}
	// The strict-gate path must not have been evaluated.
	for _, r := range []string{ruleBuilderGate, ruleCustomerGate, ruleConfidenceGate, ruleCostGate, ruleComplexityGate, ruleShouldImplement} {
		if contains(got.RuleTrace, r) {
			t.Errorf("RuleTrace = %v, want it NOT to contain strict-gate rule %q", got.RuleTrace, r)
		}
	}
}

// AC: adversary-veto-blocks — with strict-baseline knobs an adversary's
// high-confidence should-not-implement fires adversary-veto and stops before
// the approval-count steps.
func TestEvaluate_AdversaryVetoBlocks(t *testing.T) {
	roster := defaultRosterSnapshot()
	votes := []Vote{
		approveVote("engineer", "high", "🟢", "🟢"),
		approveVote("architect", "high", "🟢", "🟢"),
		approveVote("qa", "high", "🟢", "🟢"),
		approveVote("pm", "high", "🟢", "🟢"),
		approveVote("ux", "high", "🟢", "🟢"),
		approveVote("marketing", "high", "🟢", "🟢"),
		// One adversary vetoes at high confidence.
		{Verdict: "should-not-implement", Confidence: "high", Cost: "🟢", Complexity: "🟢", Argument: "no", Role: "skeptic"},
	}

	got := Evaluate(votes, roster, StrictBaseline())

	if got.Verdict != VerdictNeedsHumanReview {
		t.Fatalf("Verdict = %q, want %q", got.Verdict, VerdictNeedsHumanReview)
	}
	if !contains(got.RuleTrace, ruleAdversaryVeto) {
		t.Errorf("RuleTrace = %v, want it to contain %q", got.RuleTrace, ruleAdversaryVeto)
	}
	// Evaluation must stop before the approval-count steps.
	for _, r := range []string{ruleBuilderGate, ruleCustomerGate, ruleShouldImplement} {
		if contains(got.RuleTrace, r) {
			t.Errorf("RuleTrace = %v, want it NOT to contain approval-count rule %q", got.RuleTrace, r)
		}
	}
}

// ceilingValue maps an unexpected ceiling knob (outside low|medium) to the top
// of the scale (2) so it never silently blocks a vote.
func TestCeilingValue_UnknownKnobIsTopOfScale(t *testing.T) {
	if got := ceilingValue("high"); got != 2 {
		t.Errorf("ceilingValue(high) = %d, want 2", got)
	}
	if got := ceilingValue("low"); got != 0 {
		t.Errorf("ceilingValue(low) = %d, want 0", got)
	}
}

// ceilDiv returns ceil(a/b).
func TestCeilDiv(t *testing.T) {
	cases := []struct{ a, b, want int }{
		{4, 3, 2}, // ceil(4/3)=2
		{6, 3, 2}, // exact
		{0, 3, 0},
		{1, 3, 1},
	}
	for _, c := range cases {
		if got := ceilDiv(c.a, c.b); got != c.want {
			t.Errorf("ceilDiv(%d,%d) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// majority returns false for an empty group (n == 0).
func TestMajority_EmptyGroup(t *testing.T) {
	if majority(0, 0) {
		t.Errorf("majority(0,0) = true, want false")
	}
}

// medianOrdinal of an empty slice is 0.
func TestMedianOrdinal_Empty(t *testing.T) {
	if got := medianOrdinal(nil); got != 0 {
		t.Errorf("medianOrdinal(nil) = %d, want 0", got)
	}
}

// approvalGate returns false when there are no non-abstain voters.
func TestApprovalGate_NoNonAbstain(t *testing.T) {
	if approvalGate(0, 0, true) {
		t.Errorf("approvalGate(0,0,requireAll) = true, want false")
	}
	if approvalGate(0, 0, false) {
		t.Errorf("approvalGate(0,0,supermajority) = true, want false")
	}
}

// approvalGate supermajority path: with requireAll=false a two-thirds
// supermajority (ceil(2/3·n)) is enough; one builder rejecting out of three
// still clears the gate.
func TestApprovalGate_SupermajorityPath(t *testing.T) {
	// 2 of 3 approve -> ceil(2*3/3)=2 -> passes.
	if !approvalGate(2, 3, false) {
		t.Errorf("approvalGate(2,3,supermajority) = false, want true")
	}
	// 1 of 3 approve -> needs 2 -> fails.
	if approvalGate(1, 3, false) {
		t.Errorf("approvalGate(1,3,supermajority) = true, want false")
	}
}

// With require_all_builders:false and require_all_customers:false, a single
// dissenter per group still clears the two-thirds supermajority approval gates,
// exercising ceilDiv/majority/approvalGate non-unanimous branches end to end.
func TestEvaluate_SupermajorityApprovesWithDissenter(t *testing.T) {
	roster := defaultRosterSnapshot()
	votes := []Vote{
		approveVote("engineer", "high", "🟢", "🟢"),
		approveVote("architect", "high", "🟢", "🟢"),
		// qa rejects; 2/3 builders still approve.
		{Verdict: "should-not-implement", Confidence: "high", Cost: "🟢", Complexity: "🟢", Argument: "no", Role: "qa"},
		approveVote("pm", "high", "🟢", "🟢"),
		approveVote("ux", "high", "🟢", "🟢"),
		// marketing rejects; 2/3 customers still approve.
		{Verdict: "should-not-implement", Confidence: "high", Cost: "🟢", Complexity: "🟢", Argument: "no", Role: "marketing"},
	}
	knobs := StrictBaseline()
	knobs.RequireAllBuilders = false
	knobs.RequireAllCustomers = false

	got := Evaluate(votes, roster, knobs)

	if got.Verdict != VerdictShouldImplement {
		t.Fatalf("Verdict = %q, want %q", got.Verdict, VerdictShouldImplement)
	}
}

// Step-12 should-not-implement via majority (not unanimity): a single approver
// per group still leaves a strict majority rejecting, so both group majorities
// reject and the verdict is should-not-implement.
func TestEvaluate_ShouldNotImplementByMajority(t *testing.T) {
	roster := defaultRosterSnapshot()
	reject := func(role string) Vote {
		return Vote{Verdict: "should-not-implement", Confidence: "high", Cost: "🟢", Complexity: "🟢", Argument: "no", Role: role}
	}
	votes := []Vote{
		reject("engineer"), reject("architect"),
		approveVote("qa", "high", "🟢", "🟢"),
		reject("pm"), reject("ux"),
		approveVote("marketing", "high", "🟢", "🟢"),
	}
	knobs := StrictBaseline()
	knobs.RequireAllBuilders = false
	knobs.RequireAllCustomers = false

	got := Evaluate(votes, roster, knobs)

	if got.Verdict != VerdictShouldNotImplement {
		t.Fatalf("Verdict = %q, want %q", got.Verdict, VerdictShouldNotImplement)
	}
	if last := got.RuleTrace[len(got.RuleTrace)-1]; last != ruleShouldNotImplement {
		t.Errorf("RuleTrace ends in %q, want %q", last, ruleShouldNotImplement)
	}
}

// High-confidence abstains from a builder and an adversary are both excluded
// from their group denominators, exercising the builder and adversary arms of
// the step-2 exclusion switch.
func TestEvaluate_HighConfidenceAbstainExcludesBuilderAndAdversary(t *testing.T) {
	roster := defaultRosterSnapshot()
	votes := []Vote{
		abstainVote("engineer", "high"), // builder excluded
		approveVote("architect", "high", "🟢", "🟢"),
		approveVote("qa", "high", "🟢", "🟢"),
		approveVote("pm", "high", "🟢", "🟢"),
		approveVote("ux", "high", "🟢", "🟢"),
		approveVote("marketing", "high", "🟢", "🟢"),
		abstainVote("skeptic", "high"), // adversary excluded
	}

	got := Evaluate(votes, roster, StrictBaseline())

	if got.Denominators.Builders != 2 {
		t.Errorf("Denominators.Builders = %d, want 2", got.Denominators.Builders)
	}
	if got.Denominators.Adversaries != 2 {
		t.Errorf("Denominators.Adversaries = %d, want 2", got.Denominators.Adversaries)
	}
	if !contains(got.ExcludedVotes, "engineer") || !contains(got.ExcludedVotes, "skeptic") {
		t.Errorf("ExcludedVotes = %v, want engineer and skeptic", got.ExcludedVotes)
	}
}

// An even count of non-abstain votes exercises medianOrdinal's lower-middle
// tie-break inside Evaluate. With knobs allowing supermajority and four
// builder-only votes of mixed cost, the median picks the lower-middle.
func TestEvaluate_EvenNonAbstainCountMedian(t *testing.T) {
	roster := []RosterEntry{
		{Name: "engineer", Group: GroupBuilders},
		{Name: "architect", Group: GroupBuilders},
		{Name: "qa", Group: GroupBuilders},
		{Name: "pm", Group: GroupBuilders},
	}
	// Costs [🟢,🟢,🔴,🔴] sorted -> [0,0,2,2], lower-middle index 1 -> 0 (passes
	// cost ceiling medium). All approve at high confidence.
	votes := []Vote{
		approveVote("engineer", "high", "🟢", "🟢"),
		approveVote("architect", "high", "🟢", "🟢"),
		approveVote("qa", "high", "🔴", "🟢"),
		approveVote("pm", "high", "🔴", "🟢"),
	}
	knobs := StrictBaseline()
	knobs.RequireAllCustomers = false // no customers; customer gate would fail otherwise

	got := Evaluate(votes, roster, knobs)

	// No customers -> customer approvalGate fails (nonAbstain==0) -> not
	// should-implement; the even-count median path is still exercised.
	if got.Verdict == "" {
		t.Fatalf("Verdict empty")
	}
}

// Marshal normalizes nil RuleTrace and nil ExcludedVotes to empty lists so a
// zero-value Result still serializes deterministically.
func TestMarshal_NilSlicesNormalized(t *testing.T) {
	r := Result{Verdict: VerdictNeedsHumanReview} // RuleTrace and ExcludedVotes nil
	out, err := r.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(out)
	for _, want := range []string{"rule_trace: []", "excluded_votes: []"} {
		if !strings.Contains(s, want) {
			t.Errorf("Marshal output missing %q; got:\n%s", want, s)
		}
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
