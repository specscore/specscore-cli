package graph

// Tests for decision 0013: Tier-1 rules: blocks (graph-rules-shape), the
// policy kind (graph-policy-shape), and enum-value fragments on modelspec://
// references. Fixtures use neutral domains (identity/catalog/scheduling).

import (
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/lint"
)

// policyBaseFiles returns the shared clean fixture: identity (account/team +
// membership-of + Account model with an `owner` property), catalog (item with
// a lifecycle + Item/Availability models), and a scheduling module that owns
// the commands and policies.
func policyBaseFiles() map[string]string {
	return map[string]string{
		"spec/graph/modules/identity/README.md": fmModule("identity", "[]"),
		"spec/graph/modules/identity/entities/account.md": fmArt("entity", "account",
			"model: modelspec:///identity.Account",
			"lifecycle:", "  states: [active, suspended]"),
		"spec/graph/modules/identity/entities/team.md": fmArt("entity", "team"),
		"spec/graph/modules/identity/relationships/membership-of.md": fmArt("relationship", "membership-of",
			"from: identity.account", "to: identity.team", "cardinality: many-to-many"),
		"spec/graph/modules/identity/models/identity.hcl": "entity \"Account\" {\n  property \"owner\" {\n    type = \"uuid\"\n  }\n}\n",
		"spec/graph/modules/catalog/README.md":            fmModule("catalog", "[]"),
		"spec/graph/modules/catalog/entities/item.md": fmArt("entity", "item",
			"lifecycle:", "  states: [draft, published]"),
		"spec/graph/modules/catalog/models/catalog.hcl": "entity \"Item\" {}\nenum \"Availability\" {\n  values = [\"in-stock\", \"out-of-stock\"]\n}\n",
		"spec/graph/modules/scheduling/README.md":       fmModule("scheduling", "[catalog, identity]"),
		"spec/graph/modules/scheduling/entities/booking.md": fmArt("entity", "booking",
			"lifecycle:", "  states: [pending, confirmed, cancelled]"),
		"spec/graph/modules/scheduling/commands/create-booking.md": fmArt("command", "create-booking",
			"subject: scheduling.booking",
			"inputs:", "  - name: subject", "    ref: catalog.item"),
	}
}

func policyRepo(t *testing.T, extra map[string]string) string {
	t.Helper()
	files := policyBaseFiles()
	for k, v := range extra {
		files[k] = v
	}
	return repoWith(t, files)
}

func messagesOf(vs []lint.Violation, rule string) []string {
	var out []string
	for _, v := range vs {
		if v.Rule == rule {
			out = append(out, v.Message)
		}
	}
	return out
}

func assertMentions(t *testing.T, msgs []string, wants ...string) {
	t.Helper()
	joined := strings.Join(msgs, "\n")
	for _, w := range wants {
		if !strings.Contains(joined, w) {
			t.Errorf("no violation mentions %q; got:\n%s", w, joined)
		}
	}
}

// TestRules_CleanBlock proves a well-formed rules: block on an entity, a
// relationship, and a command lints clean and parses into Artifact.Rules —
// including graph refs, modelspec refs, and enum-value fragments in refs.
func TestRules_CleanBlock(t *testing.T) {
	root := policyRepo(t, map[string]string{
		"spec/graph/modules/catalog/entities/item.md": fmArt("entity", "item",
			"lifecycle:", "  states: [draft, published]",
			"rules:",
			"  - id: unique-sku",
			"    text: Items must have unique SKUs.",
			"    refs:",
			"      - catalog.item",
			"      - modelspec:///catalog.Item",
			"      - modelspec:///catalog.Availability#in-stock",
			"  - id: no-refs",
			"    text: A rule may carry no refs."),
		"spec/graph/modules/identity/relationships/membership-of.md": fmArt("relationship", "membership-of",
			"from: identity.account", "to: identity.team", "cardinality: many-to-many",
			"rules:",
			"  - id: single-team",
			"    text: An account belongs to at most one team."),
		"spec/graph/modules/scheduling/commands/create-booking.md": fmArt("command", "create-booking",
			"subject: scheduling.booking",
			"inputs:", "  - name: subject", "    ref: catalog.item",
			"rules:",
			"  - id: item-published",
			"    text: The item must be published.",
			"    refs: [catalog.item]"),
	})
	res := lintRepo(t, root)
	if len(res.Violations) != 0 {
		t.Fatalf("clean rules blocks should lint clean: %+v", res.Violations)
	}
	g, _, err := Load(root, "")
	if err != nil {
		t.Fatal(err)
	}
	var item *Artifact
	for _, m := range g.Modules {
		for _, a := range m.Artifacts {
			if a.QualifiedID() == "catalog.item" {
				item = a
			}
		}
	}
	if len(item.Rules) != 2 || item.Rules[0].ID != "unique-sku" || len(item.Rules[0].Refs) != 3 ||
		item.Rules[1].Text != "A rule may carry no refs." {
		t.Fatalf("parsed rules: %+v", item.Rules)
	}
}

