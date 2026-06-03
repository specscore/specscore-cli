package consilium

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// update, when set via `go test -update`, regenerates the committed golden
// files from the current engine output instead of asserting against them. The
// goldens are committed so CI gates without -update; regenerate only on an
// intentional, reviewed output change.
var update = flag.Bool("update", false, "regenerate golden snapshot files")

// goldenDir is the testdata subtree holding one golden YAML per fixture.
const goldenDir = "testdata/consilium"

// snapshotFixture is one named input triple whose serialized Result is locked
// to a committed golden file. The goldens were captured once in a prior process
// (via -update); re-deriving the Result here and byte-comparing to the golden
// is the cross-process leg of AC:gate-engine-deterministic.
type snapshotFixture struct {
	name   string
	votes  []Vote
	roster []RosterEntry
	knobs  GateKnobs
}

// snapshotFixtures covers at least one fixture per terminal verdict
// (should-implement, should-not-implement, needs-human-review) plus the
// abstain-exclusion case and the two veto cases (low-abstain-veto,
// adversary-veto). Inputs reuse the engine_test.go vote helpers.
func snapshotFixtures() []snapshotFixture {
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

	return []snapshotFixture{
		{"should-implement", approveAll(), roster, StrictBaseline()},
		{"should-not-implement", rejectAll, roster, StrictBaseline()},
		{"needs-human-review", split, roster, StrictBaseline()},
		{"high-confidence-abstain-excluded", highAbstain, roster, StrictBaseline()},
		{"low-abstain-veto", lowAbstain, roster, StrictBaseline()},
		{"adversary-veto", advVeto, roster, StrictBaseline()},
	}
}

// TestSnapshot is the golden-fixture suite CI gates on. For each fixture it
// re-derives the Result and asserts the serialized bytes are byte-identical to
// the committed golden. Run with -update to (re)generate the goldens.
func TestSnapshot(t *testing.T) {
	for _, f := range snapshotFixtures() {
		t.Run(f.name, func(t *testing.T) {
			got, err := Evaluate(f.votes, f.roster, f.knobs).Marshal()
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}
			path := filepath.Join(goldenDir, f.name+".golden.yaml")

			if *update {
				if err := os.MkdirAll(goldenDir, 0o755); err != nil {
					t.Fatalf("mkdir %s: %v", goldenDir, err)
				}
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatalf("write golden %s: %v", path, err)
				}
				return
			}

			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden %s: %v (run `go test -update` to generate)", path, err)
			}
			if string(got) != string(want) {
				t.Errorf("snapshot mismatch for %q:\n--- got ---\n%s\n--- want ---\n%s", f.name, got, want)
			}
		})
	}
}
