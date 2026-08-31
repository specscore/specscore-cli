package lifecycle

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBidirectionalMatrix_DerivedFromSameMatrixAsTransition asserts every
// edge BidirectionalMatrix reports agrees with the primitives Transition
// itself consults (LegalSources/LegalTargets) — there is no second table to
// drift out of sync.
func TestBidirectionalMatrix_DerivedFromSameMatrixAsTransition(t *testing.T) {
	for _, kind := range []Kind{KindIdea, KindFeature, KindPlan, KindTask, KindLesson, KindDecision} {
		edges := BidirectionalMatrix(kind)
		statuses := LegalStatuses(kind)
		if len(edges) != len(statuses) {
			t.Fatalf("%s: BidirectionalMatrix returned %d edges, want %d (one per LegalStatuses entry)",
				kind, len(edges), len(statuses))
		}
		for _, e := range edges {
			wantNext := LegalTargets(kind, e.Status)
			wantPrev := LegalSources(kind, e.Status)
			if !statusSlicesEqual(e.Next, wantNext) {
				t.Errorf("%s %s: Next = %v, want %v (LegalTargets)", kind, e.Status, e.Next, wantNext)
			}
			if !statusSlicesEqual(e.Previous, wantPrev) {
				t.Errorf("%s %s: Previous = %v, want %v (LegalSources)", kind, e.Status, e.Previous, wantPrev)
			}
			// Every transition Transition() accepts must appear in Next.
			for _, to := range wantNext {
				if Transition(kind, e.Status, to) != nil {
					t.Errorf("%s: Transition(%s, %s) rejected a pair BidirectionalMatrix listed as legal", kind, e.Status, to)
				}
			}
		}
	}
}

func statusSlicesEqual(a, b []Status) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestBidirectionalMatrix_TerminalAndInitialStatusesAreVisible asserts that
// Feature's known terminal statuses (Rejected, Deprecated) report an empty
// Next, and Draft — reachable only via `feature new`, never change-status —
// reports an empty Previous. This is exactly the "is it terminal, or just
// missing from the table?" ambiguity the founder flagged in --help's
// forward-only rendering.
func TestBidirectionalMatrix_TerminalAndInitialStatusesAreVisible(t *testing.T) {
	edges := BidirectionalMatrix(KindFeature)
	byStatus := map[Status]StatusEdge{}
	for _, e := range edges {
		byStatus[e.Status] = e
	}

	for _, terminal := range []Status{FeatureRejected, FeatureDeprecated} {
		e, ok := byStatus[terminal]
		if !ok {
			t.Fatalf("expected %s in the Feature matrix", terminal)
		}
		if !e.IsTerminal() {
			t.Errorf("%s: expected IsTerminal()==true (Next=%v)", terminal, e.Next)
		}
	}

	draft, ok := byStatus[FeatureDraft]
	if !ok {
		t.Fatal("expected Draft in the Feature matrix")
	}
	if !draft.IsInitialOnly() {
		t.Errorf("Draft: expected IsInitialOnly()==true (Previous=%v)", draft.Previous)
	}
	if draft.IsTerminal() {
		t.Error("Draft: expected IsTerminal()==false — it has outgoing arcs")
	}

	// Planned is a legacy, pre-vocabulary status (see FeaturePlanned's doc
	// comment) with the same "no legal predecessor" shape as Draft. Before
	// this fix it had NO outgoing arcs either, so it rendered as BOTH
	// initial-only AND terminal — a dead end indistinguishable from a
	// genuinely terminal status. It now has outgoing arcs, so it must read
	// as initial-only but NOT terminal, same as Draft.
	planned, ok := byStatus[FeaturePlanned]
	if !ok {
		t.Fatal("expected Planned in the Feature matrix")
	}
	if !planned.IsInitialOnly() {
		t.Errorf("Planned: expected IsInitialOnly()==true (Previous=%v)", planned.Previous)
	}
	if planned.IsTerminal() {
		t.Error("Planned: expected IsTerminal()==false — it now has outgoing arcs")
	}
}

func TestEdgeFor_MatchesBidirectionalMatrixEntry(t *testing.T) {
	got := EdgeFor(KindFeature, FeatureApproved)
	for _, e := range BidirectionalMatrix(KindFeature) {
		if e.Status == FeatureApproved {
			if !statusSlicesEqual(got.Next, e.Next) || !statusSlicesEqual(got.Previous, e.Previous) {
				t.Errorf("EdgeFor(%s) = %+v, want %+v", FeatureApproved, got, e)
			}
			return
		}
	}
	t.Fatal("Approved not found in BidirectionalMatrix(KindFeature)")
}

func TestCurrentStatus_ReadsWithoutValidatingOrMutating(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.md")
	original := "# Title\n\n**Status:** Draft\n\nbody\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, err := CurrentStatus(KindFeature, path)
	if err != nil {
		t.Fatalf("CurrentStatus: %v", err)
	}
	if got != FeatureDraft {
		t.Errorf("CurrentStatus = %q, want %q", got, FeatureDraft)
	}

	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("re-read fixture: %v", readErr)
	}
	if string(after) != original {
		t.Errorf("CurrentStatus mutated the file:\nbefore=%q\nafter=%q", original, string(after))
	}
}

func TestCurrentStatus_NoStatusLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.md")
	if err := os.WriteFile(path, []byte("# Title\n\nno status line here\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := CurrentStatus(KindFeature, path); err == nil {
		t.Fatal("expected an error for a file with no **Status:** line")
	}
}

func TestRenderBidirectionalMatrix_NamesEveryStatusOnceWithPreviousAndNext(t *testing.T) {
	out := RenderBidirectionalMatrix(KindFeature)
	for _, s := range LegalStatuses(KindFeature) {
		if !contains(out, string(s)) {
			t.Errorf("RenderBidirectionalMatrix output missing status %q:\n%s", s, out)
		}
	}
	if !contains(out, "previous:") || !contains(out, "next:") {
		t.Errorf("RenderBidirectionalMatrix output missing previous:/next: labels:\n%s", out)
	}
	if !contains(out, "terminal status") {
		t.Errorf("RenderBidirectionalMatrix output does not visibly mark any terminal status:\n%s", out)
	}
}

func TestRenderEdges_CustomLabelForNonLifecycleKind(t *testing.T) {
	edges := []StatusEdge{
		{Status: "Queued", Previous: nil, Next: []Status{"Implemented", "Rejected"}},
		{Status: "Implemented", Previous: []Status{"Queued"}, Next: nil},
	}
	out := RenderEdges("sidekick", edges)
	if !contains(out, "Status vocabulary for sidekick") {
		t.Errorf("RenderEdges did not use the supplied label:\n%s", out)
	}
	if !contains(out, "Queued") || !contains(out, "Implemented") {
		t.Errorf("RenderEdges missing expected statuses:\n%s", out)
	}
}
