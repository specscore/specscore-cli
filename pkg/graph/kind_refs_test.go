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
// hits (trio, collection, recordset), a kind mismatch, and the two-segment form
// failing to reach a collection.
func TestLint_KindExplicitResolution(t *testing.T) {
	hcl := "entity \"A\" {}\ncollection \"Cats\" {}\nrecordset \"Daily\" {}\n"
	mk := func(id, ref string) (string, string) {
		return "spec/graph/modules/m/entities/" + id + ".md", fmArt("entity", id, "model: "+ref)
	}
	files := map[string]string{
		"spec/graph/modules/m/README.md":    fmModule("m", "[]"),
		"spec/graph/modules/m/models/m.hcl": hcl,
	}
	okEntity, v := mk("e-entity", "modelspec:///m.entities.A")
	files[okEntity] = v
	okColl, v := mk("e-coll", "modelspec:///m.collections.Cats")
	files[okColl] = v
	okRec, v := mk("e-rec", "modelspec:///m.recordsets.Daily")
	files[okRec] = v
	mismatch, v := mk("e-mismatch", "modelspec:///m.enums.A")
	files[mismatch] = v
	twoSegColl, v := mk("e-twoseg", "modelspec:///m.Cats")
	files[twoSegColl] = v

	res := lintRepo(t, root(t, files))
	var mismatchMsg, twosegMsg string
	for _, vi := range res.Violations {
		if vi.Rule != "graph-model-ref-resolves" {
			continue
		}
		if strings.Contains(vi.Message, "not an enum") {
			mismatchMsg = vi.Message
		}
	}
	// The mismatch and two-seg-collection failures both surface as
	// graph-model-ref-resolves; confirm the mismatch message names the actual
	// kind and the two-seg form fails as unknown concept.
	for _, vi := range res.Violations {
		if vi.Rule == "graph-model-ref-resolves" && strings.Contains(vi.Message, "\"Cats\"") &&
			strings.Contains(vi.Message, "unknown concept") {
			twosegMsg = vi.Message
		}
	}
	if mismatchMsg == "" || !strings.Contains(mismatchMsg, "is an entity, not an enum") {
		t.Fatalf("expected kind-mismatch diagnostic: %+v", res.Violations)
	}
	if twosegMsg == "" {
		t.Fatalf("two-segment form must not reach a collection: %+v", res.Violations)
	}
}

// TestLint_ReservedConceptName flags concepts (any kind) named with a reserved
// kind token.
func TestLint_ReservedConceptName(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":    fmModule("m", "[]"),
		"spec/graph/modules/m/models/m.hcl": "entity \"entities\" {}\ncollection \"collections\" {}\n",
	})
	res := lintRepo(t, root)
	if ruleCounts(res.Violations)["graph-model-reserved-name"] != 2 {
		t.Fatalf("expected two reserved-name violations: %+v", res.Violations)
	}
}

// TestLint_DuplicateConceptScopes proves the trio shares one scope while
// collections and recordsets have their own, so a collection and a same-named
// entity coexist but two same-named collections (or recordsets, or two trio
// members) collide.
func TestLint_DuplicateConceptScopes(t *testing.T) {
	// entity A + enum A: same trio scope -> duplicate. entity Shared +
	// collection Shared: different scopes -> no duplicate. Two collections C
	// and two recordsets R: each collides within its own scope.
	hcl := "entity \"A\" {}\nenum \"A\" { values = [\"x\"] }\n" +
		"entity \"Shared\" {}\ncollection \"Shared\" {}\n" +
		"collection \"C\" {}\ncollection \"C\" {}\n" +
		"recordset \"R\" {}\nrecordset \"R\" {}\n"
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":    fmModule("m", "[]"),
		"spec/graph/modules/m/models/m.hcl": hcl,
	})
	res := lintRepo(t, root)
	// One duplicate per colliding scope: trio (A), collections (C), recordsets
	// (R) = 3. The entity/collection "Shared" pair must NOT collide.
	if got := ruleCounts(res.Violations)["graph-model-duplicate-concept"]; got != 3 {
		t.Fatalf("expected 3 duplicate-concept violations (trio, collections, recordsets), got %d: %+v", got, res.Violations)
	}
	for _, v := range res.Violations {
		if v.Rule == "graph-model-duplicate-concept" && strings.Contains(v.Message, "\"Shared\"") {
			t.Fatalf("entity and collection named Shared must not collide: %+v", v)
		}
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
	if len(res.Violations) != 0 {
		t.Fatalf("expected a clean lint, got: %+v", res.Violations)
	}
}

// root is a fmt-free alias for repoWith usable inline.
func root(t *testing.T, files map[string]string) string {
	t.Helper()
	return repoWith(t, files)
}