// TestRules_ShapeViolations exercises every graph-rules-shape malformation.
func TestRules_ShapeViolations(t *testing.T) {
	root := policyRepo(t, map[string]string{
		// Container is not a list.
		"spec/graph/modules/catalog/entities/scalar-rules.md": fmArt("entity", "scalar-rules",
			"rules: not-a-list"),
		// A non-map item plus every per-item malformation.
		"spec/graph/modules/catalog/entities/bad-items.md": fmArt("entity", "bad-items",
			"rules:",
			"  - just-a-string",
			"  - text: Missing id.",
			"  - id: BadCase",
			"    text: Non-kebab id.",
			"  - id: dup",
			"    text: First.",
			"  - id: dup",
			"    text: Second.",
			"  - id: no-text",
			"  - id: extras",
			"    text: Extra keys.",
			"    note: nope",
			"  - id: bad-refs",
			"    text: Refs is not a list.",
			"    refs: catalog.item",
			"  - id: nested-ref",
			"    text: Refs item is not a scalar.",
			"    refs:",
			"      - [catalog.item]"),
	})
	res := lintRepo(t, root)
	msgs := messagesOf(res.Violations, "graph-rules-shape")
	assertMentions(t, msgs,
		"rules must be a list of {id, text, refs} maps",
		"rule item is missing an `id`",
		"rule id \"BadCase\" must be bare lowercase kebab-case",
		"duplicate rule id \"dup\"",
		"rule \"no-text\" must carry a non-empty `text`",
		"rule \"extras\" has unexpected key(s): note",
		"rule \"bad-refs\" refs must be a list of qualified graph or modelspec:// references",
		"rule \"nested-ref\" refs must be a list of qualified graph or modelspec:// references",
	)
	// 9 = the scalar container + the non-map item (both report the container
	// message) + the 7 per-item malformations.
	if got := len(msgs); got != 9 {
		t.Fatalf("expected 9 rules-shape violations, got %d: %v", got, msgs)
	}
}

// TestRules_RefsResolution routes rules.refs entries through the existing
// resolution rules: graph refs, modelspec refs, dependency direction, and the
// decision-0013 enum-value fragment diagnostics.
func TestRules_RefsResolution(t *testing.T) {
	root := policyRepo(t, map[string]string{
		"spec/graph/modules/catalog/entities/ruled.md": fmArt("entity", "ruled",
			"rules:",
			"  - id: r1",
			"    text: Broken graph and model refs.",
			"    refs:",
			"      - catalog.ghost",
			"      - identity.account", // catalog does not depend on identity
			"      - modelspec:///catalog.Ghost",
			"      - modelspec:///catalog.Item#in-stock",              // fragment on a non-enum
			"      - modelspec:///catalog.Availability#gone",          // unknown enum value
			"      - modelspec:///catalog.Availability#out-of-stock"), // clean
	})
	res := lintRepo(t, root)
	c := ruleCounts(res.Violations)
	if c["graph-reference-resolves"] != 1 || c["graph-dependency-direction"] != 1 || c["graph-model-ref-resolves"] != 3 {
		t.Fatalf("unexpected counts %v: %+v", c, res.Violations)
	}
	assertMentions(t, messagesOf(res.Violations, "graph-model-ref-resolves"),
		"concept \"Item\" is an entity, not an enum — fragments address enum values (decision 0013)",
		"enum \"Availability\" has no value \"gone\"",
	)
}

// TestLint_FragmentOnModelKey covers enum-value fragments on the entity model:
// key path (checkModelspecRef is shared, so metadata/inputs come free).
func TestLint_FragmentOnModelKey(t *testing.T) {
	root := policyRepo(t, map[string]string{
		"spec/graph/modules/catalog/entities/frag.md": fmArt("entity", "frag",
			"model: modelspec:///catalog.Item#in-stock"),
	})
	res := lintRepo(t, root)
	msgs := messagesOf(res.Violations, "graph-model-ref-resolves")
	if len(msgs) != 1 || !strings.Contains(msgs[0], "not an enum") {
		t.Fatalf("expected one fragment-not-enum violation: %+v", res.Violations)
	}
}

