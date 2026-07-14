// Package ask_test contains table-driven unit tests for the deterministic
// question router and its 13 templates.
//
// Feature: cli/studio/answers (REQ: ask-verb-and-router, REQ: ask-citations,
// REQ: ask-unroutable)
package ask_test

import (
	"testing"

	"github.com/specscore/specscore-cli/internal/studio/ask"
	"github.com/specscore/specscore-cli/internal/studio/fact"
)

// --- fact builders ---

func declared(subject, predicate, object, pointer string) fact.Fact {
	return fact.Fact{
		Subject:    subject,
		Predicate:  predicate,
		Object:     object,
		Evidence:   fact.Evidence{Class: fact.Declared, Pointer: pointer},
		Adapter:    fact.Adapter{ID: "registries", Version: "0.1.0"},
		ObservedAt: "2026-07-10T00:00:00Z",
		VerifiedAt: "2026-07-10T00:00:00Z",
		Ecosystem:  "demo",
	}
}

func verified(subject, predicate, object, pointer string) fact.Fact {
	return fact.Fact{
		Subject:    subject,
		Predicate:  predicate,
		Object:     object,
		Evidence:   fact.Evidence{Class: fact.VerifiedBehavior, Pointer: pointer},
		Adapter:    fact.Adapter{ID: "probe-domain", Version: "0.1.0"},
		ObservedAt: "2026-07-10T01:00:00Z",
		VerifiedAt: "2026-07-10T01:00:00Z",
		Ecosystem:  "demo",
	}
}

func derived(subject, predicate, object, pointer string) fact.Fact {
	return fact.Fact{
		Subject:    subject,
		Predicate:  predicate,
		Object:     object,
		Evidence:   fact.Evidence{Class: fact.Derived, Pointer: pointer},
		Adapter:    fact.Adapter{ID: "manifests", Version: "0.1.0"},
		ObservedAt: "2026-07-10T00:00:00Z",
		VerifiedAt: "2026-07-10T00:00:00Z",
		Ecosystem:  "demo",
	}
}

// runTemplate routes the question, resolves the parameter through resolve
// semantics when the template targets an entity, and runs the template's query.
// It returns the template id, the answer, and whether the query produced a
// non-empty (cited) answer — the exact path the ask verb drives.
func runTemplate(t *testing.T, facts []fact.Fact, question string) (id string, ans ask.Answer, routed, answered bool) {
	t.Helper()
	tmpl, param, ok := ask.Route(question)
	if !ok {
		return "", ask.Answer{}, false, false
	}
	if tmpl.TargetsEntity {
		if r := ask.Resolve(facts, param); r.Kind == 0 { // resolve.Unique == 0
			param = r.ID
		}
	}
	a, ok := tmpl.Query(facts, param)
	return tmpl.ID, a, true, ok
}

// --- registry ---

// AC: ask-unroutable-lists-templates (the same content as --list) — the router
// exposes exactly the 13 documented templates, each with a stable id and at
// least one human-readable form.
func TestTemplates_ThirteenWithIDsAndForms(t *testing.T) {
	tmpls := ask.Templates()
	if len(tmpls) != 13 {
		t.Fatalf("template count = %d, want 13", len(tmpls))
	}
	want := map[string]bool{
		"who-fronts": true, "what-repos-implement": true, "status-of": true,
		"aliases-of": true, "member-of": true, "is-it-live": true,
		"ci-status-of": true, "what-verifies": true, "contradictions-for": true,
		"freshness-of": true, "what-uses": true, "version-pins": true,
		"aliases-resolve": true,
	}
	seen := map[string]bool{}
	for _, tm := range tmpls {
		if !want[tm.ID] {
			t.Errorf("unexpected template id %q", tm.ID)
		}
		if seen[tm.ID] {
			t.Errorf("duplicate template id %q", tm.ID)
		}
		seen[tm.ID] = true
		if len(tm.Forms) == 0 {
			t.Errorf("template %q has no forms", tm.ID)
		}
	}
	for id := range want {
		if !seen[id] {
			t.Errorf("missing template id %q", id)
		}
	}
}

// --- who-fronts ---

