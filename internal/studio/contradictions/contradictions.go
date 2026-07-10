// Package contradictions computes SpecScore Studio's contradiction detectors as
// pure functions over the facts a `studio index`/`studio probe` run wrote into
// the fact store. It reads no store and touches no network: every detector is a
// deterministic query over a `[]fact.Fact` slice, so the engine is entirely
// fixture-testable.
//
// Feature: cli/studio/answers (REQ: contradictions-verb,
// REQ: status-vs-behavior-drift, REQ: same-predicate-disagreement)
//
// Two detectors, partitioned by evidence class so the noise discipline holds:
//
//   - status-drift owns declared-vs-verified-behavior disagreement, in two
//     branches — a shipped-implying `has-status` joined by subject prefix to a
//     failing `has-verification-status` (lifecycle-vs-verification), and a
//     same-subject+predicate `declared` object differing from a
//     `verified-behavior` one (declared-vs-verified, e.g. a registry-declared
//     `serves-status` 200 against a probed `down`).
//   - naming-conflict owns declared-vs-declared disagreement: two `declared`
//     facts sharing subject+predicate but asserting different objects from
//     different evidence pointers (the `ext-<id>` vs `<id>-contract` class).
//
// verified-behavior-vs-verified-behavior differences are supersession — a
// changed probe result — and are flagged by neither detector.
package contradictions

import (
	"sort"

	"github.com/specscore/specscore-cli/internal/studio/fact"
)

// Detector identifies which detector flagged a contradiction item — the value
// carried on every item and used as the evidence pointer when an item is
// written back as a `contradicts` fact.
type Detector string

const (
	// StatusDrift is the status-vs-behavior-drift detector: a `declared` claim
	// that a `verified-behavior` observation contradicts.
	StatusDrift Detector = "status-drift"
	// NamingConflict is the same-predicate-disagreement detector: two `declared`
	// facts disagreeing on the same subject+predicate.
	NamingConflict Detector = "naming-conflict"
)

const (
	// hasStatusPredicate is the specscore adapter's spec-lifecycle predicate; its
	// subjects are spec refs (`<repo>#<feature-id>` / `<repo>#ideas/<slug>`).
	hasStatusPredicate = "has-status"
	// hasVerificationStatusPredicate is the rehearse adapter's per-AC predicate;
	// its subjects are AC refs (`<repo>#<feature-id>#ac:<id>`).
	hasVerificationStatusPredicate = "has-verification-status"
	// failObject is the verification status that overturns a shipped-implying
	// lifecycle status.
	failObject = "fail"
)

// shippedImplying is the set of `has-status` objects that assert an entity is
// done/live — the statuses a failing verification contradicts. Membership is the
// exact set the Feature names (REQ: status-vs-behavior-drift).
var shippedImplying = map[string]bool{
	"Approved":     true,
	"Stable":       true,
	"Implementing": true,
}

// Item is one detected contradiction: the two facts that disagree plus the
// detector that flagged them. Side A is always the `declared` fact and Side B
// the contradicting fact (a `verified-behavior` fact for status-drift, the
// other `declared` fact for naming-conflict), so callers render a stable
// "declared vs …" order.
type Item struct {
	Detector Detector
	A        fact.Fact
	B        fact.Fact
}

// Detect runs both detectors over the given facts and returns every
// contradiction item in a deterministic order (status-drift items first, then
// naming-conflict, each internally sorted). It is pure: no store, no network.
func Detect(facts []fact.Fact) []Item {
	var items []Item
	items = append(items, statusDrift(facts)...)
	items = append(items, namingConflict(facts)...)
	return items
}

// statusDrift computes the status-vs-behavior-drift detector's two branches:
// lifecycle-vs-verification (a shipped-implying has-status joined by subject
// prefix to a failing has-verification-status) and declared-vs-verified (a
// declared object differing from a verified-behavior one on the same
// subject+predicate). Only verified-behavior evidence can overturn a declared
// claim here — a store with no probe/rehearse facts yields no items.
func statusDrift(facts []fact.Fact) []Item {
	var items []Item
	items = append(items, lifecycleVsVerification(facts)...)
	items = append(items, declaredVsVerified(facts)...)
	return items
}