// TestPolicy_CleanArtifacts proves the two canonical policies — a command
// policy with when/requires clauses and an entity policy with an invariant —
// lint clean, and the applies fields parse.
func TestPolicy_CleanArtifacts(t *testing.T) {
	root := policyRepo(t, map[string]string{
		"spec/graph/modules/scheduling/policies/consent-required.md": fmArt("policy", "consent-required",
			"applies:",
			"  command: scheduling.create-booking",
			"when:",
			"  - input: subject",
			"  - is-role: {relationship: identity.membership-of, role: member}",
			"requires:",
			"  - entity: catalog.item",
			"    in-state: published",
			"  - actor-is: {entity: identity.account, model-role: owner}"),
		"spec/graph/modules/scheduling/policies/cancel-on-suspend.md": fmArt("policy", "cancel-on-suspend",
			"applies:",
			"  entity: scheduling.booking",
			"invariant:",
			"  - when-referenced: {entity: identity.account, in-state: suspended}",
			"    then: {self-state: cancelled}"),
	})
	res := lintRepo(t, root)
	if len(res.Violations) != 0 {
		t.Fatalf("clean policies should lint clean: %+v", res.Violations)
	}
	g, _, err := Load(root, "")
	if err != nil {
		t.Fatal(err)
	}
	var pol *Artifact
	for _, m := range g.Modules {
		for _, a := range m.Artifacts {
			if a.QualifiedID() == "scheduling.consent-required" {
				pol = a
			}
		}
	}
	if pol.AppliesKind != "command" || pol.AppliesRef != "scheduling.create-booking" {
		t.Fatalf("applies fields: %+v", pol)
	}
	// The policy kind is a first-class list citizen.
	items := List(g, "policy", "")
	if len(items) != 2 || items[0].Kind != "policy" {
		t.Fatalf("policy list rows: %+v", items)
	}
}

// TestPolicy_AppliesViolations exercises the applies: block malformations.
func TestPolicy_AppliesViolations(t *testing.T) {
	root := policyRepo(t, map[string]string{
		// No applies at all.
		"spec/graph/modules/scheduling/policies/p1.md": fmArt("policy", "p1"),
		// applies is not a map.
		"spec/graph/modules/scheduling/policies/p2.md": fmArt("policy", "p2",
			"applies: scheduling.booking"),
		// Unexpected key and zero valid keys.
		"spec/graph/modules/scheduling/policies/p3.md": fmArt("policy", "p3",
			"applies:", "  target: scheduling.booking"),
		// Two valid keys.
		"spec/graph/modules/scheduling/policies/p4.md": fmArt("policy", "p4",
			"applies:", "  command: scheduling.create-booking", "  entity: scheduling.booking"),
		// Not a qualified reference.
		"spec/graph/modules/scheduling/policies/p5.md": fmArt("policy", "p5",
			"applies:", "  command: nodot"),
		// Resolves to the wrong kind.
		"spec/graph/modules/scheduling/policies/p6.md": fmArt("policy", "p6",
			"applies:", "  entity: scheduling.create-booking"),
		// Does not resolve at all.
		"spec/graph/modules/scheduling/policies/p7.md": fmArt("policy", "p7",
			"applies:", "  command: scheduling.ghost"),
	})
	res := lintRepo(t, root)
	msgs := messagesOf(res.Violations, "graph-policy-shape")
	assertMentions(t, msgs,
		"policy must declare an `applies:` block",
		"applies must be a map carrying exactly one of command|entity|relationship",
		"applies has unexpected key \"target\"",
		"applies must carry exactly one of command|entity|relationship",
		"applies.command reference \"nodot\" is not a valid <module>.<id> reference",
		"applies.entity reference \"scheduling.create-booking\" resolves to a command, not an entity",
		"applies.command reference \"scheduling.ghost\" does not resolve to an existing command",
	)
	if got := len(msgs); got != 8 { // p3 emits both the unexpected-key and the exactly-one violations
		t.Fatalf("expected 8 policy-shape violations, got %d: %v", got, msgs)
	}
}