func TestWhoFronts(t *testing.T) {
	facts := []fact.Fact{
		declared("noise", "has-status", "draft", "products.json"),
		declared("acme.app", "fronts", "acme", "domains.json"),
	}
	tests := []struct {
		name      string
		question  string
		wantRoute bool
		wantAns   bool
		wantID    string
	}{
		{"object side (X = worker)", "who fronts acme", true, true, "who-fronts"},
		{"subject side (X = domain)", "who fronts acme.app", true, true, "who-fronts"},
		{"what fronts", "what fronts acme", true, true, "who-fronts"},
		{"empty", "who fronts nobody", true, false, "who-fronts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, ans, routed, answered := runTemplate(t, facts, tt.question)
			if routed != tt.wantRoute {
				t.Fatalf("routed = %v, want %v", routed, tt.wantRoute)
			}
			if id != tt.wantID {
				t.Errorf("id = %q, want %q", id, tt.wantID)
			}
			if answered != tt.wantAns {
				t.Fatalf("answered = %v, want %v", answered, tt.wantAns)
			}
			if answered {
				if len(ans.Citations) == 0 {
					t.Error("answer has no citations")
				}
				if ans.Citations[0].Predicate != "fronts" {
					t.Errorf("citation predicate = %q, want fronts", ans.Citations[0].Predicate)
				}
			}
		})
	}
}

// --- what-repos-implement ---

func TestWhatReposImplement(t *testing.T) {
	facts := []fact.Fact{
		declared("trackus", "implemented-by", "anymeter-repo", "ecosystem.yaml"),
		declared("trackus", "implemented-by", "trackus-ext", "ecosystem.yaml"),
	}
	t.Run("happy", func(t *testing.T) {
		id, ans, _, answered := runTemplate(t, facts, "what repos implement trackus")
		if id != "what-repos-implement" || !answered {
			t.Fatalf("id=%q answered=%v", id, answered)
		}
		if len(ans.Citations) != 2 {
			t.Errorf("citations = %d, want 2", len(ans.Citations))
		}
	})
	t.Run("alt trigger", func(t *testing.T) {
		id, _, _, answered := runTemplate(t, facts, "which repo implements trackus")
		if id != "what-repos-implement" || !answered {
			t.Fatalf("id=%q answered=%v", id, answered)
		}
	})
	t.Run("empty", func(t *testing.T) {
		_, _, routed, answered := runTemplate(t, facts, "what repos implement ghost")
		if !routed || answered {
			t.Fatalf("routed=%v answered=%v, want routed=true answered=false", routed, answered)
		}
	})
}

// --- status-of ---

func TestStatusOf(t *testing.T) {
	facts := []fact.Fact{declared("repo#feat/x", "has-status", "Stable", "spec/features/x/README.md")}
	t.Run("status of", func(t *testing.T) {
		id, ans, _, answered := runTemplate(t, facts, "status of repo#feat/x")
		if id != "status-of" || !answered {
			t.Fatalf("id=%q answered=%v", id, answered)
		}
		if ans.Citations[0].Object != "Stable" {
			t.Errorf("object = %q, want Stable", ans.Citations[0].Object)
		}
	})
	t.Run("is X approved", func(t *testing.T) {
		id, _, _, answered := runTemplate(t, facts, "is repo#feat/x approved")
		if id != "status-of" || !answered {
			t.Fatalf("id=%q answered=%v", id, answered)
		}
	})
	t.Run("is X stable", func(t *testing.T) {
		id, _, _, answered := runTemplate(t, facts, "is repo#feat/x stable")
		if id != "status-of" || !answered {
			t.Fatalf("id=%q answered=%v", id, answered)
		}
	})
	t.Run("empty", func(t *testing.T) {
		_, _, routed, answered := runTemplate(t, facts, "status of ghost")
		if !routed || answered {
			t.Fatalf("routed=%v answered=%v", routed, answered)
		}
	})
}

// --- aliases-of ---

func TestAliasesOf(t *testing.T) {
	facts := []fact.Fact{declared("sizeus", "aliased-as", "SizeChart", "ecosystem.yaml")}
	t.Run("aliases of", func(t *testing.T) {
		id, ans, _, answered := runTemplate(t, facts, "aliases of sizeus")
		if id != "aliases-of" || !answered {
			t.Fatalf("id=%q answered=%v", id, answered)
		}
		if ans.Citations[0].Object != "SizeChart" {
			t.Errorf("object = %q, want SizeChart", ans.Citations[0].Object)
		}
	})
	t.Run("what is X called", func(t *testing.T) {
		id, _, _, answered := runTemplate(t, facts, "what is sizeus called")
		if id != "aliases-of" || !answered {
			t.Fatalf("id=%q answered=%v", id, answered)
		}
	})
	t.Run("empty", func(t *testing.T) {
		_, _, routed, answered := runTemplate(t, facts, "aliases of ghost")
		if !routed || answered {
			t.Fatalf("routed=%v answered=%v", routed, answered)
		}
	})
}

