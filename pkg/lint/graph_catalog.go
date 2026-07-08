package lint

// graphCatalogRules is the catalog of GraphSpec lint rules emitted by
// `specscore graph lint` (implemented in pkg/graph). They are deliberately kept
// OUT of the shared ruleRegistry: GraphSpec validation ships standalone as
// `graph lint` first (graph/validation#req:graph-lint-standalone-first), so the
// default `spec lint` linter does not emit them and the registry/checker parity
// gate (CheckRegistryParity) must not see them. They are still surfaced in the
// rendered catalog (docs/lint-rules.md) so the family is documented.
var graphCatalogRules = []Rule{
	{ID: "graph-id-equals-filename-stem", Family: "graph", Severity: "error", Description: "Requires a graph artifact id to equal its filename stem (a module id equals its directory name)."},
	{ID: "graph-id-kebab-case", Family: "graph", Severity: "error", Description: "Requires graph artifact ids to be bare lowercase kebab-case."},
	{ID: "graph-no-module-prefix-in-id", Family: "graph", Severity: "error", Description: "Rejects a module prefix (dot) in a graph artifact id; the qualified form is computed, not stored."},
	{ID: "graph-kind-valid", Family: "graph", Severity: "error", Description: "Requires a readable kind: that is one of module|entity|relationship|command|event|policy and matches its collection directory."},
	{ID: "graph-no-owner-field", Family: "graph", Severity: "error", Description: "Rejects an owner: field; ownership is derived from placement."},
	{ID: "graph-no-inline-structure", Family: "graph", Severity: "error", Description: "Rejects inline fields:/properties: in a graph artifact; structure lives in ModelSpec."},
	{ID: "graph-reference-resolves", Family: "graph", Severity: "error", Description: "Requires qualified graph references (from/to/subject/actors/participants/inputs.ref/possibleEvents) to resolve to existing artifacts."},
	{ID: "graph-model-ref-resolves", Family: "graph", Severity: "error", Description: "Requires modelspec:// and HCL concept references to resolve per decisions 0007/0010/0011 (unknown module / unknown concept / kind mismatch / unavailable repository / bad grammar)."},
	{ID: "graph-model-legacy-form", Family: "graph", Severity: "error", Description: "Rejects the legacy modelspec://x.Y reference form (authority present, empty path); carries the exact modelspec:///x.Y rewrite that `graph lint --fix` applies (decision 0010)."},
	{ID: "graph-model-reserved-name", Family: "graph", Severity: "error", Description: "Rejects a ModelSpec concept named with a reserved kind token (entities, components, enums, collections, recordsets) (decision 0011)."},
	{ID: "graph-dependency-direction", Family: "graph", Severity: "error", Description: "Requires every cross-module reference (graph or model level) to target a module in the owning module's dependsOn."},
	{ID: "graph-relationship-owner-covers-endpoints", Family: "graph", Severity: "error", Description: "Requires a relationship's owning module to cover both endpoint modules in its dependsOn closure."},
	{ID: "graph-metadata-shape", Family: "graph", Severity: "error", Description: "Requires relationship metadata to be a flat map of scalar, qualified graph, or modelspec:// values."},
	{ID: "graph-inputs-shape", Family: "graph", Severity: "error", Description: "Requires command inputs items to carry a kebab-case name plus exactly one of ref/model."},
	{ID: "graph-rules-shape", Family: "graph", Severity: "error", Description: "Requires a Tier-1 rules: block on an entity/relationship/command artifact to be a list of {id, text, refs} maps with artifact-unique kebab-case ids, non-empty text, and resolvable refs (decision 0013)."},
	{ID: "graph-policy-shape", Family: "graph", Severity: "error", Description: "Requires a policy artifact to carry an applies: block naming exactly one of command|entity|relationship plus well-formed when/requires/invariant clauses whose operands resolve against the graph — inputs, lifecycle states, roles, and ModelSpec model properties (decision 0013)."},
	{ID: "graph-event-sources", Family: "graph", Severity: "warning", Description: "Warns on an explicitly empty sources: [] and rejects command references in event sources."},
	{ID: "graph-lifecycle-states", Family: "graph", Severity: "error", Description: "Requires a declared lifecycle.states list to be non-empty and free of duplicates."},
	{ID: "graph-duplicate-id", Family: "graph", Severity: "error", Description: "Rejects duplicate qualified graph IDs across the graph roots."},
	{ID: "graph-duplicate-module-id", Family: "graph", Severity: "error", Description: "Rejects a GraphSpec module id declared by more than one graph root in the union (decision 0009)."},
	{ID: "graph-model-duplicate-concept", Family: "graph", Severity: "error", Description: "Rejects duplicate concept names within a module's ModelSpec sources."},
	{ID: "graph-model-enum-values", Family: "graph", Severity: "error", Description: "Requires ModelSpec enum values to be non-empty and free of duplicates."},
	{ID: "graph-role-labels", Family: "graph", Severity: "error", Description: "Requires role-labeled endpoints and participants to be well-formed {ref, role} maps with kebab-case role tokens (decision 0012)."},
	{ID: "graph-ambiguous-endpoints", Family: "graph", Severity: "warning", Description: "Warns on a self-referential relationship without endpoint role labels and on same-reference event participants lacking distinguishing roles (decision 0012)."},
	{ID: "graph-unknown-key", Family: "graph", Severity: "warning", Description: "Warns on frontmatter keys the artifact's placement-derived kind does not define (silently ignored by tooling, e.g. lifecycle: on a relationship)."},
	{ID: "graph-event-reachability", Family: "graph", Severity: "info", Description: "Reports an event that is in no command's possibleEvents and declares no sources — nothing can produce it."},
}

// GraphRules returns a copy of the GraphSpec rule catalog. The `graph` command
// group uses it to validate --rules/--ignore selectors and to look up per-rule
// severity; the catalog renderer includes it in docs/lint-rules.md.
func GraphRules() []Rule {
	out := make([]Rule, len(graphCatalogRules))
	copy(out, graphCatalogRules)
	return out
}