// TestPolicy_WhenViolations exercises the when: clause malformations.
func TestPolicy_WhenViolations(t *testing.T) {
	root := policyRepo(t, map[string]string{
		// Container is not a list.
		"spec/graph/modules/scheduling/policies/w1.md": fmArt("policy", "w1",
			"applies:", "  command: scheduling.create-booking",
			"when: not-a-list"),
		// Item shapes: scalar item, extra key, both forms, neither form,
		// empty input, unknown input, is-role not a map.
		"spec/graph/modules/scheduling/policies/w2.md": fmArt("policy", "w2",
			"applies:", "  command: scheduling.create-booking",
			"when:",
			"  - just-a-string",
			"  - {input: subject, note: extra}",
			"  - {input: subject, is-role: {relationship: identity.membership-of, role: member}}",
			"  - {}",
			"  - input:",
			"  - input: unknown-input",
			"  - is-role: not-a-map",
			"  - is-role: {relationship: identity.membership-of, role: member, note: extra}",
			"  - is-role: {}",
			"  - is-role: {relationship: identity.ghost, role: BadRole}"),
		// input on a non-command policy.
		"spec/graph/modules/scheduling/policies/w3.md": fmArt("policy", "w3",
			"applies:", "  entity: scheduling.booking",
			"when:", "  - input: subject"),
		// input with an unresolved applies command: input membership is skipped
		// (the applies violation already reports the root cause).
		"spec/graph/modules/scheduling/policies/w4.md": fmArt("policy", "w4",
			"applies:", "  command: scheduling.ghost",
			"when:", "  - input: subject"),
	})
	res := lintRepo(t, root)
	msgs := messagesOf(res.Violations, "graph-policy-shape")
	assertMentions(t, msgs,
		"when must be a list of clause maps",
		"when clause must be a map carrying exactly one of `input` or `is-role`",
		"when clause has unexpected key \"note\"",
		"when clause must carry exactly one of `input` or `is-role`",
		"when.input must be a non-empty input name",
		"when.input \"unknown-input\" is not an input of command \"scheduling.create-booking\"",
		"is-role must be a {relationship, role} map",
		"is-role has unexpected key \"note\"",
		"is-role must carry a non-empty `relationship`",
		"is-role must carry a non-empty `role`",
		"is-role.relationship reference \"identity.ghost\" does not resolve to an existing relationship",
		"is-role role \"BadRole\" must be a bare lowercase kebab-case token",
		"when.input is only valid when applies names a command (decision 0013)",
	)
	// w4 reports only the unresolved applies target, not the input.
	for _, m := range msgs {
		if strings.Contains(m, "\"subject\" is not an input") {
			t.Fatalf("input membership must be skipped when applies does not resolve: %v", msgs)
		}
	}
}

