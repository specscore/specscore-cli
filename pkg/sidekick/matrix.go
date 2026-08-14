package sidekick

// Bidirectional matrix view over the seed's tiny linear state machine
// (transitions.go): every arc starts at seedLegalSource and ends at one of
// seedTargets. This derives previous/next edges from those SAME two values
// ParseSeedTarget and CheckSeedSource already consult, so
// `specscore sidekick transitions` never hand-maintains a second table.

import (
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// BidirectionalMatrix returns a lifecycle.StatusEdge for Queued and each of
// its terminal targets, sorted alphabetically by status.
func BidirectionalMatrix() []lifecycle.StatusEdge {
	edges := []lifecycle.StatusEdge{
		{
			Status:   seedLegalSource,
			Previous: nil,
			Next:     append([]lifecycle.Status{}, seedTargets...),
		},
	}
	for _, t := range seedTargets {
		edges = append(edges, lifecycle.StatusEdge{
			Status:   t,
			Previous: []lifecycle.Status{seedLegalSource},
			Next:     nil,
		})
	}
	sort.Slice(edges, func(i, j int) bool { return string(edges[i].Status) < string(edges[j].Status) })
	return edges
}

// EdgeFor returns the lifecycle.StatusEdge for a single seed status. status
// must already be in canonical title-case form (see ParseStatus) — EdgeFor
// itself does no case-folding, matching lifecycle.EdgeFor's contract.
func EdgeFor(status lifecycle.Status) lifecycle.StatusEdge {
	for _, e := range BidirectionalMatrix() {
		if e.Status == status {
			return e
		}
	}
	return lifecycle.StatusEdge{Status: status}
}

// ParseStatus case-insensitively canonicalizes a raw on-disk seed status
// value (e.g. the frontmatter `status: queued` this kind's scaffold writes
// lowercase) to its title-case Status, for read-only query verbs
// (`sidekick transitions <slug>`). Unlike ParseSeedTarget, this recognizes
// Queued too — it is not restricted to terminal --to targets. An
// unrecognized value is returned verbatim (trimmed) as a lifecycle.Status,
// so a caller can still report it as-is rather than silently substituting.
func ParseStatus(raw string) lifecycle.Status {
	trimmed := strings.TrimSpace(raw)
	for _, e := range BidirectionalMatrix() {
		if strings.EqualFold(string(e.Status), trimmed) {
			return e.Status
		}
	}
	return lifecycle.Status(trimmed)
}
