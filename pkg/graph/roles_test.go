package graph

import (
	"strings"
	"testing"
)

// fmRel builds a relationship artifact with raw frontmatter lines.
func fmRel(id string, extra ...string) string {
	return fmArt("relationship", id, extra...)
}

// TestRoles_LabeledEndpointsAndParticipantsParse covers the decision-0012 map
// forms end to end: a role-labeled self-referential relationship and an event
// with two same-ref participants distinguished by roles lint clean.
func TestRoles_LabeledEndpointsAndParticipantsParse(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
		"spec/graph/modules/m/entities/c.md": fmArt("entity", "c"),
		"spec/graph/modules/m/relationships/parent-of.md": fmRel("parent-of",
			"from: {ref: m.c, role: parent}",
			"to: {ref: m.c, role: child}",
			"cardinality: many-to-many"),
		"spec/graph/modules/m/commands/do.md": fmArt("command", "do", "subject: m.c", "possibleEvents:", "  - m.happened"),
		"spec/graph/modules/m/events/happened.md": fmArt("event", "happened",
			"subject: m.c",
			"participants:",
			"  - {ref: m.c, role: guardian}",
			"  - {ref: m.c, role: ward}"),
	})
	res := lintRepo(t, root)
	if len(res.Violations) != 0 {
		t.Fatalf("labeled forms should lint clean: %+v", res.Violations)
	}
	// The parsed views carry refs and roles.
	g, _, err := Load(root, "")
	if err != nil {
		t.Fatal(err)
	}
	var rel, ev *Artifact
	for _, m := range g.Modules {
		for _, a := range m.Artifacts {
			switch a.ID {
			case "parent-of":
				rel = a
			case "happened":
				ev = a
			}
		}
	}
	if rel.From != "m.c" || rel.FromRole != "parent" || rel.To != "m.c" || rel.ToRole != "child" {
		t.Fatalf("endpoint roles: %+v", rel)
	}
	if len(ev.Participants) != 2 || ev.Participants[0] != "m.c" ||
		len(ev.ParticipantItems) != 2 || ev.ParticipantItems[0].Role != "guardian" || ev.ParticipantItems[1].Role != "ward" {
		t.Fatalf("participant roles: %+v %+v", ev.Participants, ev.ParticipantItems)
	}
}

// TestRoles_ShapeViolations exercises every graph-role-labels malformation.
func TestRoles_ShapeViolations(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
		"spec/graph/modules/m/entities/c.md": fmArt("entity", "c"),
		// from: sequence (neither scalar nor map); to: map missing ref;
		"spec/graph/modules/m/relationships/r1.md": fmRel("r1",
			"from: [m.c]",
			"to: {role: child}",
			"cardinality: many-to-many"),
		// from: extra key; to: missing role; plus a non-kebab role.
		"spec/graph/modules/m/relationships/r2.md": fmRel("r2",
			"from: {ref: m.c, role: parent, note: extra}",
			"to: {ref: m.c}",
			"cardinality: many-to-many"),
		"spec/graph/modules/m/relationships/r3.md": fmRel("r3",
			"from: {ref: m.c, role: Parent}",
			"to: {ref: m.c, role: child}",
			"cardinality: many-to-many"),
		// participants: malformed item (sequence) + map missing ref.
		"spec/graph/modules/m/events/e1.md": fmArt("event", "e1",
			"subject: m.c",
			"sources: [automation]",
			"participants:",
			"  - [m.c]",
			"  - {role: ward}"),
		// participants: not a list at all — tolerated as absent (no items).
		"spec/graph/modules/m/events/e2.md": fmArt("event", "e2",
			"subject: m.c",
			"sources: [automation]",
			"participants: m.c"),
	})
	res := lintRepo(t, root)
	c := ruleCounts(res.Violations)
	// r1: from not scalar/map + to missing ref = 2
	// r2: extra key + to missing role = 2
	// r3: non-kebab role = 1
	// e1: malformed item + missing ref = 2
	if c["graph-role-labels"] != 7 {
		t.Fatalf("expected 7 role-label violations, got %d: %+v", c["graph-role-labels"], res.Violations)
	}
	for _, want := range []string{
		"must be a qualified reference or a {ref, role} map",
		"unexpected key \"note\"",
		"must carry a non-empty `ref`",
		"must carry a non-empty `role`",
		"must be a bare lowercase kebab-case token",
	} {
		found := false
		for _, v := range res.Violations {
			if v.Rule == "graph-role-labels" && strings.Contains(v.Message, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("no violation mentions %q: %+v", want, res.Violations)
		}
	}
}

