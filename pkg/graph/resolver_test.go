package graph

import (
	"path/filepath"
	"testing"
)

func buildValidResolver(t *testing.T) *Resolver {
	t.Helper()
	g := loadValid(t)
	return BuildResolver("testdata/valid", g)
}

func TestResolver_LocalResolution(t *testing.T) {
	r := buildValidResolver(t)
	if got := r.resolveConcept("identity", "TeamRole", ""); got != resResolved {
		t.Fatalf("local resolved: %v", got)
	}
	if got := r.resolveConcept("identity", "Ghost", ""); got != resUnknownConcept {
		t.Fatalf("unknown concept: %v", got)
	}
	if got := r.resolveConcept("zzz", "X", ""); got != resUnknownModule {
		t.Fatalf("unknown module: %v", got)
	}
}

func TestResolver_SuffixUnavailable(t *testing.T) {
	r := buildValidResolver(t)
	if got := r.resolveConcept("identity", "TeamRole", "example.com/acme/gone"); got != resRepoUnavailable {
		t.Fatalf("expected repo unavailable, got %v", got)
	}
}

func TestResolver_ProjectsAndSuffix(t *testing.T) {
	g, ok, err := Load("testdata/proja", "")
	if err != nil || !ok {
		t.Fatalf("load proja: %v %v", ok, err)
	}
	r := BuildResolver("testdata/proja", g)
	// Step 2: configured projects.
	if got := r.resolveConcept("shared", "Thing", ""); got != resResolved {
		t.Fatalf("project resolution: %v", got)
	}
	if got := r.resolveConcept("shared", "Ghost", ""); got != resUnknownConcept {
		t.Fatalf("project unknown concept: %v", got)
	}
	// Step 3: explicit suffix against the locally-available projb.
	if got := r.resolveConcept("shared", "Thing", "example.com/acme/projb"); got != resResolved {
		t.Fatalf("suffix resolution: %v", got)
	}
	if got := r.resolveConcept("shared", "Ghost", "example.com/acme/projb"); got != resUnknownConcept {
		t.Fatalf("suffix unknown concept: %v", got)
	}
	if got := r.resolveConcept("nope", "Thing", "example.com/acme/projb"); got != resUnknownModule {
		t.Fatalf("suffix unknown module: %v", got)
	}
}

func TestResolver_AmbiguousAcrossProjects(t *testing.T) {
	// Two configured projects both providing module "shared".
	base := t.TempDir()
	mk := func(name string) {
		root := filepath.Join(base, name)
		writeFile(t, filepath.Join(root, "specscore.yaml"), testConfig)
		writeFile(t, filepath.Join(root, "spec/graph/modules/shared/README.md"), fmModule("shared", "[]"))
		writeFile(t, filepath.Join(root, "spec/graph/modules/shared/models/m.hcl"), "entity \"Thing\" {}\n")
	}
	mk("p1")
	mk("p2")
	consumer := filepath.Join(base, "consumer")
	writeFile(t, filepath.Join(consumer, "specscore.yaml"),
		"# SpecScore Repo Config Schema: https://specscore.md/repo-config\n\nprojects:\n  - ../p1\n  - ../p2\n")
	writeFile(t, filepath.Join(consumer, "spec/graph/modules/c/README.md"), fmModule("c", "[shared]"))
	writeFile(t, filepath.Join(consumer, "spec/graph/modules/c/entities/e.md"),
		fmArt("entity", "e", "model: modelspec://shared.Thing"))

	g, ok, err := Load(consumer, "")
	if err != nil || !ok {
		t.Fatal(err)
	}
	r := BuildResolver(consumer, g)
	if got := r.resolveConcept("shared", "Thing", ""); got != resAmbiguousModule {
		t.Fatalf("expected ambiguous, got %v", got)
	}
	// The ambiguity also surfaces as a lint violation.
	res := lintRepo(t, consumer)
	if !hasRule(res.Violations, "graph-model-ref-resolves") {
		t.Fatalf("expected ambiguity violation: %+v", res.Violations)
	}
}

