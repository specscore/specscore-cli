package contradictions

import (
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/studio/fact"
)

// mk builds a fact with the given triple, class, and pointer; the timestamp
// fields are stamped so items carry a non-empty observed_at like real facts.
func mk(subject, predicate, object string, class fact.Class, pointer string) fact.Fact {
	return fact.Fact{
		Subject:    subject,
		Predicate:  predicate,
		Object:     object,
		Evidence:   fact.Evidence{Class: class, Pointer: pointer},
		Adapter:    fact.Adapter{ID: "test", Version: "0"},
		ObservedAt: "2026-07-10T00:00:00Z",
		VerifiedAt: "2026-07-10T00:00:00Z",
		Ecosystem:  "demo",
	}
}

// hasItem reports whether items contains an item with the given detector whose
// two sides (order-insensitive) are the given fact refs.
func hasItem(items []Item, det Detector, refA, refB string) bool {
	for _, it := range items {
		if it.Detector != det {
			continue
		}
		a, b := factRef(it.A), factRef(it.B)
		if (a == refA && b == refB) || (a == refB && b == refA) {
			return true
		}
	}
	return false
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name  string
		facts []fact.Fact
		// wantItems: each entry is {detector, refA, refB} that MUST be present.
		wantItems [][3]string
		// wantCount is the exact number of items expected.
		wantCount int
	}{
		{
			name: "status-drift lifecycle-vs-verification: Stable feature with a failing AC",
			facts: []fact.Fact{
				mk("repo#feat/x", "has-status", "Stable", fact.Declared, "spec/features/x/README.md"),
				mk("repo#feat/x#ac:y", "has-verification-status", "fail", fact.VerifiedBehavior, ".specscore/rehearse/latest.json"),
			},
			wantItems: [][3]string{
				{string(StatusDrift), "repo#feat/x|has-status|Stable", "repo#feat/x#ac:y|has-verification-status|fail"},
			},
			wantCount: 1,
		},
		{
			name: "status-drift declared-vs-verified: registry 200 vs probed down",
			facts: []fact.Fact{
				mk("dead.example", "serves-status", "200", fact.Declared, "domains.json"),
				mk("dead.example", "serves-status", "down", fact.VerifiedBehavior, "https://dead.example/"),
			},
			wantItems: [][3]string{
				{string(StatusDrift), "dead.example|serves-status|200", "dead.example|serves-status|down"},
			},
			wantCount: 1,
		},
		{
			name: "naming-conflict: two declared facts disagree on a single-valued predicate with different pointers",
			facts: []fact.Fact{
				mk("d.example", "fronts", "ext-foo", fact.Declared, "registry-a.yaml"),
				mk("d.example", "fronts", "foo-contract", fact.Declared, "registry-b.yaml"),
			},
			wantItems: [][3]string{
				{string(NamingConflict), "d.example|fronts|ext-foo", "d.example|fronts|foo-contract"},
			},
			wantCount: 1,
		},
		{
			name: "agreement not flagged: same triple, different pointers",
			facts: []fact.Fact{
				mk("d.example", "fronts", "foo", fact.Declared, "registry-a.yaml"),
				mk("d.example", "fronts", "foo", fact.Declared, "registry-b.yaml"),
			},
			wantCount: 0,
		},
		{
			name: "multi-valued predicate not flagged: two has-ac facts are two ACs, not a conflict",
			facts: []fact.Fact{
				mk("repo#feat/x", "has-ac", "first-ac", fact.Declared, "spec/features/x/README.md:10"),
				mk("repo#feat/x", "has-ac", "second-ac", fact.Declared, "spec/features/x/README.md:20"),
			},
			wantCount: 0,
		},
		{
			name: "multi-valued predicate not flagged: two implemented-by repos are fan-out, not a conflict",
			facts: []fact.Fact{
				mk("bookius", "implemented-by", "bookius", fact.Declared, "ecosystem.yaml"),
				mk("bookius", "implemented-by", "ext-bookius", fact.Declared, "ecosystem-ext.yaml"),
			},
			wantCount: 0,
		},
		{
			name: "multi-valued predicate not flagged: two contains children are structure, not a conflict",
			facts: []fact.Fact{
				mk("repo#feat", "contains", "repo#feat/a", fact.Declared, "spec/features/feat/README.md"),
				mk("repo#feat", "contains", "repo#feat/b", fact.Declared, "spec/features/feat/b/README.md"),
			},
			wantCount: 0,
		},
		{
			name: "declared agreeing with verified not flagged",
			facts: []fact.Fact{
				mk("live.example", "serves-status", "200", fact.Declared, "domains.json"),
				mk("live.example", "serves-status", "200", fact.VerifiedBehavior, "https://live.example/"),
			},
			wantCount: 0,
		},
		{
			name: "behavioral supersession not flagged: two verified with different objects",
			facts: []fact.Fact{
				mk("d", "serves-status", "200", fact.VerifiedBehavior, "https://d/"),
				mk("d", "serves-status", "down", fact.VerifiedBehavior, "https://d/"),
			},
			wantCount: 0,
		},
		{
			name: "naming-conflict same object not flagged even with different pointers is covered; different objects same pointer not flagged",
			facts: []fact.Fact{
				mk("d.example", "fronts", "a", fact.Declared, "same.yaml"),
				mk("d.example", "fronts", "b", fact.Declared, "same.yaml"),
			},
			wantCount: 0,
		},
		{
			name: "non-shipped status with failing AC not flagged",
			facts: []fact.Fact{
				mk("repo#feat/x", "has-status", "Draft", fact.Declared, "spec/features/x/README.md"),
				mk("repo#feat/x#ac:y", "has-verification-status", "fail", fact.VerifiedBehavior, ".specscore/rehearse/latest.json"),
			},
			wantCount: 0,
		},
		{
			name: "passing AC does not overturn a Stable status",
			facts: []fact.Fact{
				mk("repo#feat/x", "has-status", "Stable", fact.Declared, "spec/features/x/README.md"),
				mk("repo#feat/x#ac:y", "has-verification-status", "pass", fact.VerifiedBehavior, ".specscore/rehearse/latest.json"),
			},
			wantCount: 0,
		},
		{
			name: "failing AC on a different feature does not join by prefix",
			facts: []fact.Fact{
				mk("repo#feat/x", "has-status", "Stable", fact.Declared, "spec/features/x/README.md"),
				mk("repo#feat/other#ac:y", "has-verification-status", "fail", fact.VerifiedBehavior, ".specscore/rehearse/latest.json"),
			},
			wantCount: 0,
		},
		{
			name: "N-way naming-conflict emits one item per distinct pair",
			facts: []fact.Fact{
				mk("d.example", "fronts", "a", fact.Declared, "p1.yaml"),
				mk("d.example", "fronts", "b", fact.Declared, "p2.yaml"),
				mk("d.example", "fronts", "c", fact.Declared, "p3.yaml"),
			},
			wantItems: [][3]string{
				{string(NamingConflict), "d.example|fronts|a", "d.example|fronts|b"},
				{string(NamingConflict), "d.example|fronts|a", "d.example|fronts|c"},
				{string(NamingConflict), "d.example|fronts|b", "d.example|fronts|c"},
			},
			wantCount: 3,
		},
		{
			name:      "empty store yields no items",
			facts:     nil,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := Detect(tt.facts)
			if len(items) != tt.wantCount {
				t.Fatalf("got %d items, want %d: %+v", len(items), tt.wantCount, items)
			}
			for _, want := range tt.wantItems {
				if !hasItem(items, Detector(want[0]), want[1], want[2]) {
					t.Errorf("missing %s item {%s, %s}; got %+v", want[0], want[1], want[2], items)
				}
			}
		})
	}
}