// TestPolicy_RequiresViolations exercises the requires: clause malformations.
func TestPolicy_RequiresViolations(t *testing.T) {
	root := policyRepo(t, map[string]string{
		// An entity without a lifecycle for the has-no-lifecycle diagnostic.
		"spec/graph/modules/scheduling/entities/hold.md": fmArt("entity", "hold"),
		// Entities whose models are missing, malformed, or unresolvable, for
		// the actor-is skip paths.
		"spec/graph/modules/scheduling/entities/modelless.md": fmArt("entity", "modelless",
			"lifecycle:", "  states: [open]"),
		"spec/graph/modules/scheduling/entities/badmodel.md": fmArt("entity", "badmodel",
			"model: modelspec:///nodotname"),
		"spec/graph/modules/scheduling/entities/ghostmodel.md": fmArt("entity", "ghostmodel",
			"model: modelspec:///scheduling.Ghost"),
		"spec/graph/modules/scheduling/policies/r1.md": fmArt("policy", "r1",
			"applies:", "  command: scheduling.create-booking",
			"requires: not-a-list"),
		"spec/graph/modules/scheduling/policies/r2.md": fmArt("policy", "r2",
			"applies:", "  command: scheduling.create-booking",
			"requires:",
			"  - just-a-string",
			"  - {entity: catalog.item, in-state: published, note: extra}",
			"  - {entity: catalog.item}",
			"  - {entity: catalog.item, in-state: published, actor-is: {entity: identity.account, model-role: owner}}",
			"  - {entity: , in-state: }",
			"  - {entity: catalog.ghost, in-state: published}",
			"  - {entity: catalog.item, in-state: archived}",
			"  - {entity: scheduling.hold, in-state: held}"),
		"spec/graph/modules/scheduling/policies/r3.md": fmArt("policy", "r3",
			"applies:", "  command: scheduling.create-booking",
			"requires:",
			"  - actor-is: not-a-map",
			"  - actor-is: {}",
			"  - actor-is: {entity: identity.account, model-role: owner, note: extra}",
			"  - actor-is: {entity: identity.account, model-role: manager}",
			"  - actor-is: {entity: scheduling.modelless, model-role: owner}",
			"  - actor-is: {entity: scheduling.badmodel, model-role: owner}",
			"  - actor-is: {entity: scheduling.ghostmodel, model-role: owner}"),
	})
	res := lintRepo(t, root)
	msgs := messagesOf(res.Violations, "graph-policy-shape")
	assertMentions(t, msgs,
		"requires must be a list of clause maps",
		"requires clause must be a map: {entity, in-state} or {actor-is: {entity, model-role}}",
		"requires clause has unexpected key \"note\"",
		"requires clause must carry exactly one of {entity, in-state} or {actor-is: {entity, model-role}}",
		"requires must carry a non-empty qualified `entity` reference",
		"requires must carry a non-empty `in-state` token",
		"requires.entity reference \"catalog.ghost\" does not resolve to an existing entity",
		"requires.in-state \"archived\" is not a lifecycle state of entity \"catalog.item\" (states: draft, published)",
		"requires.in-state \"held\" cannot be validated: entity \"scheduling.hold\" has no lifecycle",
		"actor-is must be a {entity, model-role} map",
		"actor-is must carry a non-empty `entity`",
		"actor-is must carry a non-empty `model-role`",
		"actor-is has unexpected key \"note\"",
		"actor-is model-role \"manager\" is not a property of entity \"identity.account\" model concept \"Account\"",
		"actor-is model-role \"owner\" cannot be validated: entity \"scheduling.modelless\" declares no model",
	)
	// The badmodel/ghostmodel actor-is clauses report nothing under
	// graph-policy-shape — the entities' own model: violations carry the cause.
	for _, m := range msgs {
		if strings.Contains(m, "badmodel") || strings.Contains(m, "ghostmodel") {
			t.Fatalf("unvalidatable models must not double-report: %v", msgs)
		}
	}
	if c := ruleCounts(res.Violations)["graph-model-ref-resolves"]; c != 2 {
		t.Fatalf("expected the 2 entity model violations, got %d: %+v", c, res.Violations)
	}
}

// TestPolicy_InvariantViolations exercises the invariant: clause malformations.
func TestPolicy_InvariantViolations(t *testing.T) {
	root := policyRepo(t, map[string]string{
		// invariant on a command policy is illegal.
		"spec/graph/modules/scheduling/policies/i1.md": fmArt("policy", "i1",
			"applies:", "  command: scheduling.create-booking",
			"invariant:",
			"  - when-referenced: {entity: identity.account, in-state: suspended}",
			"    then: {self-state: cancelled}"),
		// Container is not a list.
		"spec/graph/modules/scheduling/policies/i2.md": fmArt("policy", "i2",
			"applies:", "  entity: scheduling.booking",
			"invariant: not-a-list"),
		// Item shapes.
		"spec/graph/modules/scheduling/policies/i3.md": fmArt("policy", "i3",
			"applies:", "  entity: scheduling.booking",
			"invariant:",
			"  - just-a-string",
			"  - {when-referenced: {entity: identity.account, in-state: suspended}, then: {self-state: cancelled}, note: extra}",
			"  - {when-referenced: {entity: identity.account, in-state: suspended}}",
			"  - {when-referenced: not-a-map, then: not-a-map}",
			"  - {when-referenced: {entity: identity.account, in-state: suspended, note: extra}, then: {self-state: cancelled, note: extra}}",
			"  - {when-referenced: {entity: identity.account, in-state: suspended}, then: {}}",
			"  - {when-referenced: {entity: identity.account, in-state: suspended}, then: {self-state: archived}}"),
		// With an unresolved applies entity, then.self-state is skipped.
		"spec/graph/modules/scheduling/policies/i4.md": fmArt("policy", "i4",
			"applies:", "  entity: scheduling.ghost",
			"invariant:",
			"  - when-referenced: {entity: identity.account, in-state: suspended}",
			"    then: {self-state: cancelled}"),
	})
	res := lintRepo(t, root)
	msgs := messagesOf(res.Violations, "graph-policy-shape")
	assertMentions(t, msgs,
		"invariant is only valid when applies names an entity (decision 0013)",
		"invariant must be a list of clause maps",
		"invariant clause must be a map: {when-referenced: {entity, in-state}, then: {self-state}}",
		"invariant clause has unexpected key \"note\"",
		"invariant clause must carry both `when-referenced` and `then`",
		"when-referenced must be a {entity, in-state} map",
		"then must be a {self-state} map",
		"when-referenced has unexpected key \"note\"",
		"then has unexpected key \"note\"",
		"then must carry a non-empty `self-state`",
		"then.self-state \"archived\" is not a lifecycle state of entity \"scheduling.booking\" (states: pending, confirmed, cancelled)",
	)
	for _, m := range msgs {
		if strings.Contains(m, "then.self-state \"cancelled\"") {
			t.Fatalf("self-state must be skipped when applies does not resolve: %v", msgs)
		}
	}
}

