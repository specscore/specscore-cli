package contradictions

import (
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
			name: "naming-conflict: two declared facts disagree with different pointers",
			facts: []fact.Fact{
				mk("subj", "provides", "ext-foo", fact.Declared, "registry-a.json"),
				mk("subj", "provides", "foo-contract", fact.Declared, "registry-b.json"),
			},
			wantItems: [][3]string{
				{string(NamingConflict), "subj|provides|ext-foo", "subj|provides|foo-contract"},
			},
			wantCount: 1,
		},
		{
			name: "agreement not flagged: same triple, different pointers",
			facts: []fact.Fact{
				mk("subj", "provides", "foo", fact.Declared, "registry-a.json"),
				mk("subj", "provides", "foo", fact.Declared, "registry-b.json"),
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
				mk("subj", "provides", "a", fact.Declared, "same.json"),
				mk("subj", "provides", "b", fact.Declared, "same.json"),
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
				mk("subj", "provides", "a", fact.Declared, "p1.json"),
				mk("subj", "provides", "b", fact.Declared, "p2.json"),
				mk("subj", "provides", "c", fact.Declared, "p3.json"),
			},
			wantItems: [][3]string{
				{string(NamingConflict), "subj|provides|a", "subj|provides|b"},
				{string(NamingConflict), "subj|provides|a", "subj|provides|c"},
				{string(NamingConflict), "subj|provides|b", "subj|provides|c"},
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
		// predicate "zeta" group (declared-vs-declared conflict)
		mk("subj", "zeta", "a", fact.Declared, "p1.json"),
		mk("subj", "zeta", "b", fact.Declared, "p2.json"),
		// predicate "alpha" group (declared-vs-declared conflict)
		mk("subj", "alpha", "a", fact.Declared, "p1.json"),
		mk("subj", "alpha", "b", fact.Declared, "p2.json"),
	}
	items := Detect(facts)
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2: %+v", len(items), items)
	}
	if items[0].A.Predicate != "alpha" || items[1].A.Predicate != "zeta" {
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
