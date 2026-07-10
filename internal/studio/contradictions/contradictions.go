// Package contradictions computes SpecScore Studio's contradiction detectors as
// pure functions over the facts a `studio index`/`studio probe` run wrote into
// the fact store. It reads no store and touches no network: every detector is a
// deterministic query over a `[]fact.Fact` slice, so the engine is entirely
// fixture-testable.
//
// Feature: cli/studio/answers (REQ: contradictions-verb,
// REQ: status-vs-behavior-drift, REQ: same-predicate-disagreement,
// REQ: contradiction-facts, REQ: suppression-ignore-list)
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
//     facts sharing a *single-valued* subject+predicate but asserting different
//     objects from different evidence pointers (the `ext-<id>` vs
//     `<id>-contract` class). Only predicates that are single-valued per
//     subject (singleValuedDeclared) participate — legitimately multi-valued
//     predicates (has-ac, contains, implemented-by, …) are excluded, or every
//     multi-AC feature would self-"conflict" (the 5,758-item Sneat dogfood
//     flood, 92% has-ac).
//
// verified-behavior-vs-verified-behavior differences are supersession — a
// changed probe result — and are flagged by neither detector.
package contradictions

import (
	"bufio"
	"io"
	"sort"
	"strings"

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

// singleValuedDeclared is the fixed set of declared predicates that are
// single-valued per subject — the only predicates the naming-conflict detector
// fires on (REQ: same-predicate-disagreement): a subject has one lifecycle
// status, a domain serves one status, a domain fronts one product. Two declared
// facts disagreeing on one of these is a real conflict between authored
// sources. Every other declared predicate (has-ac, contains, implemented-by,
// aliased-as, member-of, consumes, …) is legitimately multi-valued — two
// different objects are two valid values, not a disagreement. Evidence: the
// unscoped detector flooded the Sneat dogfood run with 5,758 items (5,328
// has-ac + 406 contains + 24 implemented-by), 92% of them the has-ac
// self-"conflicts" of ordinary multi-AC features.
var singleValuedDeclared = map[string]bool{
	"has-status":    true,
	"serves-status": true,
	"fronts":        true,
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

// namingConflict flags two `declared` facts sharing a single-valued
// subject+predicate but asserting different objects from different evidence
// pointers — the declared-vs-declared class. Only the singleValuedDeclared
// predicates participate: a multi-valued predicate's differing objects are two
// valid values, not a disagreement (REQ: same-predicate-disagreement). Facts
// agreeing on the object (the same claim from two sources) are never flagged;
// when N>2 declared sources disagree, one item per distinct unordered pair is
// emitted so every disagreement is citeable.
func namingConflict(facts []fact.Fact) []Item {
	var items []Item
	forEachGroup(facts, func(group []fact.Fact) {
		if !singleValuedDeclared[group[0].Predicate] {
			return
		}
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

// canonicalRefs returns the two side refs of an item in lexicographically
// sorted order (smaller first), providing the stable canonical identity used
// for both the `contradicts` fact's subject/object and the ignore-list key.
func canonicalRefs(it Item) (refA, refB string) {
	a, b := factRef(it.A), factRef(it.B)
	if a <= b {
		return a, b
	}
	return b, a
}

// ToFacts converts detected contradiction items into `contradicts` facts ready
// to be merged into the store via store.Merge (REQ: contradiction-facts). Each
// fact encodes the canonical conflict identity as its subject/object triple
// (the lexicographically smaller fact-ref first), carries evidence class
// `derived` (computed from other facts, not observed), and uses the detector
// id as the evidence pointer. The adapter id is "contradictions"; the adapter
// version is the CLI version string passed by the caller. Both observed_at and
// verified_at are set to the run timestamp runAt so re-runs refresh
// verified_at idempotently (the merge key is subject+predicate+object+class+adapter,
// so the same conflict yields one stable row and never duplicates).
func ToFacts(items []Item, runAt, cliVersion string) []fact.Fact {
	out := make([]fact.Fact, 0, len(items))
	for _, it := range items {
		subj, obj := canonicalRefs(it)
		out = append(out, fact.Fact{
			Subject:   subj,
			Predicate: "contradicts",
			Object:    obj,
			Evidence: fact.Evidence{
				Class:   fact.Derived,
				Pointer: string(it.Detector),
			},
			Adapter: fact.Adapter{
				ID:      "contradictions",
				Version: cliVersion,
			},
			ObservedAt: runAt,
			VerifiedAt: runAt,
		})
	}
	return out
}

// Suppressed is one contradiction item suppressed by an ignore-list entry
// (REQ: suppression-ignore-list): the item itself, the canonical
// "<side-a>  <side-b>" identity string that matched (the exact form the
// ignore file uses), and the reason comment from the matching line's inline
// "#" comment, if any — so `--show-ignored` can report what was suppressed
// and why, and suppression is never silent.
type Suppressed struct {
	Item Item
	// Identity is the canonical "<side-a-ref>  <side-b-ref>" pair (the two
	// smaller-first refs joined by two spaces), exactly the form the ignore
	// file uses per line.
	Identity string
	// Reason is the trailing inline comment (the text after " #") on the
	// matching ignore-file line, trimmed; empty when the line carried none.
	Reason string
}

// FilterIgnored partitions items into active (not suppressed) and suppressed
// (matched by an ignore-list entry) results (REQ: suppression-ignore-list).
// The reader supplies the ignore file's content: one canonical
// "<refA>  <refB>" pair per line (two spaces between refs), with #-prefixed
// comments and blank lines skipped, and an optional inline " # reason"
// comment after the pair whose text is carried onto the suppressed result.
// A nil reader is treated as an empty ignore file — all items are returned as
// active. The canonical pair is the same smaller-ref-first identity ToFacts
// uses as the contradicts fact's subject/object, so operators copy from fact
// queries into the ignore file.
func FilterIgnored(items []Item, r io.Reader) (active []Item, suppressed []Suppressed) {
	ignored := parseIgnoreList(r)
	for _, it := range items {
		a, b := canonicalRefs(it)
		key := a + "  " + b
		if reason, ok := ignored[key]; ok {
			suppressed = append(suppressed, Suppressed{Item: it, Identity: key, Reason: reason})
		} else {
			active = append(active, it)
		}
	}
	return active, suppressed
}

// parseIgnoreList reads the ignore-list reader and returns the canonical
// "<refA>  <refB>" identities it contains, each mapped to the trailing inline
// comment on its line ("" when none). Lines starting with "#" and blank lines
// are skipped. A nil reader returns an empty set.
func parseIgnoreList(r io.Reader) map[string]string {
	if r == nil {
		return nil
	}
	set := make(map[string]string)
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Split off any trailing inline comment: the text after " #" is the
		// suppression reason carried onto the suppressed item.
		reason := ""
		if idx := strings.Index(line, " #"); idx >= 0 {
			reason = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line[idx:]), "#"))
			line = strings.TrimSpace(line[:idx])
		}
		set[line] = reason
	}
	return set
}
