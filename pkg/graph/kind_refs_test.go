package graph

import (
	"strings"
	"testing"
)

// TestLint_LegacyModelspecForm proves the legacy authority-empty-path form is
// reported under its own rule and carries the exact triple-slash rewrite.
func TestLint_LegacyModelspecForm(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
		"spec/graph/modules/m/entities/e.md": fmArt("entity", "e", "model: modelspec://m.A"),
		"spec/graph/modules/m/models/m.hcl":  "entity \"A\" {}\n",
	})
	res := lintRepo(t, root)
	var msg string
	for _, v := range res.Violations {
		if v.Rule == "graph-model-legacy-form" {
			msg = v.Message
		}
	}
	if msg == "" || !strings.Contains(msg, "modelspec:///m.A") {
		t.Fatalf("expected legacy-form violation carrying the rewrite: %+v", res.Violations)
	}
	if hasRule(res.Violations, "graph-model-ref-resolves") {
		t.Fatalf("legacy form must not also report a resolution error: %+v", res.Violations)
	}
}

// TestLint_KindExplicitResolution exercises the three-segment forms: exact-kind
// hits (records and the earlier entities, in either spelling of the model), a
// kind mismatch, and the removed kind segments.
func TestLint_KindExplicitResolution(t *testing.T) {
	for name, hcl := range map[string]string{
		"earlier": "entity \"A\" {}\n",
		"current": "record \"A\" {}\n",
	} {
		t.Run(name, func(t *testing.T) {
			mk := func(id, ref string) (string, string) {
				return "spec/graph/modules/m/entities/" + id + ".md", fmArt("entity", id, "model: "+ref)
			}
			files := map[string]string{
				"spec/graph/modules/m/README.md":    fmModule("m", "[]"),
				"spec/graph/modules/m/models/m.hcl": hcl,
			}
			for id, ref := range map[string]string{
				"e-entities": "modelspec:///m.entities.A",
				"e-records":  "modelspec:///m.records.A",
				"e-twoseg":   "modelspec:///m.A",
				"e-mismatch": "modelspec:///m.enums.A",
				"e-removed":  "modelspec:///m.collections.A",
			} {
				p, v := mk(id, ref)
				files[p] = v
			}
			res := lintRepo(t, root(t, files))
			got := map[string]string{}
			for _, vi := range res.Violations {
				if vi.Rule == "graph-model-ref-resolves" {
					got[vi.File] = vi.Message
				}
			}
			for _, id := range []string{"e-entities", "e-records", "e-twoseg"} {
				if msg, bad := got["spec/graph/modules/m/entities/"+id+".md"]; bad {
					t.Errorf("%s must resolve, got %q", id, msg)
				}
			}
			if msg := got["spec/graph/modules/m/entities/e-mismatch.md"]; !strings.Contains(msg, "is an entity, not an enum") {
				t.Errorf("expected kind-mismatch diagnostic, got %q", msg)
			}
			if msg := got["spec/graph/modules/m/entities/e-removed.md"]; !strings.Contains(msg, `unknown kind segment "collections"`) {
				t.Errorf("expected removed kind segment to be refused, got %q", msg)
			}
		})
	}
}

// TestLint_ReservedConceptName flags concepts (any kind) named with a reserved
// kind token, including records, in either spelling of the block.
func TestLint_ReservedConceptName(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":    fmModule("m", "[]"),
		"spec/graph/modules/m/models/m.hcl": "entity \"entities\" {}\nrecord \"records\" {}\ncomponent \"collections\" {}\nenum \"recordsets\" { values = [\"x\"] }\nrecord \"components\" {}\nenum \"enums\" { values = [\"x\"] }\n",
	})
	res := lintRepo(t, root)
	if got := ruleCounts(res.Violations)["graph-model-reserved-name"]; got != 6 {
		t.Fatalf("expected six reserved-name violations, got %d: %+v", got, res.Violations)
	}
	for _, v := range res.Violations {
		if v.Rule == "graph-model-reserved-name" && !strings.Contains(v.Message, "records,") {
			t.Fatalf("message must list records among the reserved tokens: %+v", v)
		}
	}
}

// TestLint_DuplicateConceptScope proves record types (in either spelling),
// components, and enums share one scope, so a same-named pair collides.
func TestLint_DuplicateConceptScope(t *testing.T) {
	hcl := "entity \"A\" {}\nenum \"A\" { values = [\"x\"] }\n" +
		"record \"B\" {}\nentity \"B\" {}\n" +
		"record \"C\" {}\ncomponent \"C\" {}\n"
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":    fmModule("m", "[]"),
		"spec/graph/modules/m/models/m.hcl": hcl,
	})
	res := lintRepo(t, root)
	if got := ruleCounts(res.Violations)["graph-model-duplicate-concept"]; got != 3 {
		t.Fatalf("expected 3 duplicate-concept violations, got %d: %+v", got, res.Violations)
	}
}

// TestLint_ModuleAndEntitySameName proves a module and a same-named entity
// coexist cleanly (decision 0011: modules are bare-ID citizens, not qualified
// concepts). Uses a neutral `catalog` module with a `catalog` entity.
func TestLint_ModuleAndEntitySameName(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/catalog/README.md":           fmModule("catalog", "[]"),
		"spec/graph/modules/catalog/entities/catalog.md": fmArt("entity", "catalog", "model: modelspec:///catalog.Catalog"),
		"spec/graph/modules/catalog/models/catalog.hcl":  "entity \"Catalog\" {}\n",
	})
	res := lintRepo(t, root)
	if hasRule(res.Violations, "graph-duplicate-id") {
		t.Fatalf("module and same-named entity must not collide: %+v", res.Violations)
	}
	if vs := withoutAdvisory(res.Violations); len(vs) != 0 {
		t.Fatalf("expected a clean lint, got: %+v", vs)
	}
}

// root is a fmt-free alias for repoWith usable inline.
func root(t *testing.T, files map[string]string) string {
	t.Helper()
	return repoWith(t, files)
}
