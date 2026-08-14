package sidekick

import (
	"testing"

	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// TestSidekickBidirectionalMatrix_DerivedFromSeedTargets asserts every edge
// agrees with seedLegalSource/seedTargets — the same values
// ParseSeedTarget/CheckSeedSource already consult — rather than a
// hand-duplicated second list.
func TestSidekickBidirectionalMatrix_DerivedFromSeedTargets(t *testing.T) {
	edges := BidirectionalMatrix()
	if len(edges) != 1+len(seedTargets) {
		t.Fatalf("BidirectionalMatrix returned %d edges, want %d (Queued + %d targets)",
			len(edges), 1+len(seedTargets), len(seedTargets))
	}

	byStatus := map[lifecycle.Status]lifecycle.StatusEdge{}
	for _, e := range edges {
		byStatus[e.Status] = e
	}

	queued, ok := byStatus[SeedQueued]
	if !ok {
		t.Fatal("Queued missing from BidirectionalMatrix")
	}
	if !queued.IsInitialOnly() {
		t.Errorf("Queued: expected IsInitialOnly()==true, Previous=%v", queued.Previous)
	}
	if len(queued.Next) != len(seedTargets) {
		t.Errorf("Queued: Next=%v, want %v", queued.Next, seedTargets)
	}

	for _, target := range seedTargets {
		e, ok := byStatus[target]
		if !ok {
			t.Fatalf("%s missing from BidirectionalMatrix", target)
		}
		if !e.IsTerminal() {
			t.Errorf("%s: expected IsTerminal()==true, Next=%v", target, e.Next)
		}
		if len(e.Previous) != 1 || e.Previous[0] != SeedQueued {
			t.Errorf("%s: Previous=%v, want [Queued]", target, e.Previous)
		}
	}
}

func TestSidekickEdgeFor_UnknownStatusReturnsEmptyEdge(t *testing.T) {
	e := EdgeFor("NotARealStatus")
	if len(e.Previous) != 0 || len(e.Next) != 0 {
		t.Errorf("EdgeFor(unknown) = %+v, want zero-value edges", e)
	}
}

func TestSidekickEdgeForAndParseStatus_KnownAndUnknown(t *testing.T) {
	edge := EdgeFor(SeedQueued)
	if edge.Status != SeedQueued || len(edge.Next) != len(seedTargets) {
		t.Fatalf("EdgeFor(Queued) = %+v", edge)
	}
	if got := ParseStatus(" queued "); got != SeedQueued {
		t.Fatalf("ParseStatus(queued) = %q, want %q", got, SeedQueued)
	}
	if got := ParseStatus("custom"); got != "custom" {
		t.Fatalf("ParseStatus(custom) = %q", got)
	}
}
