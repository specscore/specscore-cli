package adapters

import (
	"testing"
	"time"

	"github.com/specscore/specscore-cli/internal/studio/fact"
)

// fakeAdapter emits a fixed set of facts and warnings for every repo.
type fakeAdapter struct {
	id       string
	version  string
	facts    []fact.Fact
	warnings []Warning
}

func (a fakeAdapter) ID() string      { return a.id }
func (a fakeAdapter) Version() string { return a.version }
func (a fakeAdapter) Ingest(string) ([]fact.Fact, []Warning) {
	// Return copies so central stamping never mutates the fixtures.
	return append([]fact.Fact(nil), a.facts...), append([]Warning(nil), a.warnings...)
}

func TestAll_RegistersBuiltinsWithUniqueIDs(t *testing.T) {
	all := All(Options{})
	if len(all) == 0 {
		t.Fatal("All() is empty")
	}
	seen := map[string]bool{}
	for _, a := range all {
		if a.ID() == "" || a.Version() == "" {
			t.Errorf("adapter %T has empty ID or Version", a)
		}
		if seen[a.ID()] {
			t.Errorf("duplicate adapter id %q", a.ID())
		}
		seen[a.ID()] = true
	}
	for _, id := range []string{"specscore", "codegraph", "manifests", "registries"} {
		if !seen[id] {
			t.Errorf("All() does not register the %s adapter", id)
		}
	}
}

func TestRun_StampsFactsCentrally(t *testing.T) {
	fixed := time.Date(2026, 7, 10, 12, 30, 0, 0, time.UTC)
	old := nowFn
	nowFn = func() time.Time { return fixed }
	t.Cleanup(func() { nowFn = old })

	a := fakeAdapter{
		id:      "fake",
		version: "9.9.9",
		facts: []fact.Fact{
			{Subject: "#x", Predicate: "has-status", Object: "Approved",
				Evidence: fact.Evidence{Class: fact.Declared, Pointer: "spec/features/x/README.md"}},
			{Subject: "#idea", Predicate: "promotes-to", Object: "#x",
				Evidence: fact.Evidence{Class: fact.Declared, Pointer: "spec/ideas/idea.md"}},
		},
	}
	// Two repos sharing a basename: the slugger disambiguates the second.
	res := Run([]Adapter{a}, []string{"/tmp/a/repo", "/tmp/b/repo"}, "demo")

	if got := len(res.Facts); got != 4 {
		t.Fatalf("len(Facts) = %d, want 4", got)
	}
	wantSubjects := []string{"repo#x", "repo#idea", "repo-2#x", "repo-2#idea"}
	wantObjects := []string{"Approved", "repo#x", "Approved", "repo-2#x"}
	for i, f := range res.Facts {
		if f.Subject != wantSubjects[i] {
			t.Errorf("Facts[%d].Subject = %q, want %q", i, f.Subject, wantSubjects[i])
		}
		if f.Object != wantObjects[i] {
			t.Errorf("Facts[%d].Object = %q, want %q", i, f.Object, wantObjects[i])
		}
		if f.Adapter.ID != "fake" || f.Adapter.Version != "9.9.9" {
			t.Errorf("Facts[%d].Adapter = %+v, want fake@9.9.9", i, f.Adapter)
		}
		if f.ObservedAt != "2026-07-10T12:30:00Z" {
			t.Errorf("Facts[%d].ObservedAt = %q, want fixed RFC 3339 stamp", i, f.ObservedAt)
		}
		if f.Ecosystem != "demo" {
			t.Errorf("Facts[%d].Ecosystem = %q, want demo", i, f.Ecosystem)
		}
	}
	if res.FactsByAdapter["fake"] != 4 {
		t.Errorf("FactsByAdapter[fake] = %d, want 4", res.FactsByAdapter["fake"])
	}
}

func TestRun_StampsWarningsAndCountsIdleAdapters(t *testing.T) {
	noisy := fakeAdapter{id: "noisy", version: "1", warnings: []Warning{{Message: "file boom"}}}
	idle := fakeAdapter{id: "idle", version: "1"}

	res := Run([]Adapter{noisy, idle}, []string{"/tmp/repo-a"}, "demo")

	if len(res.Facts) != 0 {
		t.Errorf("len(Facts) = %d, want 0", len(res.Facts))
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("len(Warnings) = %d, want 1", len(res.Warnings))
	}
	w := res.Warnings[0]
	if w.Repo != "repo-a" || w.Adapter != "noisy" || w.Message != "file boom" {
		t.Errorf("Warning = %+v, want repo-a/noisy/file boom", w)
	}
	for _, id := range []string{"noisy", "idle"} {
		if n, ok := res.FactsByAdapter[id]; !ok || n != 0 {
			t.Errorf("FactsByAdapter[%s] = %d (present=%v), want 0 entry", id, n, ok)
		}
	}
}
