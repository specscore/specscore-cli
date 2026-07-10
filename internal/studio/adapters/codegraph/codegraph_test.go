package codegraph

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/studio/fact"
)

const (
	fixtureRepo  = "testdata/snaprepo"
	danglingRepo = "testdata/danglingrepo"
	brokenRepo   = "testdata/brokenrepo"
)

func TestAdapter_Identity(t *testing.T) {
	a := New()
	if a.ID() != "codegraph" {
		t.Errorf("ID() = %q, want %q", a.ID(), "codegraph")
	}
	if a.Version() == "" {
		t.Error("Version() is empty")
	}
}

func TestIngest_RepoWithoutSnapshotYieldsNothing(t *testing.T) {
	dir := t.TempDir() // no codegraph/ directory
	facts, warnings := New().Ingest(dir)
	if len(facts) != 0 || len(warnings) != 0 {
		t.Errorf("Ingest(bare repo) = %d facts, %d warnings; want 0, 0", len(facts), len(warnings))
	}
}

// derived mirrors the emitted fact shape (pipeline stamps the rest).
func derived(subject, object, pointer string) fact.Fact {
	return fact.Fact{
		Subject:   subject,
		Predicate: "imports",
		Object:    object,
		Evidence:  fact.Evidence{Class: fact.Derived, Pointer: pointer},
	}
}

// TestIngest_FixtureRepo exercises the full derivation against the committed
// fixture (real CodeGrapher headers): file-level imports edges collapse to
// package granularity with dedupe, symbol-level sources resolve through their
// file, root files land in the `#.` package, non-imports edges are ignored,
// and snapshot-provided package-granularity edges pass through
// (REQ: adapter-codegraph).
func TestIngest_FixtureRepo(t *testing.T) {
	facts, warnings := New().Ingest(fixtureRepo)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %+v", warnings)
	}
	pointer := "codegraph/edges/edges.ingr"
	want := []fact.Fact{
		derived("#a", "b", pointer),
		derived("#a", "example.com/dep/pkg", pointer),
		derived("#c", "example.com/dep/pkg", pointer),
		derived("#.", "fmt", pointer),
		derived("#pkg1", "#pkg2", pointer),
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

// TestIngest_DanglingEdgesWarnOnceAndYieldNoFacts covers every unresolvable
// endpoint shape: unknown target node, unknown source node, source node with
// no file path, import target with a null name, and a target of the wrong
// kind. The nodes file also carries null and non-string $ID records that
// must be skipped.
func TestIngest_DanglingEdgesWarnOnceAndYieldNoFacts(t *testing.T) {
	facts, warnings := New().Ingest(danglingRepo)
	if len(facts) != 0 {
		t.Errorf("facts = %+v, want none", facts)
	}
	if len(warnings) != 1 {
		t.Fatalf("len(warnings) = %d, want 1: %+v", len(warnings), warnings)
	}
	warningsFor(t, danglingRepo, "skipped 5 of 5 imports edges with unresolvable endpoints in codegraph/edges/edges.ingr")
}

func TestIngest_MalformedFilesWarnAndSkip(t *testing.T) {
	facts, warnings := New().Ingest(brokenRepo)
	if len(facts) != 0 {
		t.Errorf("facts = %+v, want none", facts)
	}
	if len(warnings) != 2 {
		t.Fatalf("len(warnings) = %d, want 2: %+v", len(warnings), warnings)
	}
	warningsFor(t, brokenRepo, "parsing codegraph/nodes/nodes.ingr: not an INGR recordset")
	warningsFor(t, brokenRepo, "parsing codegraph/edges/edges.ingr: 3 value lines is not a multiple of 4 fields")
}

// writeSnapshot builds a temp repo with the given recordset files, keyed by
// repo-relative slash path (e.g. "codegraph/nodes/nodes.ingr").
func writeSnapshot(t *testing.T, files map[string]string) string {
	t.Helper()
	repo := t.TempDir()
	for rel, content := range files {
		abs := filepath.Join(repo, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return repo
}

const minimalNodes = `# INGR.io | nodes: $ID, kind, name, file_path
"import:i1"
"import"
"b"
"a/x.go"
`

func minimalEdge(id string) string {
	return `"` + id + `"
"file:a/x.go"
"import:i1"
"imports"
`
}

func TestIngest_HeaderErrorsWarnAndSkipFile(t *testing.T) {
	cases := []struct {
		name, nodes, wantWarn string
	}{
		{"missing required field", "# INGR.io | nodes: $ID, kind, name\n", "header lacks required field(s) file_path"},
		{"no pipe", "# INGR.io nodes $ID, kind\n", "malformed INGR header"},
		{"no colon", "# INGR.io | nodes\n", "malformed INGR header"},
		{"no fields", "# INGR.io | nodes:\n", "INGR header declares no fields"},
		{"empty file", "", "not an INGR recordset"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := writeSnapshot(t, map[string]string{"codegraph/nodes/nodes.ingr": tc.nodes})
			warningsFor(t, repo, tc.wantWarn)
		})
	}
}

// TestIngest_MultipleEdgesFilesDedupeAcrossFiles also covers: non-.ingr
// entries and subdirectories inside a recordset dir are ignored, and an
// edges recordset with zero records yields nothing.
func TestIngest_MultipleEdgesFilesDedupeAcrossFiles(t *testing.T) {
	edgesHeader := "# INGR.io | edges: $ID, source, target, kind\n"
	repo := writeSnapshot(t, map[string]string{
		"codegraph/nodes/nodes.ingr":   minimalNodes,
		"codegraph/edges/a.ingr":       edgesHeader + minimalEdge("e1"),
		"codegraph/edges/b.ingr":       edgesHeader + minimalEdge("e2"), // same package pair
		"codegraph/edges/empty.ingr":   edgesHeader + "# 0 records\n",
		"codegraph/edges/README.md":    "not a recordset",
		"codegraph/edges/sub/junk.txt": "ignored: recordset dirs are scanned one level deep",
	})
	facts, warnings := New().Ingest(repo)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %+v", warnings)
	}
	want := []fact.Fact{derived("#a", "b", "codegraph/edges/a.ingr")}
	if !reflect.DeepEqual(facts, want) {
		t.Errorf("facts mismatch:\ngot:  %+v\nwant: %+v", facts, want)
	}
}

func TestIngest_ReadDirErrorWarns(t *testing.T) {
	old := osReadDirFn
	osReadDirFn = func(string) ([]os.DirEntry, error) { return nil, errors.New("dir boom") }
	t.Cleanup(func() { osReadDirFn = old })

	facts, _ := New().Ingest(fixtureRepo)
	if len(facts) != 0 {
		t.Errorf("facts = %+v, want none", facts)
	}
	warningsFor(t, fixtureRepo, "reading codegraph/nodes: dir boom")
	warningsFor(t, fixtureRepo, "reading codegraph/edges: dir boom")
}

func TestIngest_ReadFileErrorWarnsAndSkipsFile(t *testing.T) {
	old := osReadFileFn
	osReadFileFn = func(string) ([]byte, error) { return nil, errors.New("read boom") }
	t.Cleanup(func() { osReadFileFn = old })

	facts, _ := New().Ingest(fixtureRepo)
	if len(facts) != 0 {
		t.Errorf("facts = %+v, want none", facts)
	}
	warningsFor(t, fixtureRepo, "reading codegraph/nodes/nodes.ingr: read boom")
	warningsFor(t, fixtureRepo, "reading codegraph/edges/edges.ingr: read boom")
}
