package consilium

import "sort"

// Verdict is one of the three terminal consilium outcomes
// (REQ:gate-engine-rule-order).
type Verdict string

const (
	VerdictShouldImplement    Verdict = "should-implement"
	VerdictShouldNotImplement Verdict = "should-not-implement"
	VerdictNeedsHumanReview   Verdict = "needs-human-review"
)

// Rule names recorded in a Result.RuleTrace, one per gate-algorithm step that
// fired. The names are stable so snapshot tests can string-match on them.
const (
	ruleBuilderGate        = "builder-gate"
	ruleCustomerGate       = "customer-gate"
	ruleConfidenceGate     = "confidence-gate"
	ruleCostGate           = "cost-gate"
	ruleComplexityGate     = "complexity-gate"
	ruleShouldImplement    = "should-implement"
	ruleShouldNotImplement = "should-not-implement"
	ruleNeedsHumanReview   = "needs-human-review"
)

// Denominators holds the per-group counts used as the consensus denominators,
// after abstain exclusion. Steps 2–4 (which would shrink these) land in a later
// task; for now they mirror the roster's per-group counts.
type Denominators struct {
	Builders    int
	Customers   int
	Adversaries int
}

// Result is the deterministic output of the gate engine. RuleTrace lists the
// rule names of the steps that fired, in evaluation order, ending in the
// terminal-verdict rule. ExcludedVotes lists the role slugs excluded in step 2
// (empty until that step lands). Denominators are the per-group counts after
// exclusion.
type Result struct {
	Verdict       Verdict
	RuleTrace     []string
	ExcludedVotes []string
	Denominators  Denominators
}

// confidenceRank / cellRank map the vote enum values onto ordinal scales for
// median comparison: confidence low<medium<high; cost/complexity 🟢<🟡<🔴.
var (
	confidenceRank = map[string]int{"low": 0, "medium": 1, "high": 2}
	cellRank       = map[string]int{"🟢": 0, "🟡": 1, "🔴": 2}
)

// ceilingRank maps a ceiling knob word (low|medium) onto the 🟢/🟡/🔴 ordinal
// scale that cost and complexity votes use: low→🟢(0), medium→🟡(1). Any other
// value is treated as the top of the scale so an unexpected knob never silently
// blocks a vote.
var ceilingRank = map[string]int{"low": 0, "medium": 1}

func ceilingValue(knob string) int {
	if r, ok := ceilingRank[knob]; ok {
		return r
	}
	return 2
}

