package issue

import "testing"

// TestIssueBidirectionalMatrix_DerivedFromLegalTransitions asserts every
// edge agrees with IsLegalTransition/LegalTransitions — the same map, not a
// hand-duplicated second one.
func TestIssueBidirectionalMatrix_DerivedFromLegalTransitions(t *testing.T) {
	edges := BidirectionalMatrix()
	forward := LegalTransitions()

	byStatus := map[string]StatusEdge{}
	for _, e := range edges {
		byStatus[e.Status] = e
	}

	for from, tos := range forward {
		e, ok := byStatus[from]
		if !ok {
			t.Fatalf("status %q missing from BidirectionalMatrix", from)
		}
		if len(e.Next) != len(tos) {
			t.Errorf("%s: Next = %v, want %v", from, e.Next, tos)
		}
		for _, to := range tos {
			if !IsLegalTransition(from, to) {
				t.Fatalf("test setup: LegalTransitions and IsLegalTransition disagree for %s->%s", from, to)
			}
			found := false
			for _, n := range e.Next {
				if n == to {
					found = true
				}
			}
			if !found {
				t.Errorf("%s: Next=%v missing legal target %s", from, e.Next, to)
			}
		}
	}
}

func TestIssueBidirectionalMatrix_TerminalStatusesHaveNoNext(t *testing.T) {
	for _, e := range BidirectionalMatrix() {
		if e.Status == "resolved" || e.Status == "rejected" {
			if len(e.Next) != 0 {
				t.Errorf("%s: expected terminal (no Next), got %v", e.Status, e.Next)
			}
			if len(e.Previous) == 0 {
				t.Errorf("%s: expected a non-empty Previous", e.Status)
			}
		}
	}
}

func TestIssueEdgeFor_UnknownStatusReturnsEmptyEdge(t *testing.T) {
	e := EdgeFor("not-a-real-status")
	if len(e.Previous) != 0 || len(e.Next) != 0 {
		t.Errorf("EdgeFor(unknown) = %+v, want zero-value edges", e)
	}
}

func TestIssueRenderBidirectionalMatrix_MentionsEveryStatus(t *testing.T) {
	out := RenderBidirectionalMatrix()
	for _, s := range []string{"open", "investigating", "resolved", "rejected"} {
		if !contains(out, s) {
			t.Errorf("RenderBidirectionalMatrix missing %q:\n%s", s, out)
		}
	}
	if !contains(out, "terminal status") {
		t.Errorf("RenderBidirectionalMatrix does not mark any status terminal:\n%s", out)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return len(sub) == 0
}