// --- member-of ---

func TestMemberOf(t *testing.T) {
	facts := []fact.Fact{declared("sizeus", "member-of", "payments", "ecosystem.yaml")}
	t.Run("what vertical is X in", func(t *testing.T) {
		id, ans, _, answered := runTemplate(t, facts, "what vertical is sizeus in")
		if id != "member-of" || !answered {
			t.Fatalf("id=%q answered=%v", id, answered)
		}
		if ans.Citations[0].Object != "payments" {
			t.Errorf("object = %q, want payments", ans.Citations[0].Object)
		}
	})
	t.Run("member of X", func(t *testing.T) {
		id, _, _, answered := runTemplate(t, facts, "member of sizeus")
		if id != "member-of" || !answered {
			t.Fatalf("id=%q answered=%v", id, answered)
		}
	})
	t.Run("empty", func(t *testing.T) {
		_, _, routed, answered := runTemplate(t, facts, "member of ghost")
		if !routed || answered {
			t.Fatalf("routed=%v answered=%v", routed, answered)
		}
	})
}

// --- is-it-live (fronts→serves-status hop) ---

func TestIsItLive(t *testing.T) {
	facts := []fact.Fact{
		declared("dead.example", "fronts", "acme", "domains.json"),
		verified("dead.example", "serves-status", "down", "https://dead.example/"),
	}
	t.Run("is X live via fronts hop", func(t *testing.T) {
		id, ans, _, answered := runTemplate(t, facts, "is acme live")
		if id != "is-it-live" || !answered {
			t.Fatalf("id=%q answered=%v", id, answered)
		}
		// Cites both the fronts fact and the probed serves-status fact.
		var haveFronts, haveServes bool
		for _, c := range ans.Citations {
			if c.Predicate == "fronts" {
				haveFronts = true
			}
			if c.Predicate == "serves-status" && c.Class == "verified-behavior" {
				haveServes = true
			}
		}
		if !haveFronts || !haveServes {
			t.Errorf("citations miss the two-step hop: %+v", ans.Citations)
		}
	})
	t.Run("does X serve via direct domain", func(t *testing.T) {
		id, _, _, answered := runTemplate(t, facts, "does dead.example serve")
		if id != "is-it-live" || !answered {
			t.Fatalf("id=%q answered=%v", id, answered)
		}
	})
	t.Run("empty — no probed serves-status", func(t *testing.T) {
		onlyFronts := []fact.Fact{declared("x.example", "fronts", "widget", "domains.json")}
		_, _, routed, answered := runTemplate(t, onlyFronts, "is widget live")
		if !routed || answered {
			t.Fatalf("routed=%v answered=%v, want routed=true answered=false", routed, answered)
		}
	})
}

// --- ci-status-of ---

func TestCIStatusOf(t *testing.T) {
	facts := []fact.Fact{verified("acme-repo", "ci-status", "pass", "gh:acme-repo")}
	t.Run("ci status of", func(t *testing.T) {
		id, ans, _, answered := runTemplate(t, facts, "ci status of acme-repo")
		if id != "ci-status-of" || !answered {
			t.Fatalf("id=%q answered=%v", id, answered)
		}
		if ans.Citations[0].Object != "pass" {
			t.Errorf("object = %q, want pass", ans.Citations[0].Object)
		}
	})
	t.Run("is X green", func(t *testing.T) {
		id, _, _, answered := runTemplate(t, facts, "is acme-repo green")
		if id != "ci-status-of" || !answered {
			t.Fatalf("id=%q answered=%v", id, answered)
		}
	})
	t.Run("empty", func(t *testing.T) {
		_, _, routed, answered := runTemplate(t, facts, "ci status of ghost")
		if !routed || answered {
			t.Fatalf("routed=%v answered=%v", routed, answered)
		}
	})
}

// --- what-verifies ---

func TestWhatVerifies(t *testing.T) {
	facts := []fact.Fact{
		verified("repo#x#ac:y", "verified-by", "spec/features/x/_tests/s.md", "report"),
		verified("repo#x#ac:y", "has-verification-status", "pass", "report"),
	}
	t.Run("what verifies (subject prefix)", func(t *testing.T) {
		id, ans, _, answered := runTemplate(t, facts, "what verifies repo#x")
		if id != "what-verifies" || !answered {
			t.Fatalf("id=%q answered=%v", id, answered)
		}
		if len(ans.Citations) != 2 {
			t.Errorf("citations = %d, want 2 (verified-by + has-verification-status)", len(ans.Citations))
		}
	})
	t.Run("is X tested", func(t *testing.T) {
		id, _, _, answered := runTemplate(t, facts, "is repo#x tested")
		if id != "what-verifies" || !answered {
			t.Fatalf("id=%q answered=%v", id, answered)
		}
	})
	t.Run("empty", func(t *testing.T) {
		_, _, routed, answered := runTemplate(t, facts, "what verifies ghost")
		if !routed || answered {
			t.Fatalf("routed=%v answered=%v", routed, answered)
		}
	})
}