// The A side of a status-drift item is always the declared fact and the B side
// the verified-behavior fact, regardless of input order — so a caller can rely
// on "declared vs verified" ordering.
func TestDetect_StatusDriftSideOrdering(t *testing.T) {
	// Verified fact listed first in the input.
	items := Detect([]fact.Fact{
		mk("dead.example", "serves-status", "down", fact.VerifiedBehavior, "https://dead.example/"),
		mk("dead.example", "serves-status", "200", fact.Declared, "domains.json"),
	})
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].A.Class != fact.Declared {
		t.Errorf("side A class = %q, want declared", items[0].A.Class)
	}
	if items[0].B.Class != fact.VerifiedBehavior {
		t.Errorf("side B class = %q, want verified-behavior", items[0].B.Class)
	}
}

// Two conflict groups sharing a subject but differing in predicate are ordered
// by predicate (the group-sort tiebreak) — the items stay deterministic.
func TestDetect_SameSubjectDifferentPredicateOrdering(t *testing.T) {
	facts := []fact.Fact{
		// predicate "serves-status" group (declared-vs-declared conflict)
		mk("subj", "serves-status", "200", fact.Declared, "p1.json"),
		mk("subj", "serves-status", "404", fact.Declared, "p2.json"),
		// predicate "fronts" group (declared-vs-declared conflict)
		mk("subj", "fronts", "a", fact.Declared, "p1.yaml"),
		mk("subj", "fronts", "b", fact.Declared, "p2.yaml"),
	}
	items := Detect(facts)
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2: %+v", len(items), items)
	}
	if items[0].A.Predicate != "fronts" || items[1].A.Predicate != "serves-status" {
		t.Errorf("groups not ordered by predicate: %q then %q",
			items[0].A.Predicate, items[1].A.Predicate)
	}
}

