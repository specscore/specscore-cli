package manifests

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/studio/fact"
)

const (
	fixtureRepo = "testdata/manifestrepo"
	brokenRepo  = "testdata/brokenrepo"
)

func TestAdapter_Identity(t *testing.T) {
	a := New()
	if a.ID() != "manifests" {
		t.Errorf("ID() = %q, want %q", a.ID(), "manifests")
	}
	if a.Version() == "" {
		t.Error("Version() is empty")
	}
}

func TestIngest_RepoWithoutManifestsYieldsNothing(t *testing.T) {
	dir := t.TempDir() // no go.mod, no package.json
	facts, warnings := New().Ingest(dir)
	if len(facts) != 0 || len(warnings) != 0 {
		t.Errorf("Ingest(bare repo) = %d facts, %d warnings; want 0, 0", len(facts), len(warnings))
	}
}

// TestIngest_FixtureRepo exercises the full derived vocabulary against the
// committed fixture: publishes + consumes from go.mod and package.json at the
// repo root and one nested level, skipping indirect Go deps, dot-dirs,
// node_modules, and deeper nesting (REQ: adapter-manifests).
func TestIngest_FixtureRepo(t *testing.T) {
	facts, warnings := New().Ingest(fixtureRepo)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %+v", warnings)
	}

	want := []fact.Fact{
		derived("#go.mod", "publishes", "example.com/fixture", "go.mod"),
		derived("#go.mod", "consumes", "example.com/m@v1.2.3", "go.mod"),
		derived("#go.mod", "consumes", "example.com/direct@v0.5.0", "go.mod"),
		derived("#package.json", "publishes", "fixture-app@1.0.0", "package.json"),
		derived("#package.json", "consumes", "left-pad@^1.3.0", "package.json"),
		derived("#package.json", "consumes", "typescript@~5.4.2", "package.json"),
		derived("#backend/go.mod", "publishes", "example.com/fixture/backend", "backend/go.mod"),
		derived("#backend/go.mod", "consumes", "example.com/util@v0.1.0", "backend/go.mod"),
		derived("#frontend/package.json", "publishes", "fixture-frontend", "frontend/package.json"),
		derived("#frontend/package.json", "consumes", "react@18.2.0", "frontend/package.json"),
		derived("#nomodule/go.mod", "consumes", "example.com/x@v1.0.0", "nomodule/go.mod"),
		derived("#scripts/package.json", "consumes", "x@1.0.0", "scripts/package.json"),
	}
	if !reflect.DeepEqual(facts, want) {
		t.Errorf("Ingest(fixture) mismatch:\ngot:  %+v\nwant: %+v", facts, want)
	}
}

// warningsFor runs Ingest on repo and returns the warnings whose message
// contains substr, failing the test when there are none.
func warningsFor(t *testing.T, repo, substr string) []fact.Warning {
	t.Helper()
	_, warnings := New().Ingest(repo)
	var matched []fact.Warning
	for _, w := range warnings {
		if strings.Contains(w.Message, substr) {
			matched = append(matched, w)
		}
	}
	if len(matched) == 0 {
		t.Errorf("no warning containing %q; got %+v", substr, warnings)
	}
	return matched
}

func TestIngest_BrokenManifestsWarnAndYieldNoFacts(t *testing.T) {
	facts, warnings := New().Ingest(brokenRepo)
	if len(facts) != 0 {
		t.Errorf("Ingest(broken repo) facts = %+v, want none", facts)
	}
	if len(warnings) != 2 {
		t.Fatalf("len(warnings) = %d, want 2 (go.mod + package.json): %+v", len(warnings), warnings)
	}
	warningsFor(t, brokenRepo, "parsing go.mod")
	warningsFor(t, brokenRepo, "parsing package.json")
}

func TestIngest_RepoReadDirErrorWarns(t *testing.T) {
	old := osReadDirFn
	osReadDirFn = func(string) ([]os.DirEntry, error) { return nil, errors.New("dir boom") }
	t.Cleanup(func() { osReadDirFn = old })

	facts, _ := New().Ingest(fixtureRepo)
	if len(facts) != 0 {
		t.Errorf("facts = %+v, want none", facts)
	}
	warningsFor(t, fixtureRepo, "reading repo")
}

func TestIngest_ManifestReadErrorWarnsAndSkipsFile(t *testing.T) {
	old := osReadFileFn
	osReadFileFn = func(path string) ([]byte, error) {
		slash := strings.ReplaceAll(path, "\\", "/")
		if strings.HasSuffix(slash, "backend/go.mod") || strings.HasSuffix(slash, "frontend/package.json") {
			return nil, errors.New("read boom")
		}
		return old(path)
	}
	t.Cleanup(func() { osReadFileFn = old })

	facts, _ := New().Ingest(fixtureRepo)
	warningsFor(t, fixtureRepo, "reading backend/go.mod")
	warningsFor(t, fixtureRepo, "reading frontend/package.json")
	for _, f := range facts {
		if strings.HasPrefix(f.Subject, "#backend/") || strings.HasPrefix(f.Subject, "#frontend/") {
			t.Errorf("fact emitted for a failing manifest: %+v", f)
		}
	}
	// Healthy sibling manifests are still ingested.
	var sawRoot bool
	for _, f := range facts {
		if f.Subject == "#go.mod" {
			sawRoot = true
		}
	}
	if !sawRoot {
		t.Error("healthy root go.mod lost its facts")
	}
}