// TestPolicy_DependencyDirection proves policy clause references count against
// the owning module's dependsOn like every other reference.
func TestPolicy_DependencyDirection(t *testing.T) {
	root := policyRepo(t, map[string]string{
		// catalog does not depend on identity or scheduling.
		"spec/graph/modules/catalog/policies/stray.md": fmArt("policy", "stray",
			"applies:", "  command: scheduling.create-booking",
			"when:",
			"  - is-role: {relationship: identity.membership-of, role: member}"),
	})
	res := lintRepo(t, root)
	if got := ruleCounts(res.Violations)["graph-dependency-direction"]; got != 2 {
		t.Fatalf("expected 2 dependency-direction violations, got %d: %+v", got, res.Violations)
	}
}

// TestPolicy_UnknownKeyAndKindChecks proves policies participate in the shared
// artifact rules: unknown keys warn, and the kind must match the collection.
func TestPolicy_UnknownKeyAndKindChecks(t *testing.T) {
	root := policyRepo(t, map[string]string{
		// rules: is NOT defined for the policy kind (decision 0013).
		"spec/graph/modules/scheduling/policies/extra.md": fmArt("policy", "extra",
			"applies:", "  entity: scheduling.booking",
			"rules:", "  - id: r", "    text: t"),
		"spec/graph/modules/scheduling/policies/miskind.md": fmArt("entity", "miskind",
			"applies:", "  entity: scheduling.booking"),
	})
	res := lintRepo(t, root)
	assertMentions(t, messagesOf(res.Violations, "graph-unknown-key"),
		"key \"rules\" is not defined for kind \"policy\"")
	assertMentions(t, messagesOf(res.Violations, "graph-kind-valid"),
		"kind \"entity\" does not match its collection directory (expected \"policy\")")
	if hasRule(res.Violations, "graph-rules-shape") {
		t.Fatalf("rules on a policy must not run graph-rules-shape: %+v", res.Violations)
	}
}

// TestScaffold_Policy scaffolds `graph new policy`: the artifact lands in
// policies/ with a commented applies: hint, and — since applies is required —
// the fresh artifact reports exactly the missing-applies policy-shape error.
func TestScaffold_Policy(t *testing.T) {
	root := repoWith(t, map[string]string{})
	if _, err := Scaffold(ScaffoldOptions{Kind: KindModule, Name: "Reservations", RepoRoot: root}); err != nil {
		t.Fatal(err)
	}
	res, err := Scaffold(ScaffoldOptions{Kind: KindPolicy, Name: "ApprovalRequired", Module: "reservations", RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if res.Target != "spec/graph/modules/reservations/policies/approval-required.md" {
		t.Fatalf("unexpected target %q", res.Target)
	}
	data, err := readFileFn(root + "/" + res.Target)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{"kind: policy", "# applies:", "#   command: reservations.<command-id>", "# Policy: ApprovalRequired"} {
		if !strings.Contains(content, want) {
			t.Errorf("scaffolded policy missing %q:\n%s", want, content)
		}
	}
	lres := lintRepo(t, root)
	msgs := messagesOf(lres.Violations, "graph-policy-shape")
	if len(lres.Violations) != 1 || len(msgs) != 1 || !strings.Contains(msgs[0], "must declare an `applies:` block") {
		t.Fatalf("fresh policy should report exactly the missing applies: %+v", lres.Violations)
	}
}
