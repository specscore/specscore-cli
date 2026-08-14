package lifecycle

import (
	"fmt"
	"strings"
)

// StatusEdge is one status's place in a kind's transition matrix: which
// statuses can legally transition INTO it (Previous) and which it can
// legally transition TO (Next). Both are derived from the SAME
// transitionMatrix rows Transition/LegalTargets/LegalSources already
// consult — there is no second, hand-authored table to drift out of sync
// with the forward-only matrix.
//
// An empty Previous means the status is only reachable by initial creation
// (e.g. Feature's Draft, written by `feature new`, never by change-status).
// An empty Next means the status is terminal — no change-status verb can
// move an artifact away from it.
type StatusEdge struct {
	Status   Status   `json:"status" yaml:"status"`
	Previous []Status `json:"previous" yaml:"previous"`
	Next     []Status `json:"next" yaml:"next"`
}

// IsInitialOnly reports whether Status has no legal predecessor — it is
// reachable only by the kind's `new`/scaffold verb, never by change-status.
func (e StatusEdge) IsInitialOnly() bool { return len(e.Previous) == 0 }

// IsTerminal reports whether Status has no legal successor — no
// change-status verb can move an artifact away from it.
func (e StatusEdge) IsTerminal() bool { return len(e.Next) == 0 }

// EdgeFor returns the StatusEdge for a single (kind, status) pair. It is the
// primitive `<kind> transitions <slug>` uses to report what a specific
// artifact — already read at its current status — can legally become next.
func EdgeFor(kind Kind, status Status) StatusEdge {
	return StatusEdge{
		Status:   status,
		Previous: LegalSources(kind, status),
		Next:     LegalTargets(kind, status),
	}
}

// BidirectionalMatrix returns a StatusEdge for every status LegalStatuses
// recognizes for kind, sorted alphabetically by Status. This is the
// complete status vocabulary for the kind — including statuses that never
// appear as a From (initial-only) or never appear as a To (terminal) in the
// forward-only matrix — so a reader never has to infer "is this terminal,
// or just missing from the table?" by hand.
func BidirectionalMatrix(kind Kind) []StatusEdge {
	statuses := LegalStatuses(kind)
	out := make([]StatusEdge, len(statuses))
	for i, s := range statuses {
		out[i] = EdgeFor(kind, s)
	}
	return out
}

// CurrentStatus reads artifactPath's current Status without validating any
// transition — the read-only counterpart to Validate, used by query verbs
// (`<kind> transitions <slug>`) that report state without a --to target.
func CurrentStatus(kind Kind, artifactPath string) (Status, error) {
	return readStatus(artifactPath)
}

// statusList renders a Status slice as a comma-separated string, or a
// bracketed placeholder when empty.
func statusList(ss []Status, emptyPlaceholder string) string {
	if len(ss) == 0 {
		return emptyPlaceholder
	}
	names := make([]string, len(ss))
	for i, s := range ss {
		names[i] = string(s)
	}
	return strings.Join(names, ", ")
}

// RenderBidirectionalMatrix formats BidirectionalMatrix(kind) as ANSI-free
// text: every recognized status for kind, each with its previous/next
// legal-transition lists, with initial-only and terminal statuses spelled
// out rather than left to look like an omission.
//
//	Status vocabulary for feature (8 statuses):
//
//	  Amending
//	    previous: Approved, Stable
//	    next:     Approved, Stable
//
//	  Approved
//	    previous: Draft, In Review
//	    next:     Amending, Deprecated, Implementing, Rejected
//	  ...
func RenderBidirectionalMatrix(kind Kind) string {
	return RenderEdges(string(kind), BidirectionalMatrix(kind))
}

// RenderEdges formats a []StatusEdge as the same ANSI-free text
// RenderBidirectionalMatrix produces, labeled with label instead of a
// lifecycle.Kind. It exists so a kind with its OWN small matrix outside
// pkg/lifecycle (e.g. pkg/issue, pkg/sidekick — neither is a registered
// Kind here) can still render in the identical shape, without duplicating
// this formatting logic.
func RenderEdges(label string, edges []StatusEdge) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Status vocabulary for %s (%d status(es)):\n", label, len(edges))
	for _, e := range edges {
		fmt.Fprintf(&b, "\n  %s\n", string(e.Status))
		prev := statusList(e.Previous, "(none — initial status, set only by the kind's scaffold/new verb)")
		next := statusList(e.Next, "(none — terminal status)")
		fmt.Fprintf(&b, "    previous: %s\n", prev)
		fmt.Fprintf(&b, "    next:     %s\n", next)
	}
	return b.String()
}