// --- contradictions-for ---

func TestContradictionsFor(t *testing.T) {
	facts := []fact.Fact{
		derived("dead.example|serves-status|200", "contradicts",
			"dead.example|serves-status|down", "status-drift"),
	}
	t.Run("contradictions for (subject side)", func(t *testing.T) {
		id, ans, _, answered := runTemplate(t, facts, "contradictions for dead.example")
		if id != "contradictions-for" || !answered {
			t.Fatalf("id=%q answered=%v", id, answered)
		}
		if ans.Citations[0].Predicate != "contradicts" {
			t.Errorf("predicate = %q, want contradicts", ans.Citations[0].Predicate)
		}
	})
	t.Run("does X conflict", func(t *testing.T) {
		id, _, _, answered := runTemplate(t, facts, "does dead.example conflict")
		if id != "contradictions-for" || !answered {
			t.Fatalf("id=%q answered=%v", id, answered)
		}
	})
	t.Run("empty", func(t *testing.T) {
		_, _, routed, answered := runTemplate(t, facts, "contradictions for ghost")
		if !routed || answered {
			t.Fatalf("routed=%v answered=%v", routed, answered)
		}
	})
}

// --- freshness-of ---

func TestFreshnessOf(t *testing.T) {
	older := verified("acme.app", "serves-status", "200", "https://acme.app/")
	older.VerifiedAt = "2026-07-10T00:00:00Z"
	newer := verified("acme.app", "serves-status", "200", "https://acme.app/")
	newer.VerifiedAt = "2026-07-10T05:00:00Z"
	facts := []fact.Fact{older, newer}
	t.Run("how fresh is X — freshest wins", func(t *testing.T) {
		id, ans, _, answered := runTemplate(t, facts, "how fresh is acme.app")
		if id != "freshness-of" || !answered {
			t.Fatalf("id=%q answered=%v", id, answered)
		}
		if len(ans.Citations) != 1 {
			t.Fatalf("citations = %d, want 1 (only the freshest)", len(ans.Citations))
		}
	})
	t.Run("when was X verified", func(t *testing.T) {
		id, _, _, answered := runTemplate(t, facts, "when was acme.app verified")
		if id != "freshness-of" || !answered {
			t.Fatalf("id=%q answered=%v", id, answered)
		}
	})
	t.Run("empty", func(t *testing.T) {
		_, _, routed, answered := runTemplate(t, facts, "how fresh is ghost")
		if !routed || answered {
			t.Fatalf("routed=%v answered=%v", routed, answered)
		}
	})
}

// --- what-uses ---

func TestWhatUses(t *testing.T) {
	facts := []fact.Fact{
		derived("consumer-a", "consumes", "contactus@1.2.0", "go.mod"),
		derived("consumer-b", "consumes", "contactus@1.3.0", "go.mod"),
	}
	t.Run("what uses (object prefix)", func(t *testing.T) {
		id, ans, _, answered := runTemplate(t, facts, "what uses contactus")
		if id != "what-uses" || !answered {
			t.Fatalf("id=%q answered=%v", id, answered)
		}
		if len(ans.Citations) != 2 {
			t.Errorf("citations = %d, want 2 consumers", len(ans.Citations))
		}
	})
	t.Run("who consumes", func(t *testing.T) {
		id, _, _, answered := runTemplate(t, facts, "who consumes contactus")
		if id != "what-uses" || !answered {
			t.Fatalf("id=%q answered=%v", id, answered)
		}
	})
	t.Run("empty", func(t *testing.T) {
		_, _, routed, answered := runTemplate(t, facts, "what uses ghost")
		if !routed || answered {
			t.Fatalf("routed=%v answered=%v", routed, answered)
		}
	})
}

// --- version-pins ---

