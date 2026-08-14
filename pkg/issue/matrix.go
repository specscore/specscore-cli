package issue

// Package-local bidirectional matrix view.
//
// Issue is not a lifecycle.Kind — its tiny matrix and its frontmatter
// `status:` convention (not the `**Status:**` line pkg/lifecycle owns) live
// entirely in this package (see transitions.go). This file derives a
// previous/next view of that SAME matrix (LegalTransitions) that
// IsLegalTransition already consults, so `specscore issue transitions` never
// hand-maintains a second table that could drift from it.

import (
	"fmt"
	"sort"
	"strings"
)

// StatusEdge mirrors lifecycle.StatusEdge's shape for the Issue kind's
// plain-string statuses.
type StatusEdge struct {
	Status   string   `json:"status" yaml:"status"`
	Previous []string `json:"previous" yaml:"previous"`
	Next     []string `json:"next" yaml:"next"`
}

// BidirectionalMatrix returns a StatusEdge for every status appearing in
// LegalTransitions (as either a source or a target), sorted alphabetically.
func BidirectionalMatrix() []StatusEdge {
	forward := LegalTransitions()

	seen := map[string]struct{}{}
	for from, tos := range forward {
		seen[from] = struct{}{}
		for _, to := range tos {
			seen[to] = struct{}{}
		}
	}
	statuses := make([]string, 0, len(seen))
	for s := range seen {
		statuses = append(statuses, s)
	}
	sort.Strings(statuses)

	out := make([]StatusEdge, 0, len(statuses))
	for _, s := range statuses {
		var prev []string
		for from, tos := range forward {
			for _, to := range tos {
				if to == s {
					prev = append(prev, from)
				}
			}
		}
		sort.Strings(prev)
		next := append([]string{}, forward[s]...)
		sort.Strings(next)
		out = append(out, StatusEdge{Status: s, Previous: prev, Next: next})
	}
	return out
}

// EdgeFor returns the StatusEdge for a single status.
func EdgeFor(status string) StatusEdge {
	for _, e := range BidirectionalMatrix() {
		if e.Status == status {
			return e
		}
	}
	return StatusEdge{Status: status}
}

// RenderBidirectionalMatrix formats BidirectionalMatrix() as ANSI-free text,
// matching lifecycle.RenderBidirectionalMatrix's shape for the lifecycle.Kind
// verbs.
func RenderBidirectionalMatrix() string {
	edges := BidirectionalMatrix()
	var b strings.Builder
	fmt.Fprintf(&b, "Status vocabulary for issue (%d status(es)):\n", len(edges))
	for _, e := range edges {
		fmt.Fprintf(&b, "\n  %s\n", e.Status)
		prev := "(none — initial status, set only by `issue new`)"
		if len(e.Previous) > 0 {
			prev = strings.Join(e.Previous, ", ")
		}
		next := "(none — terminal status)"
		if len(e.Next) > 0 {
			next = strings.Join(e.Next, ", ")
		}
		fmt.Fprintf(&b, "    previous: %s\n", prev)
		fmt.Fprintf(&b, "    next:     %s\n", next)
	}
	return b.String()
}
