// Package resolve_test contains table-driven unit tests for the Resolve
// function per REQ: resolve-verb and REQ: resolve-ambiguous-and-unknown.
package resolve_test

import (
	"testing"

	"github.com/specscore/specscore-cli/internal/studio/fact"
	"github.com/specscore/specscore-cli/internal/studio/resolve"
)

// aliasedFact returns a declared aliased-as fact: (subject, aliased-as, object).
func aliasedFact(subject, alias string) fact.Fact {
	return fact.Fact{
		Subject:   subject,
		Predicate: "aliased-as",
		Object:    alias,
		Evidence:  fact.Evidence{Class: fact.Declared, Pointer: "ecosystem.yaml"},
		Adapter:   fact.Adapter{ID: "registries", Version: "0.1.0"},
	}
}

// plainFact returns a declared fact with an arbitrary predicate.
func plainFact(subject, predicate, object string) fact.Fact {
	return fact.Fact{
		Subject:   subject,
		Predicate: predicate,
		Object:    object,
		Evidence:  fact.Evidence{Class: fact.Declared, Pointer: "registry.json"},
		Adapter:   fact.Adapter{ID: "registries", Version: "0.1.0"},
	}
}

// --- unique resolution ---

// AC: resolve-alias-to-canonical — a brand name matching an aliased-as object
// resolves to the canonical entity id (the subject).
func TestResolve_UniqueViaAlias(t *testing.T) {
	facts := []fact.Fact{
		aliasedFact("sizeus", "SizeChart"),
	}
	r := resolve.Resolve(facts, "SizeChart")
	if r.Kind != resolve.Unique {
		t.Fatalf("kind = %v, want Unique", r.Kind)
	}
	if r.ID != "sizeus" {
		t.Errorf("ID = %q, want sizeus", r.ID)
	}
	if r.Citation == (fact.Fact{}) {
		t.Error("Citation is empty, want the aliased-as fact")
	}
	if r.Citation.Predicate != "aliased-as" {
		t.Errorf("Citation.Predicate = %q, want aliased-as", r.Citation.Predicate)
	}
}

// AC: resolve-alias-to-canonical (case-insensitive) — different case of the
// same name resolves identically.
func TestResolve_UniqueAliasIsCaseInsensitive(t *testing.T) {
	facts := []fact.Fact{
		aliasedFact("sizeus", "SizeChart"),
	}
	r := resolve.Resolve(facts, "sizechart")
	if r.Kind != resolve.Unique {
		t.Fatalf("kind = %v, want Unique", r.Kind)
	}
	if r.ID != "sizeus" {
		t.Errorf("ID = %q, want sizeus", r.ID)
	}
}

// Resolve returns the entity itself when the name matches an entity id that
// appears as a fact subject.
func TestResolve_UniqueViaEntityIDSubject(t *testing.T) {
	facts := []fact.Fact{
		plainFact("sizeus", "member-of", "payments"),
	}
	r := resolve.Resolve(facts, "sizeus")
	if r.Kind != resolve.Unique {
		t.Fatalf("kind = %v, want Unique", r.Kind)
	}
	if r.ID != "sizeus" {
		t.Errorf("ID = %q, want sizeus", r.ID)
	}
}

// Resolve returns the entity when the name matches an entity id that appears
// as a fact object.
func TestResolve_UniqueViaEntityIDObject(t *testing.T) {
	facts := []fact.Fact{
		plainFact("acme.app", "fronts", "acme"),
	}
	r := resolve.Resolve(facts, "acme")
	if r.Kind != resolve.Unique {
		t.Fatalf("kind = %v, want Unique", r.Kind)
	}
	if r.ID != "acme" {
		t.Errorf("ID = %q, want acme", r.ID)
	}
}

// Entity-id matching is case-insensitive.
func TestResolve_EntityIDIsCaseInsensitive(t *testing.T) {
	facts := []fact.Fact{
		plainFact("SIZEUS", "member-of", "payments"),
	}
	r := resolve.Resolve(facts, "sizeus")
	if r.Kind != resolve.Unique {
		t.Fatalf("kind = %v, want Unique", r.Kind)
	}
	if r.ID != "SIZEUS" {
		t.Errorf("ID = %q, want SIZEUS (the canonical form from the store)", r.ID)
	}
}

