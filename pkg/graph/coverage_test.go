package graph

// Targeted tests for defensive branches and rare shapes that the main fixture
// suites do not reach. Table-driven where possible; each case names the branch
// it pins.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLint_ModuleIDMismatchesDirectory(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md": fmModule("other", "[]"),
	})
	res := lintRepo(t, root)
	if !hasRule(res.Violations, "graph-id-equals-filename-stem") {
		t.Fatalf("expected module id/directory mismatch: %+v", res.Violations)
	}
}

func TestLint_MessageTieBreakSort(t *testing.T) {
	// Two duplicate lifecycle states on the same line/rule produce two
	// violations differing only in message — exercising the final sort key.
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
		"spec/graph/modules/m/entities/e.md": fmArt("entity", "e", "lifecycle:", "  states: [a, a, b, b]"),
	})
	res := lintRepo(t, root)
	if ruleCounts(res.Violations)["graph-lifecycle-states"] != 2 {
		t.Fatalf("expected 2 duplicate-state violations: %+v", res.Violations)
	}
}

func TestLint_LifecycleMappingWithOtherKeys(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
		"spec/graph/modules/m/entities/e.md": fmArt("entity", "e", "lifecycle:", "  transitions: []", "  states: [a]"),
	})
	res := lintRepo(t, root)
	if hasRule(res.Violations, "graph-lifecycle-states") {
		t.Fatalf("valid states after other lifecycle keys: %+v", res.Violations)
	}
}

func TestLint_DependsOnClosureDiamond(t *testing.T) {
	// o -> [a, b], a -> [b]: BFS revisits b (seen branch); relationship owned
	// by o with an endpoint in c, reachable only transitively o->a->c.
	root := repoWith(t, map[string]string{
		"spec/graph/modules/o/README.md":          fmModule("o", "[a, b]"),
		"spec/graph/modules/a/README.md":          fmModule("a", "[b, c]"),
		"spec/graph/modules/b/README.md":          fmModule("b", "[]"),
		"spec/graph/modules/c/README.md":          fmModule("c", "[]"),
		"spec/graph/modules/a/entities/x.md":      fmArt("entity", "x"),
		"spec/graph/modules/c/entities/y.md":      fmArt("entity", "y"),
		"spec/graph/modules/o/relationships/r.md": fmArt("relationship", "r", "from: a.x", "to: c.y"),
	})
	res := lintRepo(t, root)
	// Endpoint coverage passes via the closure; only dependency-direction on
	// the direct references may fire (o does not list c in dependsOn).
	if hasRule(res.Violations, "graph-relationship-owner-covers-endpoints") {
		t.Fatalf("closure should cover endpoints: %+v", res.Violations)
	}
}

func TestLinterRel_ErrorFallsBackToAbs(t *testing.T) {
	l := &linter{g: &Graph{RepoRoot: "relative-root"}}
	if got := l.rel("/absolute/path"); got != "/absolute/path" {
		t.Fatalf("rel error fallback: %q", got)
	}
}

func TestLint_MetadataQualifiedRefs(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":          fmModule("m", "[]"),
		"spec/graph/modules/m/entities/a.md":      fmArt("entity", "a"),
		"spec/graph/modules/m/entities/b.md":      fmArt("entity", "b"),
		"spec/graph/modules/m/relationships/r.md": fmArt("relationship", "r", "from: m.a", "to: m.b", "metadata:", "  good: m.a", "  bad: m.ghost", "  plain: just-a-string"),
	})
	res := lintRepo(t, root)
	c := ruleCounts(res.Violations)
	if c["graph-reference-resolves"] != 1 {
		t.Fatalf("expected exactly one unresolved metadata ref: %+v", res.Violations)
	}
}

func TestLint_InputsShapeVariants(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":      fmModule("m", "[]"),
		"spec/graph/modules/m/entities/a.md":  fmArt("entity", "a"),
		"spec/graph/modules/m/commands/c1.md": fmArt("command", "c1", "inputs: not-a-list"),
		"spec/graph/modules/m/commands/c2.md": fmArt("command", "c2",
			"inputs:",
			"  - ref: m.a",      // missing name (unnamed label)
			"  - name: BadName", // non-kebab + neither ref nor model
			"  - name: dup",     // ok shape
			"    ref: m.a",
			"  - name: dup", // duplicate name
			"    ref: m.a",
			"  - name: extras", // extra keys
			"    ref: m.a",
			"    fields: [x]",
			"  - name: modelled", // model-only input resolving
			"    model: modelspec:///m.A"),
		"spec/graph/modules/m/models/m.hcl": "entity \"A\" {}\n",
	})
	res := lintRepo(t, root)
	c := ruleCounts(res.Violations)
	// c1 malformed container; c2: missing name, non-kebab, missing ref/model,
	// duplicate, extra keys.
	if c["graph-inputs-shape"] < 5 {
		t.Fatalf("expected >=5 inputs-shape violations, got %d: %+v", c["graph-inputs-shape"], res.Violations)
	}
}