// TestRoles_AmbiguousEndpoints covers the decision-0012 warnings: unlabeled
// self-referential endpoints and duplicate same-ref participants.
func TestRoles_AmbiguousEndpoints(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
		"spec/graph/modules/m/entities/c.md": fmArt("entity", "c"),
		// Unlabeled self-reference: warn.
		"spec/graph/modules/m/relationships/kin.md": fmRel("kin",
			"from: m.c", "to: m.c", "cardinality: many-to-many"),
		// Duplicate roleless participants + duplicate same-role participants: warn each.
		"spec/graph/modules/m/events/e1.md": fmArt("event", "e1",
			"subject: m.c",
			"sources: [automation]",
			"participants:",
			"  - m.c",
			"  - m.c",
			"  - {ref: m.c, role: ward}",
			"  - {ref: m.c, role: ward}"),
	})
	res := lintRepo(t, root)
	if got := ruleCounts(res.Violations)["graph-ambiguous-endpoints"]; got != 3 {
		t.Fatalf("expected 3 ambiguous-endpoint warnings, got %d: %+v", got, res.Violations)
	}
	// Distinguishing roles silence the duplicate warning; a labeled
	// self-reference is clean.
	root2 := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
		"spec/graph/modules/m/entities/c.md": fmArt("entity", "c"),
		"spec/graph/modules/m/relationships/kin.md": fmRel("kin",
			"from: {ref: m.c, role: parent}", "to: {ref: m.c, role: child}", "cardinality: many-to-many"),
		"spec/graph/modules/m/events/e1.md": fmArt("event", "e1",
			"subject: m.c",
			"sources: [automation]",
			"participants:",
			"  - {ref: m.c, role: guardian}",
			"  - {ref: m.c, role: ward}"),
	})
	res2 := lintRepo(t, root2)
	if hasRule(res2.Violations, "graph-ambiguous-endpoints") {
		t.Fatalf("distinguished roles must not warn: %+v", res2.Violations)
	}
}

// TestLint_UnknownKeys warns on kind-inapplicable frontmatter keys.
func TestLint_UnknownKeys(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
		"spec/graph/modules/m/entities/a.md": fmArt("entity", "a"),
		"spec/graph/modules/m/entities/b.md": fmArt("entity", "b"),
		// lifecycle on a relationship is silently meaningless (Phase 5A probe).
		"spec/graph/modules/m/relationships/r.md": fmRel("r",
			"from: m.a", "to: m.b", "cardinality: many-to-one",
			"lifecycle:", "  states: [active]"),
		// owner/fields are covered by dedicated rules, not unknown-key.
		"spec/graph/modules/m/entities/c.md": fmArt("entity", "c", "owner: alice", "custom: x"),
	})
	res := lintRepo(t, root)
	var msgs []string
	for _, v := range res.Violations {
		if v.Rule == "graph-unknown-key" {
			msgs = append(msgs, v.Message)
		}
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 unknown-key warnings, got %v (all: %+v)", msgs, res.Violations)
	}
	if !strings.Contains(strings.Join(msgs, " "), "\"lifecycle\"") || !strings.Contains(strings.Join(msgs, " "), "\"custom\"") {
		t.Fatalf("unexpected unknown-key messages: %v", msgs)
	}
}