// When a name matches both an alias target and its own entity id in the store
// (the same canonical id), it still resolves uniquely to that id.
func TestResolve_AliasAndEntityIDSameResult(t *testing.T) {
	facts := []fact.Fact{
		aliasedFact("sizeus", "sizeus"), // aliased-as itself (degenerate but valid)
		plainFact("sizeus", "member-of", "payments"),
	}
	r := resolve.Resolve(facts, "sizeus")
	if r.Kind != resolve.Unique {
		t.Fatalf("kind = %v, want Unique", r.Kind)
	}
	if r.ID != "sizeus" {
		t.Errorf("ID = %q, want sizeus", r.ID)
	}
}

// --- ambiguous resolution ---

// AC: resolve-ambiguous-lists-candidates — a name matching two different
// canonical ids yields Ambiguous with both candidates listed.
func TestResolve_AmbiguousAlias(t *testing.T) {
	facts := []fact.Fact{
		aliasedFact("product-a", "shared"),
		aliasedFact("product-b", "shared"),
	}
	r := resolve.Resolve(facts, "shared")
	if r.Kind != resolve.Ambiguous {
		t.Fatalf("kind = %v, want Ambiguous", r.Kind)
	}
	if len(r.Candidates) != 2 {
		t.Fatalf("len(Candidates) = %d, want 2", len(r.Candidates))
	}
	ids := map[string]bool{r.Candidates[0].ID: true, r.Candidates[1].ID: true}
	if !ids["product-a"] || !ids["product-b"] {
		t.Errorf("Candidates %+v do not contain both product-a and product-b", r.Candidates)
	}
	// Each candidate carries a citation.
	for _, c := range r.Candidates {
		if c.Citation == (fact.Fact{}) {
			t.Errorf("candidate %q has empty citation", c.ID)
		}
	}
}

// Ambiguous resolution via entity ids when two different ids share a case-
// insensitive match is still flagged.
func TestResolve_AmbiguousEntityIDAndAlias(t *testing.T) {
	// "Shared" is both an alias for product-a AND appears as an entity id
	// in the store as subject "shared" (different case).
	facts := []fact.Fact{
		aliasedFact("product-a", "shared"),
		plainFact("shared", "member-of", "platform"),
	}
	r := resolve.Resolve(facts, "shared")
	if r.Kind != resolve.Ambiguous {
		t.Fatalf("kind = %v, want Ambiguous (alias → product-a; entity id → shared)", r.Kind)
	}
	if len(r.Candidates) != 2 {
		t.Fatalf("len(Candidates) = %d, want 2", len(r.Candidates))
	}
}

// --- not found ---

// AC: resolve-unknown-guides — a name with no matching alias or entity id
// returns NotFound.
func TestResolve_NotFound(t *testing.T) {
	facts := []fact.Fact{
		plainFact("sizeus", "member-of", "payments"),
	}
	r := resolve.Resolve(facts, "nonexistent")
	if r.Kind != resolve.NotFound {
		t.Fatalf("kind = %v, want NotFound", r.Kind)
	}
	if r.ID != "" {
		t.Errorf("ID = %q, want empty for NotFound", r.ID)
	}
	if len(r.Candidates) != 0 {
		t.Errorf("Candidates = %v, want empty for NotFound", r.Candidates)
	}
}

// An empty fact slice yields NotFound.
func TestResolve_EmptyStoreNotFound(t *testing.T) {
	r := resolve.Resolve(nil, "anything")
	if r.Kind != resolve.NotFound {
		t.Fatalf("kind = %v, want NotFound", r.Kind)
	}
}

// --- determinism ---

// Multiple calls with the same input return the same candidate order.
func TestResolve_CandidatesDeterministic(t *testing.T) {
	facts := []fact.Fact{
		aliasedFact("z-product", "shared"),
		aliasedFact("a-product", "shared"),
	}
	r1 := resolve.Resolve(facts, "shared")
	r2 := resolve.Resolve(facts, "shared")
	if r1.Kind != resolve.Ambiguous || r2.Kind != resolve.Ambiguous {
		t.Fatalf("both must be Ambiguous")
	}
	for i := range r1.Candidates {
		if r1.Candidates[i].ID != r2.Candidates[i].ID {
			t.Errorf("candidate[%d]: %q vs %q — not deterministic", i, r1.Candidates[i].ID, r2.Candidates[i].ID)
		}
	}
}