func TestLint_MalformedGraphAndModelspecRefs(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
		"spec/graph/modules/m/commands/c.md": fmArt("command", "c", "subject: nodot"),
		"spec/graph/modules/m/entities/e.md": fmArt("entity", "e", "model: modelspec:///nodotname"),
	})
	res := lintRepo(t, root)
	c := ruleCounts(res.Violations)
	if c["graph-reference-resolves"] != 1 || c["graph-model-ref-resolves"] != 1 {
		t.Fatalf("expected malformed-ref violations: %+v", res.Violations)
	}
}

func TestLint_ModelRefSuffixUnavailable(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
		"spec/graph/modules/m/entities/e.md": fmArt("entity", "e", "model: modelspec://example.com/acme/gone/shared.Thing"),
	})
	res := lintRepo(t, root)
	found := false
	for _, v := range res.Violations {
		if v.Rule == "graph-model-ref-resolves" && strings.Contains(v.Message, "repository") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected repository-not-available diagnostic: %+v", res.Violations)
	}
}

func TestLint_HCLQualifiedUnknownConcept(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":    fmModule("m", "[n]"),
		"spec/graph/modules/n/README.md":    fmModule("n", "[]"),
		"spec/graph/modules/n/models/n.hcl": "entity \"Real\" {}\n",
		"spec/graph/modules/m/models/m.hcl": "entity \"A\" {\n  property \"p\" {\n    entity = \"n.Ghost\"\n  }\n}\n",
	})
	res := lintRepo(t, root)
	found := false
	for _, v := range res.Violations {
		if v.Rule == "graph-model-ref-resolves" && strings.Contains(v.Message, "unknown concept") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unknown-concept diagnostic: %+v", res.Violations)
	}
}

func TestLint_HCLQualifiedAmbiguousModule(t *testing.T) {
	base := t.TempDir()
	for _, p := range []string{"p1", "p2"} {
		writeFile(t, filepath.Join(base, p, "specscore.yaml"), testConfig)
		writeFile(t, filepath.Join(base, p, "spec/graph/modules/shared/README.md"), fmModule("shared", "[]"))
		writeFile(t, filepath.Join(base, p, "spec/graph/modules/shared/models/m.hcl"), "entity \"Thing\" {}\n")
	}
	consumer := filepath.Join(base, "c")
	writeFile(t, filepath.Join(consumer, "specscore.yaml"),
		"# SpecScore Repo Config Schema: https://specscore.md/repo-config\n\nprojects:\n  - ../p1\n  - ../p2\n")
	writeFile(t, filepath.Join(consumer, "spec/graph/modules/m/README.md"), fmModule("m", "[shared]"))
	writeFile(t, filepath.Join(consumer, "spec/graph/modules/m/models/m.hcl"),
		"entity \"A\" {\n  property \"p\" {\n    entity = \"shared.Thing\"\n  }\n}\n")
	res, err := Lint(LintOptions{RepoRoot: consumer})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, v := range res.Violations {
		if v.Rule == "graph-model-ref-resolves" && strings.Contains(v.Message, "ambiguous") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected ambiguous-module diagnostic: %+v", res.Violations)
	}
}

func TestModel_NonPropertyNestedBlocksIgnored(t *testing.T) {
	m := loadModelSrc(t, "entity \"A\" {\n  index \"i\" {\n    unique = true\n  }\n\n  property \"p\" {\n    type = \"uuid\"\n  }\n}\n")
	if !m.HasConcept("A") {
		t.Fatal("entity missing")
	}
	for _, r := range m.Refs {
		t.Fatalf("index block should contribute no refs: %+v", r)
	}
}

