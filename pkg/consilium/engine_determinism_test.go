package consilium

import (
	"bytes"
	"testing"
)

// determinismCase bundles an input triple with a human-readable name so the
// determinism checks can exercise several distinct inputs.
type determinismCase struct {
	name   string
	votes  []Vote
	roster []RosterEntry
	knobs  GateKnobs
}

// determinismCases returns one input triple per terminal verdict plus the
// abstain-exclusion and the two veto cases, so the order-independence check
// interleaves a representative spread of engine paths.
func determinismCases() []determinismCase {
	roster := defaultRosterSnapshot()
	approveAll := func() []Vote {
		return []Vote{
			approveVote("engineer", "high", "🟢", "🟢"),
			approveVote("architect", "high", "🟢", "🟢"),
			approveVote("qa", "high", "🟢", "🟢"),
			approveVote("pm", "high", "🟢", "🟢"),
			approveVote("ux", "high", "🟢", "🟢"),
			approveVote("marketing", "high", "🟢", "🟢"),
		}
	}
	reject := func(role string) Vote {
		return Vote{Verdict: "should-not-implement", Confidence: "high", Cost: "🟢", Complexity: "🟢", Argument: "no", Role: role}
	}

	rejectAll := []Vote{
		reject("engineer"), reject("architect"), reject("qa"),
		reject("pm"), reject("ux"), reject("marketing"),
	}

	split := approveAll()
	split[2] = reject("qa")

	highAbstain := approveAll()
	highAbstain[3] = abstainVote("pm", "high")

	lowAbstain := approveAll()
	lowAbstain[0] = abstainVote("engineer", "low")

	advVeto := append(approveAll(), reject("skeptic"))

	return []determinismCase{
		{"should-implement", approveAll(), roster, StrictBaseline()},
		{"should-not-implement", rejectAll, roster, StrictBaseline()},
		{"needs-human-review", split, roster, StrictBaseline()},
		{"high-abstain-excluded", highAbstain, roster, StrictBaseline()},
		{"low-abstain-veto", lowAbstain, roster, StrictBaseline()},
		{"adversary-veto", advVeto, roster, StrictBaseline()},
	}
}

// AC: gate-engine-deterministic (in-process leg) — evaluating the same input
// triple twice in the same process MUST produce byte-identical serialized
// Results. Combined with the committed goldens in snapshot_test.go (the
// fresh-process leg), this proves the engine's output is a pure function of its
// inputs and never varies with wall-clock time.
func TestEvaluate_DeterministicAcrossRepeatedCalls(t *testing.T) {
	for _, tc := range determinismCases() {
		t.Run(tc.name, func(t *testing.T) {
			first, err := Evaluate(tc.votes, tc.roster, tc.knobs).Marshal()
			if err != nil {
				t.Fatalf("Marshal (first) error: %v", err)
			}
			second, err := Evaluate(tc.votes, tc.roster, tc.knobs).Marshal()
			if err != nil {
				t.Fatalf("Marshal (second) error: %v", err)
			}
			if !bytes.Equal(first, second) {
				t.Errorf("serialized Result differs between calls:\nfirst:\n%s\nsecond:\n%s", first, second)
			}
		})
	}
}

// AC: gate-engine-deterministic (run-order leg) — no field MUST vary with run
// order. Evaluating the cases interleaved (each twice, in a shuffled-by-index
// order) and comparing each case against a baseline computed in isolation
// confirms the engine carries no cross-call state.
func TestEvaluate_IndependentOfCallOrder(t *testing.T) {
	cases := determinismCases()

	// Baseline: each case serialized on its own, in declaration order.
	baseline := make(map[string][]byte, len(cases))
	for _, tc := range cases {
		b, err := Evaluate(tc.votes, tc.roster, tc.knobs).Marshal()
		if err != nil {
			t.Fatalf("baseline Marshal for %q: %v", tc.name, err)
		}
		baseline[tc.name] = b
	}

	// Interleave: walk the cases in a non-declaration order, twice, and assert
	// every result still matches its isolated baseline.
	order := []int{5, 0, 3, 1, 4, 2, 2, 4, 1, 3, 0, 5}
	for _, idx := range order {
		tc := cases[idx]
		got, err := Evaluate(tc.votes, tc.roster, tc.knobs).Marshal()
		if err != nil {
			t.Fatalf("interleaved Marshal for %q: %v", tc.name, err)
		}
		if !bytes.Equal(got, baseline[tc.name]) {
			t.Errorf("case %q varied with call order:\nbaseline:\n%s\ngot:\n%s", tc.name, baseline[tc.name], got)
		}
	}
}