// Detect is deterministic: the same fact set (any input order) yields the same
// item order.
func TestDetect_DeterministicOrder(t *testing.T) {
	facts := []fact.Fact{
		mk("z.example", "serves-status", "200", fact.Declared, "domains.json"),
		mk("z.example", "serves-status", "down", fact.VerifiedBehavior, "https://z.example/"),
		mk("a.example", "serves-status", "200", fact.Declared, "domains.json"),
		mk("a.example", "serves-status", "down", fact.VerifiedBehavior, "https://a.example/"),
	}
	items := Detect(facts)
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	if items[0].A.Subject != "a.example" || items[1].A.Subject != "z.example" {
		t.Errorf("items not in deterministic subject order: %q then %q",
			items[0].A.Subject, items[1].A.Subject)
	}
}

// --- ToFacts (REQ: contradiction-facts) ---

// TestToFacts_ShapeAndCanonicalization verifies that ToFacts produces
// contradicts facts with the exact shape the Feature requires:
// subject = smaller fact-ref, object = larger fact-ref, predicate = "contradicts",
// class = "derived", pointer = detector id, adapter = "contradictions".
func TestToFacts_ShapeAndCanonicalization(t *testing.T) {
	a := mk("dead.example", "serves-status", "200", fact.Declared, "domains.json")
	b := mk("dead.example", "serves-status", "down", fact.VerifiedBehavior, "https://dead.example/")
	items := []Item{{Detector: StatusDrift, A: a, B: b}}

	ts := "2026-07-10T12:00:00Z"
	cliVer := "0.42.0"
	facts := ToFacts(items, ts, cliVer)
	if len(facts) != 1 {
		t.Fatalf("got %d facts, want 1", len(facts))
	}
	f := facts[0]

	// Predicate must be "contradicts".
	if f.Predicate != "contradicts" {
		t.Errorf("Predicate = %q, want contradicts", f.Predicate)
	}
	// Evidence class must be "derived".
	if f.Class != fact.Derived {
		t.Errorf("Class = %q, want derived", f.Class)
	}
	// Pointer must be the detector id.
	if f.Pointer != string(StatusDrift) {
		t.Errorf("Pointer = %q, want %q", f.Pointer, string(StatusDrift))
	}
	// Adapter id must be "contradictions".
	if f.Adapter.ID != "contradictions" {
		t.Errorf("Adapter.ID = %q, want contradictions", f.Adapter.ID)
	}
	// Adapter version must equal the CLI version.
	if f.Adapter.Version != cliVer {
		t.Errorf("Adapter.Version = %q, want %q", f.Adapter.Version, cliVer)
	}
	// observed_at and verified_at must equal the run time.
	if f.ObservedAt != ts || f.VerifiedAt != ts {
		t.Errorf("ObservedAt=%q VerifiedAt=%q, want both %q", f.ObservedAt, f.VerifiedAt, ts)
	}
	// Subject and Object are fact-refs; the smaller one must be the subject.
	refA := factRef(a)
	refB := factRef(b)
	wantSubj, wantObj := refA, refB
	if refA > refB {
		wantSubj, wantObj = refB, refA
	}
	if f.Subject != wantSubj {
		t.Errorf("Subject = %q, want smaller ref %q", f.Subject, wantSubj)
	}
	if f.Object != wantObj {
		t.Errorf("Object = %q, want larger ref %q", f.Object, wantObj)
	}
}