// lifecycleVsVerification pairs each shipped-implying `has-status` (declared)
// fact with every failing `has-verification-status` (verified-behavior) fact
// whose subject extends the spec ref by the AC-ref suffix scheme
// (`<spec-ref>#ac:<id>`). One flagged pair per (has-status, failing AC) join.
func lifecycleVsVerification(facts []fact.Fact) []Item {
	var statuses, fails []fact.Fact
	for _, f := range facts {
		switch {
		case f.Predicate == hasStatusPredicate && f.Class == fact.Declared && shippedImplying[f.Object]:
			statuses = append(statuses, f)
		case f.Predicate == hasVerificationStatusPredicate && f.Class == fact.VerifiedBehavior && f.Object == failObject:
			fails = append(fails, f)
		}
	}
	var items []Item
	for _, s := range statuses {
		for _, v := range fails {
			if joinsBySubjectPrefix(s.Subject, v.Subject) {
				items = append(items, Item{Detector: StatusDrift, A: s, B: v})
			}
		}
	}
	sortItems(items)
	return items
}

// joinsBySubjectPrefix reports whether an AC-ref subject extends a spec-ref
// subject by the rehearse adapter's `#ac:` suffix scheme — the join key for the
// lifecycle-vs-verification branch (a `has-status` on `<repo>#<feature-id>`
// pairs with a failing `has-verification-status` on `<repo>#<feature-id>#ac:<id>`).
func joinsBySubjectPrefix(specRef, acRef string) bool {
	prefix := specRef + "#ac:"
	return len(acRef) > len(prefix) && acRef[:len(prefix)] == prefix
}

// declaredVsVerified groups facts by (subject, predicate) and, for each group
// holding at least one `declared` and one `verified-behavior` fact with
// different objects, flags each such cross-class differing pair. A declared and
// a verified-behavior fact *agreeing* on the object is behavioral confirmation,
// never flagged.
func declaredVsVerified(facts []fact.Fact) []Item {
	var items []Item
	forEachGroup(facts, func(group []fact.Fact) {
		for _, a := range group {
			if a.Class != fact.Declared {
				continue
			}
			for _, b := range group {
				if b.Class == fact.VerifiedBehavior && b.Object != a.Object {
					items = append(items, Item{Detector: StatusDrift, A: a, B: b})
				}
			}
		}
	})
	sortItems(items)
	return items
}

// namingConflict flags two `declared` facts sharing subject+predicate but
// asserting different objects from different evidence pointers — the
// declared-vs-declared class. Facts agreeing on the object (the same claim from
// two sources) are never flagged; when N>2 declared sources disagree, one item
// per distinct unordered pair is emitted so every disagreement is citeable.
func namingConflict(facts []fact.Fact) []Item {
	var items []Item
	forEachGroup(facts, func(group []fact.Fact) {
		var declared []fact.Fact
		for _, f := range group {
			if f.Class == fact.Declared {
				declared = append(declared, f)
			}
		}
		for i := 0; i < len(declared); i++ {
			for j := i + 1; j < len(declared); j++ {
				a, b := declared[i], declared[j]
				if a.Object != b.Object && a.Pointer != b.Pointer {
					items = append(items, Item{Detector: NamingConflict, A: a, B: b})
				}
			}
		}
	})
	sortItems(items)
	return items
}

// groupKey is the (subject, predicate) grouping key shared by the
// declared-vs-verified and naming-conflict detectors.
type groupKey struct {
	subject, predicate string
}

// forEachGroup buckets facts by (subject, predicate) — preserving input order
// within each bucket — and invokes fn once per group, in deterministic
// (subject, predicate) order.
func forEachGroup(facts []fact.Fact, fn func(group []fact.Fact)) {
	groups := map[groupKey][]fact.Fact{}
	var order []groupKey
	for _, f := range facts {
		k := groupKey{f.Subject, f.Predicate}
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], f)
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].subject != order[j].subject {
			return order[i].subject < order[j].subject
		}
		return order[i].predicate < order[j].predicate
	})
	for _, k := range order {
		fn(groups[k])
	}
}

// sortItems orders items deterministically by their two sides' fact refs so the
// same fact set always yields the same item order.
func sortItems(items []Item) {
	sort.SliceStable(items, func(i, j int) bool {
		if ri, rj := factRef(items[i].A), factRef(items[j].A); ri != rj {
			return ri < rj
		}
		return factRef(items[i].B) < factRef(items[j].B)
	})
}

// factRef is a pointer-free triple key for a fact (`<subject>|<predicate>|<object>`),
// used to order items deterministically.
func factRef(f fact.Fact) string {
	return f.Subject + "|" + f.Predicate + "|" + f.Object
}