func TestVersionPins(t *testing.T) {
	facts := []fact.Fact{
		derived("consumer-a", "consumes", "platform@2.0.0", "go.mod"),
		derived("consumer-b", "consumes", "platform@2.1.0", "go.mod"),
	}
	t.Run("what version of", func(t *testing.T) {
		id, ans, _, answered := runTemplate(t, facts, "what version of platform")
		if id != "version-pins" || !answered {
			t.Fatalf("id=%q answered=%v", id, answered)
		}
		if len(ans.Citations) != 2 {
			t.Errorf("citations = %d, want 2 pins", len(ans.Citations))
		}
	})
	t.Run("which version pins", func(t *testing.T) {
		id, _, _, answered := runTemplate(t, facts, "which version pins platform")
		if id != "version-pins" || !answered {
			t.Fatalf("id=%q answered=%v", id, answered)
		}
	})
	t.Run("empty", func(t *testing.T) {
		_, _, routed, answered := runTemplate(t, facts, "what version of ghost")
		if !routed || answered {
			t.Fatalf("routed=%v answered=%v", routed, answered)
		}
	})
}

// --- aliases-resolve (bare id lookup) ---

func TestAliasesResolve(t *testing.T) {
	facts := []fact.Fact{declared("sizeus", "aliased-as", "SizeChart", "ecosystem.yaml")}
	t.Run("what is X", func(t *testing.T) {
		id, ans, _, answered := runTemplate(t, facts, "what is SizeChart")
		if id != "aliases-resolve" || !answered {
			t.Fatalf("id=%q answered=%v", id, answered)
		}
		if ans.Citations[0].Predicate != "aliased-as" {
			t.Errorf("predicate = %q, want aliased-as", ans.Citations[0].Predicate)
		}
	})
	t.Run("resolve X", func(t *testing.T) {
		id, _, _, answered := runTemplate(t, facts, "resolve SizeChart")
		if id != "aliases-resolve" || !answered {
			t.Fatalf("id=%q answered=%v", id, answered)
		}
	})
	t.Run("empty — unknown name", func(t *testing.T) {
		_, _, routed, answered := runTemplate(t, facts, "resolve ghost")
		if !routed || answered {
			t.Fatalf("routed=%v answered=%v", routed, answered)
		}
	})
	t.Run("empty — ambiguous declines to single answer", func(t *testing.T) {
		amb := []fact.Fact{
			declared("product-a", "aliased-as", "shared", "a.yaml"),
			declared("product-b", "aliased-as", "shared", "b.yaml"),
		}
		_, _, routed, answered := runTemplate(t, amb, "what is shared")
		if !routed || answered {
			t.Fatalf("routed=%v answered=%v, want routed=true answered=false for ambiguous", routed, answered)
		}
	})
}

// --- unroutable ---

// AC: ask-unroutable-lists-templates — a question matching no template does not
// route (the caller then exits 1 and lists the templates).
func TestRoute_Unroutable(t *testing.T) {
	for _, q := range []string{
		"why does contactus exist",
		"what secrets does this pipeline need",
		"how do i deploy sizeus",
		"",
		"who fronts",  // trigger with no parameter → not a match
		"who fronts ", // prefix present but empty tail → not a match
		"status of ?", // prefix present, tail collapses to "" after "?" strip
	} {
		if _, _, ok := ask.Route(q); ok {
			t.Errorf("question %q routed, want unroutable", q)
		}
	}
}

// The router lowercases the question so a mixed-case trigger still matches.
func TestRoute_CaseInsensitiveTrigger(t *testing.T) {
	tmpl, param, ok := ask.Route("WHO FRONTS Acme")
	if !ok {
		t.Fatal("mixed-case question did not route")
	}
	if tmpl.ID != "who-fronts" {
		t.Errorf("id = %q, want who-fronts", tmpl.ID)
	}
	if param != "acme" {
		t.Errorf("param = %q, want acme (lowercased)", param)
	}
}

// A trailing "?" is tolerated and stripped from the captured parameter.
func TestRoute_TrailingQuestionMark(t *testing.T) {
	_, param, ok := ask.Route("who fronts acme?")
	if !ok || param != "acme" {
		t.Fatalf("ok=%v param=%q, want ok=true param=acme", ok, param)
	}
}

// A suffix-gated trigger whose prefix matches but whose closing keyword is
// absent does not capture — "is acme running" shares the "is " prefix with the
// suffix-gated ci/status/tested triggers but ends in none of their keywords, so
// it stays unroutable (exercising the suffix-mismatch branch of capture).
func TestRoute_PrefixMatchesButSuffixDoesNot(t *testing.T) {
	if tmpl, _, ok := ask.Route("is acme running"); ok {
		t.Errorf("question routed to %q, want unroutable (no suffix keyword matched)", tmpl.ID)
	}
}
