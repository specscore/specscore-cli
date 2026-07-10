package specscore

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/studio/fact"
	"github.com/specscore/specscore-cli/pkg/feature"
	"github.com/specscore/specscore-cli/pkg/idea"
)

const fixtureRepo = "testdata/specrepo"

func TestAdapter_Identity(t *testing.T) {
	a := New()
	if a.ID() != "specscore" {
		t.Errorf("ID() = %q, want %q", a.ID(), "specscore")
	}
	if a.Version() == "" {
		t.Error("Version() is empty")
	}
}

func TestIngest_NonSpecScoreRepoYieldsNothing(t *testing.T) {
	dir := t.TempDir() // no specscore.yaml
	facts, warnings := New().Ingest(dir)
	if len(facts) != 0 || len(warnings) != 0 {
		t.Errorf("Ingest(non-specscore repo) = %d facts, %d warnings; want 0, 0", len(facts), len(warnings))
	}
}

func TestIngest_SpecScoreRepoWithoutSpecTree(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "specscore.yaml"), []byte("project:\n  title: Bare\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	facts, warnings := New().Ingest(dir)
	if len(facts) != 0 || len(warnings) != 0 {
		t.Errorf("Ingest(bare repo) = %d facts, %d warnings; want 0, 0", len(facts), len(warnings))
	}
}

// TestIngest_FixtureRepo exercises the full declared vocabulary against the
// committed fixture: has-status (features + ideas), has-ac, contains,
// sourced-from, and promotes-to (REQ: adapter-specscore).
func TestIngest_FixtureRepo(t *testing.T) {
	facts, warnings := New().Ingest(fixtureRepo)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %+v", warnings)
	}

	want := []fact.Fact{
		declared("#group/child", "has-status", "Draft", "spec/features/group/child/README.md"),
		declared("#x", "has-status", "Approved", "spec/features/x/README.md"),
		declared("#x", "has-ac", "first-ac", "spec/features/x/README.md:17"),
		declared("#x", "has-ac", "second-ac", "spec/features/x/README.md:21"),
		declared("#x/y", "has-status", "Draft", "spec/features/x/y/README.md"),
		declared("#x", "contains", "#x/y", "spec/features/x/y/README.md"),
		declared("#x", "sourced-from", "#ideas/seed-idea", "spec/features/x/README.md"),
		declared("#ideas/seed-idea", "has-status", "Approved", "spec/ideas/seed-idea.md"),
		declared("#ideas/seed-idea", "promotes-to", "#x", "spec/ideas/seed-idea.md"),
	}
	if !reflect.DeepEqual(facts, want) {
		t.Errorf("Ingest(fixture) mismatch:\ngot:  %+v\nwant: %+v", facts, want)
	}
}

// warningsFor runs Ingest on the fixture and returns the warnings whose
// message contains substr, failing the test when there are none.
func warningsFor(t *testing.T, substr string) []fact.Warning {
	t.Helper()
	_, warnings := New().Ingest(fixtureRepo)
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

func TestIngest_FeatureDiscoverErrorWarns(t *testing.T) {
	old := featureDiscoverFn
	featureDiscoverFn = func(string) ([]feature.Feature, error) { return nil, errors.New("walk boom") }
	t.Cleanup(func() { featureDiscoverFn = old })
	warningsFor(t, "discovering features")
}

func TestIngest_FeatureStatusErrorWarnsAndSkipsFile(t *testing.T) {
	old := parseFeatureStatusFn
	parseFeatureStatusFn = func(readmePath string) (string, error) {
		if strings.Contains(filepath.ToSlash(readmePath), "features/x/README.md") {
			return "", errors.New("status boom")
		}
		return feature.ParseFeatureStatus(readmePath)
	}
	t.Cleanup(func() { parseFeatureStatusFn = old })

	facts, _ := New().Ingest(fixtureRepo)
	warningsFor(t, "reading status of feature x")
	for _, f := range facts {
		if f.Subject == "#x" && f.Predicate == "has-status" {
			t.Errorf("has-status fact emitted for the failing feature: %+v", f)
		}
		if f.Subject == "#x/y" && f.Predicate == "has-status" {
			return // other files still ingested
		}
	}
	t.Error("healthy sibling feature x/y lost its has-status fact")
}

func TestIngest_ACReadErrorWarns(t *testing.T) {
	old := osReadFileFn
	osReadFileFn = func(string) ([]byte, error) { return nil, errors.New("read boom") }
	t.Cleanup(func() { osReadFileFn = old })
	warningsFor(t, "reading acceptance criteria")
}

func TestIngest_SourceIdeasErrorWarns(t *testing.T) {
	old := featureSourceIdeasFn
	featureSourceIdeasFn = func(string) (map[string][]string, error) { return nil, errors.New("scan boom") }
	t.Cleanup(func() { featureSourceIdeasFn = old })
	warningsFor(t, "reading feature Source Ideas")
}

func TestIngest_IdeaDiscoverErrorWarns(t *testing.T) {
	old := ideaDiscoverFn
	ideaDiscoverFn = func(string) ([]idea.Discovered, error) { return nil, errors.New("discover boom") }
	t.Cleanup(func() { ideaDiscoverFn = old })
	warningsFor(t, "discovering ideas")
}

func TestIngest_IdeaParseErrorWarnsAndSkipsFile(t *testing.T) {
	old := ideaParseFn
	ideaParseFn = func(path string) (*idea.Idea, error) {
		if strings.HasSuffix(path, "empty-idea.md") {
			return nil, errors.New("parse boom")
		}
		return idea.Parse(path)
	}
	t.Cleanup(func() { ideaParseFn = old })

	facts, _ := New().Ingest(fixtureRepo)
	warningsFor(t, "parsing idea empty-idea")
	for _, f := range facts {
		if f.Subject == "#ideas/seed-idea" && f.Predicate == "has-status" {
			return // sibling idea still ingested
		}
	}
	t.Error("healthy sibling idea seed-idea lost its has-status fact")
}

func TestIngest_IdeaRelErrorWarnsAndSkipsFile(t *testing.T) {
	old := filepathRelFn
	filepathRelFn = func(string, string) (string, error) { return "", errors.New("rel boom") }
	t.Cleanup(func() { filepathRelFn = old })
	warningsFor(t, "resolving idea path")
}

func TestParseACs_SkipsHeadingsOutsideACSectionAndEmptySlugs(t *testing.T) {
	dir := t.TempDir()
	readme := filepath.Join(dir, "README.md")
	content := strings.Join([]string{
		"# Feature: P",
		"",
		"## Requirements",
		"",
		"### AC: not-in-ac-section",
		"",
		"## Acceptance Criteria",
		"",
		"### AC: real-one",
		"",
		"### AC: :", // slug trims to empty → skipped
		"",
	}, "\n")
	if err := os.WriteFile(readme, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	acs, err := parseACs(readme)
	if err != nil {
		t.Fatal(err)
	}
	want := []acRef{{slug: "real-one", line: 9}}
	if !reflect.DeepEqual(acs, want) {
		t.Errorf("parseACs = %+v, want %+v", acs, want)
	}
}