// TestLint_EventReachability reports non-draft events nothing can produce.
func TestLint_EventReachability(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
		"spec/graph/modules/m/entities/c.md": fmArt("entity", "c"),
		// Unreachable, non-draft: reported.
		"spec/graph/modules/m/events/orphan.md": strings.Replace(
			fmArt("event", "orphan", "subject: m.c"), "status: draft", "status: approved", 1),
		// Unreachable but draft: exempt.
		"spec/graph/modules/m/events/young.md": fmArt("event", "young", "subject: m.c"),
		// Fed by sources: clean.
		"spec/graph/modules/m/events/timed.md": strings.Replace(
			fmArt("event", "timed", "subject: m.c", "sources: [automation]"), "status: draft", "status: approved", 1),
		// Fed by a command: clean.
		"spec/graph/modules/m/events/done.md": strings.Replace(
			fmArt("event", "done", "subject: m.c"), "status: draft", "status: approved", 1),
		"spec/graph/modules/m/commands/do.md": fmArt("command", "do", "subject: m.c", "possibleEvents:", "  - m.done"),
	})
	res := lintRepo(t, root)
	var hits []string
	for _, v := range res.Violations {
		if v.Rule == "graph-event-reachability" {
			hits = append(hits, v.Message)
		}
	}
	if len(hits) != 1 || !strings.Contains(hits[0], "m.orphan") {
		t.Fatalf("expected exactly the orphan event, got %v (all: %+v)", hits, res.Violations)
	}
	if res.Violations[0].Severity == "" {
		t.Fatal("severity must be set")
	}
}

// TestWithArticle covers the indefinite-article helper.
func TestWithArticle(t *testing.T) {
	cases := map[string]string{
		"entity": "an entity", "enum": "an enum",
		"component": "a component", "collection": "a collection", "recordset": "a recordset",
	}
	for in, want := range cases {
		if got := withArticle(in); got != want {
			t.Errorf("withArticle(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestRefs_ModelDerivedEdges proves association-object visibility: an entity
// whose model references another entity's concept shows up in `graph refs`
// for that entity.
func TestRefs_ModelDerivedEdges(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md": fmModule("m", "[]"),
		"spec/graph/modules/m/models/m.hcl": `
entity "Contact" {}
entity "Guardianship" {
  property "guardian" {
    entity = "Contact"
  }
  property "self" {
    entity = "Guardianship"
  }
  property "ghost" {
    entity = "Unmapped"
  }
}
entity "Unmapped" {}
component "Loose" {
  field "x" {
    entity = "Contact"
  }
}
`,
		"spec/graph/modules/m/entities/contact.md":      fmArt("entity", "contact", "model: modelspec:///m.Contact"),
		"spec/graph/modules/m/entities/guardianship.md": fmArt("entity", "guardianship", "model: modelspec:///m.Guardianship"),
		// Cross-repo model ref: skipped by edge derivation.
		"spec/graph/modules/m/entities/remote.md": fmArt("entity", "remote", "model: modelspec://example.com/acme/gone/m.Contact"),
	})
	g, ok, err := Load(root, "")
	if err != nil || !ok {
		t.Fatal(err)
	}
	items := Refs(g, "m.contact", false)
	found := false
	for _, it := range items {
		if it.ID == "m.guardianship" {
			found = true
		}
	}
	if !found {
		t.Fatalf("model-derived edge missing: %+v", items)
	}
	// The self-edge and unmapped-concept refs derive nothing.
	if items2 := Refs(g, "m.guardianship", false); len(items2) != 0 {
		t.Fatalf("no inbound refs expected for guardianship: %+v", items2)
	}
	// The lint pass stays clean apart from the expected unavailable-repo error
	// on the cross-repo fixture.
	res := lintRepo(t, root)
	for _, v := range res.Violations {
		if v.Rule != "graph-model-ref-resolves" {
			t.Fatalf("unexpected violation: %+v", v)
		}
	}
}
