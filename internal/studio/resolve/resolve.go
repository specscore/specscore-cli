// Package resolve maps a brand, domain, repo slug, package, or product name
// to its canonical entity id by querying the fact store facts in memory.
//
// Feature: cli/studio/answers (REQ: resolve-verb,
// REQ: resolve-ambiguous-and-unknown)
//
// Resolution is case-insensitive and draws candidates from two sources:
//
//   - aliased-as facts: the object of an (entity, aliased-as, <name>) fact
//     maps <name> back to the entity subject (the canonical id).
//   - entity ids present in the store as subjects or objects of any fact.
//
// Results are Unique (one canonical id + the citing fact), Ambiguous (two or
// more distinct canonical ids + their citations), or NotFound. The function is
// pure — no store, no network access — so it is directly consumed by both the
// `studio resolve` verb and the `studio ask` router (Task 4).
package resolve

import (
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/internal/studio/fact"
)

// Kind classifies the outcome of a Resolve call.
type Kind int

const (
	// Unique means exactly one canonical id was found; Result.ID and
	// Result.Citation are populated.
	Unique Kind = iota
	// Ambiguous means more than one distinct canonical id matched the name;
	// Result.Candidates is populated.
	Ambiguous
	// NotFound means no candidate canonical id matched the name.
	NotFound
)

// Candidate is one resolution candidate: a canonical entity id and the fact
// that cites it (either the aliased-as fact whose object matched, or the first
// fact in the store whose subject/object equals the entity id).
type Candidate struct {
	ID       string
	Citation fact.Fact
}

// Result is the outcome of a Resolve call.
type Result struct {
	Kind Kind
	// ID is the canonical entity id for Unique results.
	ID string
	// Citation is the citing fact for Unique results.
	Citation fact.Fact
	// Candidates is the set of matching candidates for Ambiguous results.
	Candidates []Candidate
}

// Resolve maps name to its canonical entity id(s) using the given facts.
// Matching is case-insensitive. The returned Result classifies the outcome and
// carries the citation(s) so callers can render evidence without additional
// store queries.
func Resolve(facts []fact.Fact, name string) Result {
	lower := strings.ToLower(name)

	// candidatesByID accumulates unique canonical id → first citation, in
	// insertion order (via the ids slice for determinism).
	candidatesByID := map[string]fact.Fact{}
	var ids []string

	addCandidate := func(id string, citation fact.Fact) {
		if _, seen := candidatesByID[id]; !seen {
			candidatesByID[id] = citation
			ids = append(ids, id)
		}
	}

	// Pass 1: aliased-as facts — object matches the query → the subject is the
	// canonical id (REQ: resolve-verb source (a)).
	for _, f := range facts {
		if f.Predicate == "aliased-as" && strings.ToLower(f.Object) == lower {
			addCandidate(f.Subject, f)
		}
	}

	// Pass 2: entity ids present as subjects or objects of any fact
	// (REQ: resolve-verb source (b)). We use the first fact that references
	// each id as the citation.
	//
	// For aliased-as facts the subject is already handled by Pass 1 (and will
	// be deduplicated by addCandidate); the object is the alias string itself
	// — not a canonical entity id — so we skip aliased-as objects here.
	//
	// NOTE: we iterate facts again to prefer alias facts as citations (Pass 1
	// runs first), and only add new ids in Pass 2.
	for _, f := range facts {
		if strings.ToLower(f.Subject) == lower {
			addCandidate(f.Subject, f)
		}
		// Skip the object of aliased-as facts: the alias string (the object)
		// is the lookup key, not a canonical entity id.
		if f.Predicate != "aliased-as" && strings.ToLower(f.Object) == lower {
			addCandidate(f.Object, f)
		}
	}

	switch len(ids) {
	case 0:
		return Result{Kind: NotFound}
	case 1:
		id := ids[0]
		return Result{Kind: Unique, ID: id, Citation: candidatesByID[id]}
	default:
		// Sort candidates deterministically by id so the output is stable
		// across runs regardless of the fact slice order.
		sort.Strings(ids)
		candidates := make([]Candidate, 0, len(ids))
		for _, id := range ids {
			candidates = append(candidates, Candidate{ID: id, Citation: candidatesByID[id]})
		}
		return Result{Kind: Ambiguous, Candidates: candidates}
	}
}