func TestList_TieBreakOnPathForDuplicateIDs(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
		"spec/graph/modules/m/entities/x.md": fmArt("entity", "x"),
		"spec/graph/modules/m/commands/x.md": fmArt("command", "x"),
	})
	g, _, _ := Load(root, "")
	items := List(g, "", "")
	var dups []ListItem
	for _, it := range items {
		if it.ID == "m.x" {
			dups = append(dups, it)
		}
	}
	if len(dups) != 2 || dups[0].Path > dups[1].Path {
		t.Fatalf("expected two m.x rows sorted by path: %+v", dups)
	}
}

func TestNavigation_SkipsIDlessArtifacts(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":        fmModule("m", "[]"),
		"spec/graph/modules/m/entities/ok.md":   fmArt("entity", "ok"),
		"spec/graph/modules/m/entities/noid.md": "---\nkind: entity\nname: NoID\n---\n",
	})
	g, _, _ := Load(root, "")
	if _, _, found := ResolveRef(g, "NoID"); found {
		t.Fatal("id-less artifact should not resolve")
	}
	if r := Refs(g, "m.ok", false); len(r) != 0 {
		t.Fatalf("unexpected refs: %+v", r)
	}
}

func TestResolver_ProjectModuleWithoutModels(t *testing.T) {
	base := t.TempDir()
	prov := filepath.Join(base, "prov")
	writeFile(t, filepath.Join(prov, "specscore.yaml"), testConfig)
	// One module without models, one with.
	writeFile(t, filepath.Join(prov, "spec/graph/modules/nomodels/README.md"), fmModule("nomodels", "[]"))
	writeFile(t, filepath.Join(prov, "spec/graph/modules/withmodels/README.md"), fmModule("withmodels", "[]"))
	writeFile(t, filepath.Join(prov, "spec/graph/modules/withmodels/models/m.hcl"), "entity \"T\" {}\n")

	consumer := filepath.Join(base, "c")
	writeFile(t, filepath.Join(consumer, "specscore.yaml"),
		"# SpecScore Repo Config Schema: https://specscore.md/repo-config\n\nprojects:\n  - ../prov\n")
	writeFile(t, filepath.Join(consumer, "spec/graph/modules/m/README.md"), fmModule("m", "[withmodels]"))
	g, _, err := Load(consumer, "")
	if err != nil {
		t.Fatal(err)
	}
	r := BuildResolver(consumer, g)
	if got := r.resolveConcept("withmodels", "T", "", "").outcome; got != resResolved {
		t.Fatalf("withmodels: %v", got)
	}
	if got := r.resolveConcept("nomodels", "T", "", "").outcome; got != resUnknownModule {
		t.Fatalf("nomodels should be unknown (no models): %v", got)
	}
}

func TestScaffold_ArtifactLoadError(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules": "a file, not a dir",
	})
	_, err := Scaffold(ScaffoldOptions{Kind: KindEntity, Name: "X", Module: "m", RepoRoot: root})
	if code := exitOf(t, err); code != 10 {
		t.Fatalf("want 10 on load error, got %d", code)
	}
}

func TestScaffold_ArtifactWriteError(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md": fmModule("m", "[]"),
		// entities exists as a FILE so MkdirAll fails inside writeAll.
		"spec/graph/modules/m/entities": "a file",
	})
	_, err := Scaffold(ScaffoldOptions{Kind: KindEntity, Name: "X", Module: "m", RepoRoot: root})
	if code := exitOf(t, err); code != 10 {
		t.Fatalf("want 10 on write error, got %d", code)
	}
}

func TestScaffold_WriteFileError(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md": fmModule("m", "[]"),
	})
	// A read-only entities directory: MkdirAll succeeds, WriteFile fails.
	dir := filepath.Join(root, "spec/graph/modules/m/entities")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	_, err := Scaffold(ScaffoldOptions{Kind: KindEntity, Name: "X", Module: "m", RepoRoot: root})
	if code := exitOf(t, err); code != 10 {
		t.Fatalf("want 10 on WriteFile error, got %d", code)
	}
}

func TestScaffold_IDOnlyNoName(t *testing.T) {
	root := repoWith(t, map[string]string{})
	// Module with only an --id: name falls back to id in content.
	if _, err := Scaffold(ScaffoldOptions{Kind: KindModule, ID: "core", RepoRoot: root, Summary: "Core."}); err != nil {
		t.Fatal(err)
	}
	if _, err := Scaffold(ScaffoldOptions{Kind: KindEntity, ID: "thing", Module: "core", RepoRoot: root}); err != nil {
		t.Fatal(err)
	}
	if v := lintRepo(t, root).Violations; len(v) != 0 {
		t.Fatalf("id-only scaffold should lint clean: %+v", v)
	}
}