// TestToFacts_Idempotent verifies that ToFacts produces the same
// subject/object pair regardless of item A/B assignment order, confirming
// that the canonicalization (smaller-ref first) is stable.
func TestToFacts_Idempotent(t *testing.T) {
	a := mk("dead.example", "serves-status", "200", fact.Declared, "domains.json")
	b := mk("dead.example", "serves-status", "down", fact.VerifiedBehavior, "https://dead.example/")

	// Item with A=a, B=b
	f1 := ToFacts([]Item{{Detector: StatusDrift, A: a, B: b}}, "2026-07-10T00:00:00Z", "v1")
	// Item with A=b, B=a (reversed)
	f2 := ToFacts([]Item{{Detector: StatusDrift, A: b, B: a}}, "2026-07-10T00:00:00Z", "v1")

	if len(f1) != 1 || len(f2) != 1 {
		t.Fatalf("unexpected fact counts: %d, %d", len(f1), len(f2))
	}
	if f1[0].Subject != f2[0].Subject || f1[0].Object != f2[0].Object {
		t.Errorf("canonicalization differs by item order:\n  f1: subj=%q obj=%q\n  f2: subj=%q obj=%q",
			f1[0].Subject, f1[0].Object, f2[0].Subject, f2[0].Object)
	}
}

// TestToFacts_EmptyItems returns an empty slice (not nil) for empty input.
func TestToFacts_EmptyItems(t *testing.T) {
	facts := ToFacts(nil, "2026-07-10T00:00:00Z", "v1")
	if facts == nil {
		t.Error("ToFacts(nil, ...) returned nil, want empty slice")
	}
	if len(facts) != 0 {
		t.Errorf("got %d facts, want 0", len(facts))
	}
}

// --- FilterIgnored (REQ: suppression-ignore-list) ---

// TestFilterIgnored_SuppressesMatchingItem verifies that an item whose
// canonical identity appears in the ignore list is moved to the suppressed
// result (carrying its canonical identity, with an empty reason when the line
// has no inline comment) and removed from the active result.
func TestFilterIgnored_SuppressesMatchingItem(t *testing.T) {
	a := mk("dead.example", "serves-status", "200", fact.Declared, "domains.json")
	b := mk("dead.example", "serves-status", "down", fact.VerifiedBehavior, "https://dead.example/")
	item := Item{Detector: StatusDrift, A: a, B: b}

	refA, refB := canonicalRefs(item)
	identity := refA + "  " + refB

	active, suppressed := FilterIgnored([]Item{item}, strings.NewReader(identity+"\n"))
	if len(active) != 0 {
		t.Errorf("active = %d, want 0 (suppressed item must be removed)", len(active))
	}
	if len(suppressed) != 1 {
		t.Fatalf("suppressed = %d, want 1", len(suppressed))
	}
	if suppressed[0].Identity != identity {
		t.Errorf("suppressed identity = %q, want %q", suppressed[0].Identity, identity)
	}
	if suppressed[0].Reason != "" {
		t.Errorf("suppressed reason = %q, want empty (no inline comment)", suppressed[0].Reason)
	}
	if suppressed[0].Item.Detector != StatusDrift {
		t.Errorf("suppressed item detector = %q, want status-drift", suppressed[0].Item.Detector)
	}
}

// TestFilterIgnored_CommentsAndBlankLines confirms that #-prefixed comment
// lines and blank lines in the ignore file are skipped without error.
func TestFilterIgnored_CommentsAndBlankLines(t *testing.T) {
	a := mk("subj", "provides", "ext-foo", fact.Declared, "r-a.json")
	b := mk("subj", "provides", "foo-contract", fact.Declared, "r-b.json")
	item := Item{Detector: NamingConflict, A: a, B: b}

	refA, refB := canonicalRefs(item)
	ignoreFile := "# this is a comment\n\n" + refA + "  " + refB + "\n# another comment\n"

	active, suppressed := FilterIgnored([]Item{item}, strings.NewReader(ignoreFile))
	if len(active) != 0 {
		t.Errorf("active = %d, want 0", len(active))
	}
	if len(suppressed) != 1 {
		t.Errorf("suppressed = %d, want 1", len(suppressed))
	}
}

