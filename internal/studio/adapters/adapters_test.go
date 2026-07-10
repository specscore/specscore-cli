package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/specscore/specscore-cli/internal/studio/fact"
)

// fakeAdapter emits a fixed set of facts and warnings for every repo and
// records the repo paths it was asked to ingest.
type fakeAdapter struct {
	id       string
	version  string
	facts    []fact.Fact
	warnings []Warning
	ingested []string
}

func (a *fakeAdapter) ID() string      { return a.id }
func (a *fakeAdapter) Version() string { return a.version }
func (a *fakeAdapter) Ingest(repoPath string) ([]fact.Fact, []Warning) {
	a.ingested = append(a.ingested, repoPath)
	// Return copies so central stamping never mutates the fixtures.
	return append([]fact.Fact(nil), a.facts...), append([]Warning(nil), a.warnings...)
}

// panicAdapter panics on every Ingest call.
type panicAdapter struct{}

func (panicAdapter) ID() string      { return "panicky" }
func (panicAdapter) Version() string { return "1" }
func (panicAdapter) Ingest(string) ([]fact.Fact, []Warning) {
	panic("boom")
}

// mkRepoDir creates dir/name and returns its absolute path.
func mkRepoDir(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
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

	a := &fakeAdapter{
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
	dir := t.TempDir()
	repoA := mkRepoDir(t, dir, filepath.Join("a", "repo"))
	repoB := mkRepoDir(t, dir, filepath.Join("b", "repo"))
	res := Run([]Adapter{a}, []string{repoA, repoB}, "demo")

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
	// Per-repo fact attribution (REQ: ingr-export): each repo's group holds
	// exactly the facts its adapter runs produced, matching the summary count.
	for slug, want := range map[string][]string{
		"repo":   {"repo#x", "repo#idea"},
		"repo-2": {"repo-2#x", "repo-2#idea"},
	} {
		group := res.FactsByRepo[slug]
		if len(group) != len(want) {
			t.Fatalf("len(FactsByRepo[%s]) = %d, want %d", slug, len(group), len(want))
		}
		for i, f := range group {
			if f.Subject != want[i] {
				t.Errorf("FactsByRepo[%s][%d].Subject = %q, want %q", slug, i, f.Subject, want[i])
			}
		}
	}
	// Per-repo summaries: two facts per repo, no warnings.
	if len(res.Repos) != 2 {
		t.Fatalf("len(Repos) = %d, want 2", len(res.Repos))
	}
	wantRepos := []RepoSummary{
		{Path: repoA, Slug: "repo", Facts: 2, Warnings: 0},
		{Path: repoB, Slug: "repo-2", Facts: 2, Warnings: 0},
	}
	for i, want := range wantRepos {
		if res.Repos[i] != want {
			t.Errorf("Repos[%d] = %+v, want %+v", i, res.Repos[i], want)
		}
	}
}

func TestRun_StampsWarningsAndCountsIdleAdapters(t *testing.T) {
	noisy := &fakeAdapter{id: "noisy", version: "1", warnings: []Warning{{Message: "file boom"}}}
	idle := &fakeAdapter{id: "idle", version: "1"}
	repoA := mkRepoDir(t, t.TempDir(), "repo-a")

	res := Run([]Adapter{noisy, idle}, []string{repoA}, "demo")

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
	if len(res.Repos) != 1 || res.Repos[0].Warnings != 1 || res.Repos[0].Facts != 0 {
		t.Errorf("Repos = %+v, want one summary with 0 facts, 1 warning", res.Repos)
	}
}

// REQ: partial-tolerance (repo granularity) — a repo path that is not an
// existing directory is skipped with a repo-level warning; no adapter runs
// for it and every other repo is still ingested.
func TestRun_MissingRepoDirWarnsAndSkips(t *testing.T) {
	dir := t.TempDir()
	healthy := mkRepoDir(t, dir, "repo-a")
	missing := filepath.Join(dir, "gone")
	a := &fakeAdapter{id: "fake", version: "1",
		facts: []fact.Fact{{Subject: "s", Predicate: "p", Object: "o"}}}

	res := Run([]Adapter{a}, []string{healthy, missing}, "demo")

	if len(a.ingested) != 1 || a.ingested[0] != healthy {
		t.Errorf("adapter ingested %v, want only [%s]", a.ingested, healthy)
	}
	if len(res.Facts) != 1 {
		t.Errorf("len(Facts) = %d, want 1 from the healthy repo", len(res.Facts))
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("len(Warnings) = %d, want 1", len(res.Warnings))
	}
	w := res.Warnings[0]
	if w.Repo != "gone" || w.Adapter != "" {
		t.Errorf("Warning = %+v, want repo-level warning (Repo gone, empty Adapter)", w)
	}
	if !strings.Contains(w.Message, missing) {
		t.Errorf("Warning message %q does not name the missing path %q", w.Message, missing)
	}
	wantRepos := []RepoSummary{
		{Path: healthy, Slug: "repo-a", Facts: 1, Warnings: 0},
		{Path: missing, Slug: "gone", Facts: 0, Warnings: 1},
	}
	for i, want := range wantRepos {
		if res.Repos[i] != want {
			t.Errorf("Repos[%d] = %+v, want %+v", i, res.Repos[i], want)
		}
	}
}

// A repo path naming a regular file (not a directory) is skipped the same
// way as a missing one.
func TestRun_RepoPathIsFileWarnsAndSkips(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &fakeAdapter{id: "fake", version: "1"}

	res := Run([]Adapter{a}, []string{filePath}, "demo")

	if len(a.ingested) != 0 {
		t.Errorf("adapter ingested %v, want nothing", a.ingested)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0].Message, filePath) {
		t.Errorf("Warnings = %+v, want one naming %s", res.Warnings, filePath)
	}
}

// REQ: partial-tolerance (adapter granularity) — a panicking adapter is
// recovered into a warning naming the adapter and repo; the remaining
// adapters still run for that repo.
func TestRun_AdapterPanicRecoveredAsWarning(t *testing.T) {
	repoA := mkRepoDir(t, t.TempDir(), "repo-a")
	healthy := &fakeAdapter{id: "fake", version: "1",
		facts: []fact.Fact{{Subject: "s", Predicate: "p", Object: "o"}}}

	res := Run([]Adapter{panicAdapter{}, healthy}, []string{repoA}, "demo")

	if len(healthy.ingested) != 1 {
		t.Errorf("healthy adapter ingested %v, want the repo despite the earlier panic", healthy.ingested)
	}
	if len(res.Facts) != 1 {
		t.Errorf("len(Facts) = %d, want 1 from the healthy adapter", len(res.Facts))
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("len(Warnings) = %d, want 1", len(res.Warnings))
	}
	w := res.Warnings[0]
	if w.Repo != "repo-a" || w.Adapter != "panicky" {
		t.Errorf("Warning = %+v, want it stamped with repo-a/panicky", w)
	}
	for _, want := range []string{"panic", "boom"} {
		if !strings.Contains(w.Message, want) {
			t.Errorf("Warning message %q does not mention %q", w.Message, want)
		}
	}
	if res.FactsByAdapter["panicky"] != 0 || res.FactsByAdapter["fake"] != 1 {
		t.Errorf("FactsByAdapter = %v, want panicky:0 fake:1", res.FactsByAdapter)
	}
	if len(res.Repos) != 1 || res.Repos[0].Facts != 1 || res.Repos[0].Warnings != 1 {
		t.Errorf("Repos = %+v, want one summary with 1 fact, 1 warning", res.Repos)
	}
}
