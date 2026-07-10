package registries

import (
	"errors"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/studio/fact"
)

// declaredFact mirrors the adapter's fact builder for expectations.
func declaredFact(s, p, o, pointer string) fact.Fact {
	return fact.Fact{
		Subject:   s,
		Predicate: p,
		Object:    o,
		Evidence:  fact.Evidence{Class: fact.Declared, Pointer: pointer},
	}
}

func TestAdapter_Identity(t *testing.T) {
	a := New(nil)
	if a.ID() != "registries" {
		t.Errorf("ID = %q, want registries", a.ID())
	}
	if a.Version() == "" {
		t.Error("Version is empty")
	}
}

func TestIngest_RegRepo_EmitsCuratedAndDomainFacts(t *testing.T) {
	facts, warnings := New(nil).Ingest("testdata/regrepo")

	want := []fact.Fact{
		// ecosystem-map.yaml (root, curated — canonical fronts source)
		declaredFact("examplus", "aliased-as", "Examplus", "ecosystem-map.yaml"),
		declaredFact("examplus", "implemented-by", "example-repo", "ecosystem-map.yaml"),
		declaredFact("examplus", "implemented-by", "example-ext", "ecosystem-map.yaml"),
		declaredFact("examplus", "member-of", "hospitality", "ecosystem-map.yaml"),
		declaredFact("example.app", "fronts", "examplus", "ecosystem-map.yaml"),
		// data/ecosystem.yaml (generated shape: mapping-form domains, null vertical)
		declaredFact("mappy", "aliased-as", "Mappy", "data/ecosystem.yaml"),
		declaredFact("mappy", "implemented-by", "mappy-repo", "data/ecosystem.yaml"),
		declaredFact("mapped.app", "fronts", "mappy", "data/ecosystem.yaml"),
		// domains.json, domains sorted; DomainID lower-cases and trims the dot;
		// no "/" status falls back to the first path; curated example.app gets
		// no worker-derived fronts, uncurated orphan.dev does.
		declaredFact("orphan.dev", "serves-status", "404", "domains.json"),
		declaredFact("orphan.dev", "fronts", "orphan-worker", "domains.json"),
		declaredFact("example.app", "serves-status", "200", "domains.json"),
		declaredFact("pick.app", "serves-status", "200", "domains.json"),
	}
	if len(facts) != len(want) {
		t.Fatalf("len(facts) = %d, want %d\nfacts: %+v", len(facts), len(want), facts)
	}
	for i := range want {
		if facts[i] != want[i] {
			t.Errorf("facts[%d] = %+v, want %+v", i, facts[i], want[i])
		}
	}
	for i, f := range facts {
		if strings.HasPrefix(f.Subject, "#") || strings.HasPrefix(f.Object, "#") {
			t.Errorf("facts[%d] has a repo-relative coordinate; registry subjects/objects are global: %+v", i, f)
		}
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0].Message, "has no id") {
		t.Errorf("warnings = %+v, want exactly one no-id product warning", warnings)
	}
}

func TestIngest_AgreeingRegistryFilesDedupToOneFact(t *testing.T) {
	// Two ecosystem files (data/ecosystem-map.yaml and data/ecosystem.yaml)
	// declare the same product identically. Cross-source agreement is a
	// confidence signal, but each relationship is one fact: the adapter must
	// not emit duplicates. The surviving pointer is the first file in
	// discovery order — data/ecosystem-map.yaml (glob sorts "-" before ".").
	facts, warnings := New(nil).Ingest("testdata/agreerepo")

	want := []fact.Fact{
		declaredFact("dupius", "aliased-as", "Dupius", "data/ecosystem-map.yaml"),
		declaredFact("dupius", "implemented-by", "dupius-repo", "data/ecosystem-map.yaml"),
		declaredFact("dupius", "member-of", "hospitality", "data/ecosystem-map.yaml"),
		declaredFact("dup.app", "fronts", "dupius", "data/ecosystem-map.yaml"),
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %+v, want none", warnings)
	}
	if len(facts) != len(want) {
		t.Fatalf("len(facts) = %d, want %d — duplicate registry facts leaked\nfacts: %+v", len(facts), len(want), facts)
	}
	for i := range want {
		if facts[i] != want[i] {
			t.Errorf("facts[%d] = %+v, want %+v", i, facts[i], want[i])
		}
	}
	// No two facts may share (subject, predicate, object, evidence_class).
	type key struct {
		s, p, o string
		c       fact.Class
	}
	seen := map[key]bool{}
	for _, f := range facts {
		k := key{f.Subject, f.Predicate, f.Object, f.Class}
		if seen[k] {
			t.Errorf("duplicate fact for %+v", k)
		}
		seen[k] = true
	}
}