// TestFilterIgnored_NonMatchingItemStaysActive verifies that items not in the
// ignore list remain in the active slice.
func TestFilterIgnored_NonMatchingItemStaysActive(t *testing.T) {
	a := mk("subj", "provides", "ext-foo", fact.Declared, "r-a.json")
	b := mk("subj", "provides", "foo-contract", fact.Declared, "r-b.json")
	item := Item{Detector: NamingConflict, A: a, B: b}

	// ignore file names a different, unrelated pair
	ignoreFile := "other|pred|obj  other|pred|obj2\n"

	active, suppressed := FilterIgnored([]Item{item}, strings.NewReader(ignoreFile))
	if len(active) != 1 {
		t.Errorf("active = %d, want 1", len(active))
	}
	if len(suppressed) != 0 {
		t.Errorf("suppressed = %d, want 0", len(suppressed))
	}
}

// TestFilterIgnored_EmptyIgnoreFile returns all items as active.
func TestFilterIgnored_EmptyIgnoreFile(t *testing.T) {
	a := mk("subj", "provides", "ext-foo", fact.Declared, "r-a.json")
	b := mk("subj", "provides", "foo-contract", fact.Declared, "r-b.json")
	item := Item{Detector: NamingConflict, A: a, B: b}

	active, suppressed := FilterIgnored([]Item{item}, strings.NewReader(""))
	if len(active) != 1 {
		t.Errorf("active = %d, want 1", len(active))
	}
	if len(suppressed) != 0 {
		t.Errorf("suppressed = %d, want 0", len(suppressed))
	}
}

// TestFilterIgnored_NilReader treats nil reader as empty (no suppression).
func TestFilterIgnored_NilReader(t *testing.T) {
	a := mk("subj", "provides", "ext-foo", fact.Declared, "r-a.json")
	b := mk("subj", "provides", "foo-contract", fact.Declared, "r-b.json")
	item := Item{Detector: NamingConflict, A: a, B: b}

	active, suppressed := FilterIgnored([]Item{item}, nil)
	if len(active) != 1 {
		t.Errorf("active = %d, want 1", len(active))
	}
	if len(suppressed) != 0 {
		t.Errorf("suppressed = %d, want 0", len(suppressed))
	}
}

// TestCanonicalRefs_SmallestFirst verifies that canonicalRefs always returns
// the lexicographically smaller ref first.
func TestCanonicalRefs_SmallestFirst(t *testing.T) {
	a := mk("zzz", "provides", "ext-foo", fact.Declared, "r-a.json")
	b := mk("aaa", "provides", "foo-contract", fact.Declared, "r-b.json")
	item := Item{Detector: NamingConflict, A: a, B: b}

	refA, refB := canonicalRefs(item)
	if refA > refB {
		t.Errorf("canonicalRefs returned (%q, %q) — refA must be ≤ refB", refA, refB)
	}
}

// TestFilterIgnored_TrailingComment verifies that an inline "#" comment after
// the identity pair still matches the identity AND is carried onto the
// suppressed result as its reason (REQ: suppression-ignore-list — the
// "reason comment on the matching line").
func TestFilterIgnored_TrailingComment(t *testing.T) {
	a := mk("subj", "provides", "ext-foo", fact.Declared, "r-a.json")
	b := mk("subj", "provides", "foo-contract", fact.Declared, "r-b.json")
	item := Item{Detector: NamingConflict, A: a, B: b}

	refA, refB := canonicalRefs(item)
	identity := refA + "  " + refB
	// Line has a trailing inline comment after the identity pair.
	ignoreFile := identity + " # known issue, accepted\n"

	active, suppressed := FilterIgnored([]Item{item}, strings.NewReader(ignoreFile))
	if len(active) != 0 {
		t.Errorf("active = %d, want 0 (trailing comment must be stripped)", len(active))
	}
	if len(suppressed) != 1 {
		t.Fatalf("suppressed = %d, want 1", len(suppressed))
	}
	if suppressed[0].Identity != identity {
		t.Errorf("suppressed identity = %q, want %q", suppressed[0].Identity, identity)
	}
	if suppressed[0].Reason != "known issue, accepted" {
		t.Errorf("suppressed reason = %q, want %q", suppressed[0].Reason, "known issue, accepted")
	}
}