func TestResolver_SkipsBadProjectEntries(t *testing.T) {
	consumer := t.TempDir()
	writeFile(t, filepath.Join(consumer, "specscore.yaml"),
		"# SpecScore Repo Config Schema: https://specscore.md/repo-config\n\nprojects:\n  - https://example.com/not-local\n  - ../missing\n  - ./notaproject\n")
	// notaproject exists but has no specscore.yaml -> Load ok=false or no config; either way skipped.
	writeFile(t, filepath.Join(consumer, "notaproject", ".keep"), "")
	writeFile(t, filepath.Join(consumer, "spec/graph/modules/m/README.md"), fmModule("m", "[]"))
	g, _, err := Load(consumer, "")
	if err != nil {
		t.Fatal(err)
	}
	r := BuildResolver(consumer, g)
	if got := r.resolveConcept("anything", "X", ""); got != resUnknownModule {
		t.Fatalf("expected unknown module with all projects skipped, got %v", got)
	}
}

func TestResolver_NoConfig(t *testing.T) {
	dir := t.TempDir() // no specscore.yaml
	writeFile(t, filepath.Join(dir, "spec/graph/modules/m/README.md"), fmModule("m", "[]"))
	g, _, err := Load(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	r := BuildResolver(dir, g)
	if got := r.resolveConcept("zzz", "X", ""); got != resUnknownModule {
		t.Fatalf("local-only resolver: %v", got)
	}
}

func TestResolver_AbsoluteProjectPath(t *testing.T) {
	base := t.TempDir()
	prov := filepath.Join(base, "prov")
	writeFile(t, filepath.Join(prov, "specscore.yaml"), testConfig)
	writeFile(t, filepath.Join(prov, "spec/graph/modules/shared/README.md"), fmModule("shared", "[]"))
	writeFile(t, filepath.Join(prov, "spec/graph/modules/shared/models/m.hcl"), "entity \"Thing\" {}\n")

	consumer := filepath.Join(base, "consumer")
	writeFile(t, filepath.Join(consumer, "specscore.yaml"),
		"# SpecScore Repo Config Schema: https://specscore.md/repo-config\n\nprojects:\n  - "+prov+"\n")
	writeFile(t, filepath.Join(consumer, "spec/graph/modules/m/README.md"), fmModule("m", "[shared]"))
	g, _, err := Load(consumer, "")
	if err != nil {
		t.Fatal(err)
	}
	r := BuildResolver(consumer, g)
	if got := r.resolveConcept("shared", "Thing", ""); got != resResolved {
		t.Fatalf("absolute project path: %v", got)
	}
}

func TestProjectIdentity(t *testing.T) {
	// Missing config.
	if projectIdentity(t.TempDir()) != "" {
		t.Fatal("no config should yield empty identity")
	}
	// Config without project block.
	d1 := t.TempDir()
	writeFile(t, filepath.Join(d1, "specscore.yaml"),
		"# SpecScore Repo Config Schema: https://specscore.md/repo-config\n\nspecs_dir_name: spec\n")
	if projectIdentity(d1) != "" {
		t.Fatal("no project block should yield empty identity")
	}
	// Incomplete identity.
	d2 := t.TempDir()
	writeFile(t, filepath.Join(d2, "specscore.yaml"),
		"# SpecScore Repo Config Schema: https://specscore.md/repo-config\n\nproject:\n  host: h\n  org: o\n")
	if projectIdentity(d2) != "" {
		t.Fatal("incomplete identity should be empty")
	}
	// Full identity.
	d3 := t.TempDir()
	writeFile(t, filepath.Join(d3, "specscore.yaml"),
		"# SpecScore Repo Config Schema: https://specscore.md/repo-config\n\nproject:\n  host: h\n  org: o\n  repo: r\n")
	if projectIdentity(d3) != "h/o/r" {
		t.Fatalf("identity: %q", projectIdentity(d3))
	}
}

func TestConceptOutcome(t *testing.T) {
	mm := &ModelModule{Concepts: []*Concept{{Name: "A"}}}
	if conceptOutcome(nil, "A", false) != resUnknownModule {
		t.Fatal("not found")
	}
	if conceptOutcome(nil, "A", true) != resUnknownModule {
		t.Fatal("nil module")
	}
	if conceptOutcome(mm, "A", true) != resResolved {
		t.Fatal("resolved")
	}
	if conceptOutcome(mm, "B", true) != resUnknownConcept {
		t.Fatal("unknown concept")
	}
}