// Evaluate runs the gate algorithm (REQ:gate-engine-rule-order) over the votes
// against the roster and knobs, producing a deterministic Result. It is a pure
// function: no clock, no randomness, no I/O.
//
// This task implements steps 1 and 5–13. Steps 2–4 (abstain exclusion,
// low-abstain-veto, adversary-veto) are inserted before step 5 by a later task;
// the structure leaves that seam open (ExcludedVotes is built empty, the
// denominators are computed once up front, and the per-group approval tallies
// run on the non-abstain votes).
func Evaluate(votes []Vote, roster []RosterEntry, knobs GateKnobs) Result {
	// Map each role slug to its group via the roster.
	groupOf := make(map[string]Group, len(roster))
	for _, e := range roster {
		groupOf[e.Name] = e.Group
	}

	// Step 1 (denominators): per-group roster counts. Step 2 exclusion (later)
	// would subtract from these.
	den := Denominators{}
	for _, e := range roster {
		switch e.Group {
		case GroupBuilders:
			den.Builders++
		case GroupCustomers:
			den.Customers++
		case GroupAdversaries:
			den.Adversaries++
		}
	}

	// Steps 2–4 land here in a later task. ExcludedVotes is empty for now.
	excluded := []string{}

	// Step 5: count non-abstain approvals per group, and gather the non-abstain
	// votes' ordinal scales for the median gates (steps 8–10).
	var builderApprove, builderReject, builderNonAbstain int
	var customerApprove, customerReject, customerNonAbstain int
	var confidences, costs, complexities []int

	for _, v := range votes {
		if v.Verdict == "abstain" {
			continue
		}
		confidences = append(confidences, confidenceRank[v.Confidence])
		costs = append(costs, cellRank[v.Cost])
		complexities = append(complexities, cellRank[v.Complexity])

		switch groupOf[v.Role] {
		case GroupBuilders:
			builderNonAbstain++
			switch v.Verdict {
			case "should-implement":
				builderApprove++
			case "should-not-implement":
				builderReject++
			}
		case GroupCustomers:
			customerNonAbstain++
			switch v.Verdict {
			case "should-implement":
				customerApprove++
			case "should-not-implement":
				customerReject++
			}
		}
	}

	trace := []string{}

	// Step 6: builder gate.
	builderPass := approvalGate(builderApprove, builderNonAbstain, knobs.RequireAllBuilders)
	trace = append(trace, ruleBuilderGate)

	// Step 7: customer gate.
	customerPass := approvalGate(customerApprove, customerNonAbstain, knobs.RequireAllCustomers)
	trace = append(trace, ruleCustomerGate)

	// Step 8: confidence floor — median confidence ≥ min_median_confidence.
	confidencePass := medianOrdinal(confidences) >= confidenceRank[knobs.MinMedianConfidence]
	trace = append(trace, ruleConfidenceGate)

	// Step 9: cost ceiling — median cost ≤ cost_ceiling.
	costPass := medianOrdinal(costs) <= ceilingValue(knobs.CostCeiling)
	trace = append(trace, ruleCostGate)

	// Step 10: complexity ceiling — median complexity ≤ complexity_ceiling.
	complexityPass := medianOrdinal(complexities) <= ceilingValue(knobs.ComplexityCeiling)
	trace = append(trace, ruleComplexityGate)

	// Step 11: all five gates pass → should-implement.
	if builderPass && customerPass && confidencePass && costPass && complexityPass {
		trace = append(trace, ruleShouldImplement)
		return Result{
			Verdict:       VerdictShouldImplement,
			RuleTrace:     trace,
			ExcludedVotes: excluded,
			Denominators:  den,
		}
	}

	// Step 12: builders AND customers both majority should-not-implement →
	// should-not-implement.
	if majority(builderReject, builderNonAbstain) && majority(customerReject, customerNonAbstain) {
		trace = append(trace, ruleShouldNotImplement)
		return Result{
			Verdict:       VerdictShouldNotImplement,
			RuleTrace:     trace,
			ExcludedVotes: excluded,
			Denominators:  den,
		}
	}

	// Step 13: otherwise → needs-human-review.
	trace = append(trace, ruleNeedsHumanReview)
	return Result{
		Verdict:       VerdictNeedsHumanReview,
		RuleTrace:     trace,
		ExcludedVotes: excluded,
		Denominators:  den,
	}
}

// approvalGate reports whether a group's approvals clear its gate. When
// requireAll is true every non-abstain voter in the group must approve; else a
// two-thirds supermajority (ceil(2/3·n)) is required. A group with no
// non-abstain votes fails the gate (nothing to approve it).
func approvalGate(approvals, nonAbstain int, requireAll bool) bool {
	if nonAbstain == 0 {
		return false
	}
	if requireAll {
		return approvals == nonAbstain
	}
	return approvals >= ceilDiv(2*nonAbstain, 3)
}

// majority reports whether count is a strict majority of n (> half). A group
// with no votes has no majority.
func majority(count, n int) bool {
	if n == 0 {
		return false
	}
	return 2*count > n
}

// ceilDiv returns ceil(a/b) for non-negative integers with b > 0.
func ceilDiv(a, b int) int {
	return (a + b - 1) / b
}

// medianOrdinal returns the median of a sorted copy of vals on its ordinal
// scale. For an even count the lower-middle element is chosen (deterministic
// tie-break: index (n-1)/2). An empty slice returns 0.
//
// The lower-middle rule matters only for even vote counts; the
// gate-engine-applies-rules-in-order AC uses odd counts, so it is unaffected,
// but determinism is required for later snapshot tests.
func medianOrdinal(vals []int) int {
	if len(vals) == 0 {
		return 0
	}
	sorted := make([]int, len(vals))
	copy(sorted, vals)
	sort.Ints(sorted)
	return sorted[(len(sorted)-1)/2]
}
