package graph

import "testing"

func loadValid(t *testing.T) *Graph {
	t.Helper()
	g, ok, err := Load("testdata/valid", "")
	if err != nil || !ok {
		t.Fatalf("load valid: ok=%v err=%v", ok, err)
	}
	return g
}

func TestList_All(t *testing.T) {
	g := loadValid(t)
	all := List(g, "", "")
	if len(all) == 0 {
		t.Fatal("expected items")
	}
	// includes modules and collection artifacts, sorted by id.
	for i := 1; i < len(all); i++ {
		if all[i-1].ID > all[i].ID {
			t.Fatalf("not sorted: %q > %q", all[i-1].ID, all[i].ID)
		}
	}
}

func TestList_Filters(t *testing.T) {
	g := loadValid(t)
	entities := List(g, "entity", "")
	if len(entities) == 0 {
		t.Fatal("no entities")
	}
	for _, it := range entities {
		if it.Kind != "entity" {
			t.Fatalf("kind filter leaked %q", it.Kind)
		}
	}
	res := List(g, "", "reservations")
	if len(res) == 0 {
		t.Fatal("no reservations items")
	}
	for _, it := range res {
		if it.ID != "reservations" && it.Kind != "module" {
			// module row's ID is the bare module id
		}
	}
	mods := List(g, "module", "")
	found := false
	for _, it := range mods {
		if it.ID == "scheduling" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected scheduling module listed")
	}
}

func TestResolveRef(t *testing.T) {
	g := loadValid(t)
	// qualified id.
	if qid, cand, ok := ResolveRef(g, "reservations.booking"); !ok || len(cand) != 0 || qid != "reservations.booking" {
		t.Fatalf("qualified resolve: %q %v %v", qid, cand, ok)
	}
	// shorthand bare id (unique).
	if qid, _, ok := ResolveRef(g, "booking"); !ok || qid != "reservations.booking" {
		t.Fatalf("shorthand resolve: %q %v", qid, ok)
	}
	// shorthand by display name.
	if qid, _, ok := ResolveRef(g, "Contact"); !ok || qid != "directory.contact" {
		t.Fatalf("name resolve: %q %v", qid, ok)
	}
	// not found.
	if _, _, ok := ResolveRef(g, "does-not-exist"); ok {
		t.Fatal("expected not found")
	}
}

func TestResolveRef_Ambiguous(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/a/README.md":     fmModule("a", "[]"),
		"spec/graph/modules/b/README.md":     fmModule("b", "[]"),
		"spec/graph/modules/a/entities/x.md": fmArt("entity", "x"),
		"spec/graph/modules/b/entities/x.md": fmArt("entity", "x"),
	})
	g, _, _ := Load(root, "")
	qid, cand, ok := ResolveRef(g, "x")
	if !ok || qid != "" || len(cand) != 2 {
		t.Fatalf("expected ambiguous, got qid=%q cand=%v ok=%v", qid, cand, ok)
	}
}

func TestRefs_DirectAndTransitive(t *testing.T) {
	g := loadValid(t)
	direct := Refs(g, "reservations.booking", false)
	if len(direct) == 0 {
		t.Fatal("expected inbound refs to booking")
	}
	// create-booking references booking (subject), so must appear.
	var haveCreate bool
	for _, it := range direct {
		if it.ID == "reservations.create-booking" {
			haveCreate = true
		}
	}
	if !haveCreate {
		t.Fatalf("expected create-booking among referrers: %+v", direct)
	}
	// transitive terminates and includes the direct referrers.
	trans := Refs(g, "reservations.booking", true)
	if len(trans) < len(direct) {
		t.Fatalf("transitive should include direct: %d < %d", len(trans), len(direct))
	}
	// asset has an inbound relationship + event + command input.
	assetRefs := Refs(g, "catalog.asset", true)
	if len(assetRefs) == 0 {
		t.Fatal("expected refs to catalog.asset")
	}
}

func TestRefs_Empty(t *testing.T) {
	g := loadValid(t)
	if r := Refs(g, "nobody.references-this", false); len(r) != 0 {
		t.Fatalf("expected no refs, got %+v", r)
	}
}