func TestIngest_DomainsOnly_FallsBackToWorkerFronts(t *testing.T) {
	facts, warnings := New(nil).Ingest("testdata/domonly")

	want := []fact.Fact{
		declaredFact("solo.app", "serves-status", "200", "domains.json"),
		declaredFact("solo.app", "fronts", "solo-worker", "domains.json"),
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %+v, want none", warnings)
	}
	if len(facts) != len(want) {
		t.Fatalf("len(facts) = %d, want %d\nfacts: %+v", len(facts), len(want), facts)
	}
	for i := range want {
		if facts[i] != want[i] {
			t.Errorf("facts[%d] = %+v, want %+v", i, facts[i], want[i])
		}
	}
}

func TestIngest_MalformedFilesWarnAndSkip(t *testing.T) {
	facts, warnings := New(nil).Ingest("testdata/brokenrepo")

	if len(facts) != 0 {
		t.Errorf("facts = %+v, want none", facts)
	}
	// One warning per skipped file: bad JSON, bad YAML, bad domain ref.
	if len(warnings) != 3 {
		t.Fatalf("warnings = %+v, want 3", warnings)
	}
	for _, wantRel := range []string{"ecosystem.yaml", "data/ecosystem.yaml", "domains.json"} {
		found := false
		for _, w := range warnings {
			if strings.Contains(w.Message, wantRel) {
				found = true
			}
			if strings.Contains(w.Message, "\n") {
				t.Errorf("warning is not one line: %q", w.Message)
			}
		}
		if !found {
			t.Errorf("no warning names %s: %+v", wantRel, warnings)
		}
	}
}

func TestIngest_NoRegistryFiles(t *testing.T) {
	facts, warnings := New(nil).Ingest(t.TempDir())
	if len(facts) != 0 || len(warnings) != 0 {
		t.Errorf("facts = %+v, warnings = %+v, want none", facts, warnings)
	}
}

func TestIngest_ConfiguredRegistries(t *testing.T) {
	repo := "testdata/configrepo"
	a := New(map[string][]string{
		repo: {
			"ops/registry.json",
			"ops/registry.json", // duplicate — parsed once
			"ops/products.yml",
			"missing.json", // configured but absent — warns
			"notes.txt",    // unrecognized type — warns
		},
	})
	facts, warnings := a.Ingest(repo)

	want := []fact.Fact{
		declaredFact("configius", "aliased-as", "Configius", "ops/products.yml"),
		declaredFact("config.app", "fronts", "configius", "ops/products.yml"),
		// Curated fronts suppress the worker fallback for config.app.
		declaredFact("config.app", "serves-status", "301", "ops/registry.json"),
	}
	if len(facts) != len(want) {
		t.Fatalf("len(facts) = %d, want %d\nfacts: %+v", len(facts), len(want), facts)
	}
	for i := range want {
		if facts[i] != want[i] {
			t.Errorf("facts[%d] = %+v, want %+v", i, facts[i], want[i])
		}
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings = %+v, want 2", warnings)
	}
	if !strings.Contains(warnings[0].Message, "notes.txt") {
		t.Errorf("warnings[0] = %+v, want unrecognized-type warning for notes.txt", warnings[0])
	}
	if !strings.Contains(warnings[1].Message, "missing.json") {
		t.Errorf("warnings[1] = %+v, want read warning for missing.json", warnings[1])
	}
}

func TestIngest_ConfigForOtherRepoIsIgnored(t *testing.T) {
	a := New(map[string][]string{"testdata/configrepo": {"ops/registry.json"}})
	facts, warnings := a.Ingest(t.TempDir())
	if len(facts) != 0 || len(warnings) != 0 {
		t.Errorf("facts = %+v, warnings = %+v, want none", facts, warnings)
	}
}

func TestIngest_ReadErrorWarns(t *testing.T) {
	old := osReadFileFn
	osReadFileFn = func(string) ([]byte, error) { return nil, errors.New("read boom") }
	t.Cleanup(func() { osReadFileFn = old })

	facts, warnings := New(nil).Ingest("testdata/regrepo")
	if len(facts) != 0 {
		t.Errorf("facts = %+v, want none", facts)
	}
	// One warning per unreadable file: both ecosystem maps + domains.json.
	if len(warnings) != 3 {
		t.Fatalf("warnings = %+v, want 3", warnings)
	}
	for i, w := range warnings {
		if !strings.Contains(w.Message, "read boom") {
			t.Errorf("warnings[%d] = %+v, want a read-error warning", i, w)
		}
	}
}

func TestIngest_GlobErrorWarns(t *testing.T) {
	old := filepathGlobFn
	filepathGlobFn = func(string) ([]string, error) { return nil, errors.New("glob boom") }
	t.Cleanup(func() { filepathGlobFn = old })

	facts, warnings := New(nil).Ingest("testdata/domonly")
	// domains.json still parses; only ecosystem discovery warns (root + data/).
	if len(facts) != 2 {
		t.Errorf("len(facts) = %d, want 2 (domains.json still ingested)", len(facts))
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings = %+v, want 2 (one per scanned dir)", warnings)
	}
	for _, w := range warnings {
		if !strings.Contains(w.Message, "glob boom") {
			t.Errorf("warning %+v does not carry the glob error", w)
		}
	}
}
