package lint

import (
	"fmt"
	"sort"
)

// Rule is a structured registry entry describing a single lint rule. The
// registry of Rule values is the single source of truth for the set of known
// rule names, their human-readable descriptions, the family they belong to,
// and the severity at which their checker reports.
//
// Tags mirror the Violation struct style so downstream commands can emit
// registry entries as JSON/YAML directly.
type Rule struct {
	ID          string `json:"id" yaml:"id"`
	Description string `json:"description" yaml:"description"`
	Family      string `json:"family" yaml:"family"`
	Severity    string `json:"severity" yaml:"severity"`
}

// ruleRegistry is the canonical registry, keyed by rule ID. It is the single
// source of truth: AllRuleNames and ValidateRuleNames derive from it, and
// custom-checker registration adds/removes synthetic entries here.
var ruleRegistry = map[string]Rule{}

// registerRule inserts r into the registry, enforcing the registry invariant
// that every entry carries a non-empty ID, description, family, and severity.
// It panics on an empty field or a duplicate ID. Panicking at construction
// time makes an entry with an empty description impossible to register
// (cli/rules#req:structured-registry).
func registerRule(r Rule) {
	if r.ID == "" {
		panic("lint: rule registered with empty id")
	}
	if r.Description == "" {
		panic(fmt.Sprintf("lint: rule %q registered with empty description", r.ID))
	}
	if r.Family == "" {
		panic(fmt.Sprintf("lint: rule %q registered with empty family", r.ID))
	}
	if r.Severity == "" {
		panic(fmt.Sprintf("lint: rule %q registered with empty severity", r.ID))
	}
	if _, dup := ruleRegistry[r.ID]; dup {
		panic(fmt.Sprintf("lint: rule %q registered twice", r.ID))
	}
	ruleRegistry[r.ID] = r
}

// AllRules returns a deterministically sorted (by ID) copy of the registry
// entries. Downstream consumers (the `rules` command, the catalog renderer)
// read structured rules through this accessor.
func AllRules() []Rule {
	out := make([]Rule, 0, len(ruleRegistry))
	for _, r := range ruleRegistry {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// builtinRules is the structured catalog of every built-in lint rule. Family
// tokens follow the checker groupings; severity matches each rule's checker
// severity() contract.
var builtinRules = []Rule{
	// Core / misc rules.
	{ID: "readme-exists", Family: "core", Severity: "error", Description: "Requires a README.md in every spec directory."},
	{ID: "oq-section", Family: "core", Severity: "error", Description: "Requires an Open Questions section in feature README files."},
	{ID: "oq-not-empty", Family: "core", Severity: "error", Description: "Warns when the Open Questions section is empty."},
	{ID: "index-entries", Family: "core", Severity: "error", Description: "Requires index README files to list every child entry."},
	{ID: "plan-hierarchy", Family: "core", Severity: "error", Description: "Requires plan tasks to form a valid hierarchy."},
	{ID: "plan-roi-metadata", Family: "core", Severity: "warning", Description: "Warns when a plan is missing ROI metadata."},
	{ID: "plan-index-sync", Family: "plan", Severity: "error", Description: "Requires the canonical plans index table to match single-file Plan metadata; --fix regenerates drifted rows."},
	{ID: "adherence-footer", Family: "core", Severity: "error", Description: "Requires the spec-adherence footer on managed documents."},
	{ID: "studio-toolbar", Family: "core", Severity: "error", Description: "Requires the SpecStudio toolbar block on managed documents."},
	{ID: "dogfood-version-bump", Family: "core", Severity: "warning", Description: "Warns when a workflow pins a stale specscore CLI version."},
	{ID: "config-user-scoped-key", Family: "core", Severity: "error", Description: "Rejects user-scoped keys in the project-scoped config file."},

	// Idea lint rules.
	{ID: "idea-location", Family: "idea", Severity: "error", Description: "Requires idea files to live under spec/ideas/."},
	{ID: "idea-slug-format", Family: "idea", Severity: "error", Description: "Requires idea filenames to use kebab-case slugs."},
	{ID: "idea-single-file", Family: "idea", Severity: "error", Description: "Requires each idea to be a single file, not a directory."},
	{ID: "idea-title-format", Family: "idea", Severity: "error", Description: "Requires the idea H1 title to follow the canonical format."},
	{ID: "idea-header-fields", Family: "idea", Severity: "error", Description: "Requires the idea header to declare all mandatory fields."},
	{ID: "idea-id-is-slug", Family: "idea", Severity: "error", Description: "Requires the idea Id field to match its filename slug."},
	{ID: "idea-required-sections", Family: "idea", Severity: "error", Description: "Requires every mandatory idea body section to be present."},
	{ID: "idea-not-doing-non-empty", Family: "idea", Severity: "error", Description: "Requires the Not Doing section to be non-empty."},
	{ID: "idea-must-be-true-present", Family: "idea", Severity: "error", Description: "Requires the Must Be True section to be present."},
	{ID: "idea-hmw-framing", Family: "idea", Severity: "error", Description: "Requires the idea to use How-Might-We framing."},
	{ID: "idea-status-values", Family: "idea", Severity: "error", Description: "Rejects idea Status values outside the allowed set."},
	{ID: "idea-specified-requires-promotion", Family: "idea", Severity: "error", Description: "Requires a Specified idea to record its promotion target."},
	{ID: "idea-archived-location", Family: "idea", Severity: "error", Description: "Requires archived ideas to live under the archived directory."},
	{ID: "idea-archive-reason", Family: "idea", Severity: "error", Description: "Requires an archived idea to record an archive reason."},
	{ID: "idea-supersedes-target-archived", Family: "idea", Severity: "error", Description: "Requires a superseded idea target to be archived."},
	{ID: "idea-related-ideas-format", Family: "idea", Severity: "error", Description: "Requires the Related Ideas list to use the canonical format."},
	{ID: "idea-related-ideas-target-exists", Family: "idea", Severity: "error", Description: "Requires every Related Ideas target to exist."},
	{ID: "idea-sync-lint-strict", Family: "idea", Severity: "error", Description: "Enforces strict synchronization between idea fields and index."},
	{ID: "idea-feature-cross-reference", Family: "idea", Severity: "error", Description: "Requires idea-to-feature cross-references to resolve."},
	{ID: "idea-index-completeness", Family: "idea", Severity: "error", Description: "Requires the idea index to list every idea."},
	{ID: "idea-index-row-sync", Family: "idea", Severity: "error", Description: "Requires each idea index row to match its idea file."},
	{ID: "idea-archived-index-chronological", Family: "idea", Severity: "error", Description: "Requires the archived idea index to be in chronological order."},
	{ID: "idea-type-values", Family: "idea", Severity: "error", Description: "Rejects idea Type values outside the allowed set."},
	{ID: "idea-type-title-consistency", Family: "idea", Severity: "error", Description: "Requires the idea Type to be consistent with its title."},
	{ID: "idea-targets-required", Family: "idea", Severity: "error", Description: "Requires a change-request idea to declare its targets."},
	{ID: "idea-targets-exists", Family: "idea", Severity: "error", Description: "Requires every declared idea target to exist."},
	{ID: "idea-change-request-location", Family: "idea", Severity: "error", Description: "Requires change-request ideas to live in the expected location."},
	{ID: "idea-phase-non-empty", Family: "idea", Severity: "error", Description: "Requires the idea Phase field to be non-empty."},

	// Feature index lint rules.
	{ID: "feature-index-row-sync", Family: "feature-index", Severity: "error", Description: "Requires each feature index row to match its feature file."},

	// Sidekick-seed lint rule.
	{ID: "sidekick-seed", Family: "sidekick", Severity: "error", Description: "Requires sidekick seed files to be well-formed."},

	// Grade body-metadata-field lint rules.
	{ID: "grade-values-shape", Family: "grade", Severity: "error", Description: "Requires the grade metadata field to have the canonical shape."},
	{ID: "grade-single-value", Family: "grade", Severity: "error", Description: "Requires exactly one grade value per artifact."},
	{ID: "grade-placement", Family: "grade", Severity: "error", Description: "Requires the grade field to appear in the correct position."},
	{ID: "grade-value", Family: "grade", Severity: "error", Description: "Rejects grade values outside the allowed A-F set."},

	// Parked body-metadata-field lint rules.
	{ID: "parked-shape", Family: "parked", Severity: "error", Description: "Requires a **Parked:** true axis to carry a non-empty **Parked Reason:** and a well-formed **Parked Date:**."},
	{ID: "parked-stale", Family: "parked", Severity: "warning", Description: "Warns when an artifact has been parked longer than the configured (or default 90-day) review window."},

	// Artifact-frontmatter-convention lint rules. Graced at warning severity
	// during the migration rollout (REQ:migration-sequencing); flips to error
	// after target repos are migrated.
	{ID: "format-field", Family: "frontmatter", Severity: "error", Description: "Requires every artifact to carry a frontmatter format: field matching its type's spec URL."},
	{ID: "status-mirror", Family: "frontmatter", Severity: "error", Description: "Requires a status-bearing artifact's frontmatter status: to mirror its body **Status:** line, and forbids status: on status-less types."},
	{ID: "footer-format-mirror", Family: "frontmatter", Severity: "error", Description: "Requires the adherence-footer URL to match the frontmatter format: URL; --fix derives the footer from format:."},

	// Plan lint rules.
	{ID: "P-001", Family: "plan", Severity: "error", Description: "Requires every plan task to reference at least one feature AC."},
	{ID: "P-002", Family: "plan", Severity: "error", Description: "Requires plan tasks to declare valid dependency references."},
	{ID: "P-003", Family: "plan", Severity: "error", Description: "Requires the plan to follow the single-file structure contract."},
	{ID: "P-004", Family: "plan", Severity: "error", Description: "Requires plan metadata to be complete and well-formed."},
	{ID: "P-005", Family: "plan", Severity: "error", Description: "Validates a plan's **Parent:** reference — same-repo parents resolve, are acyclic, and not self-referential; cross-repo <repo>:<slug> refs are checked syntactically only."},
	{ID: "P-006", Family: "plan", Severity: "error", Description: "Requires a single-file plan's body **Status:** value to be one of the canonical Plan statuses (Draft, In Review, Approved, Executing, Blocked, Implemented, Failed, Rejected, Withdrawn, Superseded, Deprecated)."},
	{ID: "P-007", Family: "plan", Severity: "error", Description: "Derives a single-file plan's execution-band **Status:** (Executing/Blocked/Implemented/Failed) from its task-status rollup when the body status is Approved or an execution band; --fix reconciles drift. Prep and disposition statuses are never overwritten."},
	{ID: "P-008", Family: "plan", Severity: "error", Description: "Validates a task's optional **Implemented-by:** implementation-commit provenance value against the provenance-ref-format (<repo>@<sha> or bare <sha>, optional trailing branch; <sha> = 7-40 hex chars) — syntactic only, never scans the referenced repo."},
	{ID: "P-009", Family: "plan", Severity: "error", Description: "Validates optional same-repository **Prerequisite Plans:** references: canonical slugs, existence, no duplicates or self-references, and an acyclic dependency graph."},
	{ID: "P-010", Family: "plan", Severity: "error", Description: "Validates a plan's optional **Coordination:** reference against coordination-branch-format (<owner>/<repo>@<branch>) — syntactic only, never resolves or scans the named repo/branch."},

	// Feature lint rules.
	{ID: "feature-source-ideas-required", Family: "feature", Severity: "error", Description: "Requires every Feature README to carry a **Source Ideas:** line with an explicit sentinel (— / none) or a slug list; --fix backfills the sentinel."},
	{ID: "ac-heading-format", Family: "feature", Severity: "error", Description: "Requires Acceptance Criteria headings to read `### AC: <id>` with one space after the colon and a kebab-case id; --fix repairs whitespace-only deviations."},

	// Decision lint rules.
	{ID: "D-title-format", Family: "decision", Severity: "error", Description: "Requires the decision H1 title to follow the canonical format."},
	{ID: "D-header-fields", Family: "decision", Severity: "error", Description: "Requires the decision header to declare all mandatory fields."},
	{ID: "D-status-values", Family: "decision", Severity: "error", Description: "Rejects decision Status values outside the allowed set."},
	{ID: "D-filename-format", Family: "decision", Severity: "error", Description: "Requires decision filenames to follow the canonical format."},
	{ID: "D-number-assignment", Family: "decision", Severity: "error", Description: "Requires each decision to have a unique sequential number."},
	{ID: "D-single-file", Family: "decision", Severity: "error", Description: "Requires each decision to be a single file, not a directory."},
	{ID: "D-required-sections", Family: "decision", Severity: "error", Description: "Requires every mandatory decision body section to be present."},
	{ID: "D-declined-alternatives-non-empty", Family: "decision", Severity: "error", Description: "Requires the Declined Alternatives section to be non-empty."},
	{ID: "D-observed-consequences-placeholder", Family: "decision", Severity: "error", Description: "Requires the Observed Consequences section to use the placeholder until filled."},
	{ID: "D-source-idea-optional", Family: "decision", Severity: "error", Description: "Validates the optional Source Idea reference when present."},
	{ID: "D-supersedes-target-exists", Family: "decision", Severity: "error", Description: "Requires a superseding decision target to exist."},
	{ID: "D-supersedes-bidirectional", Family: "decision", Severity: "error", Description: "Requires supersedes references to be bidirectional."},
	{ID: "D-archived-location", Family: "decision", Severity: "error", Description: "Requires archived decisions to live under the archived directory."},
	{ID: "D-superseded-requires-successor", Family: "decision", Severity: "error", Description: "Requires a superseded decision to name its successor."},
	{ID: "D-affected-features-target-exists", Family: "decision", Severity: "error", Description: "Requires every Affected Features target to exist."},

	// Decision immutability rules.
	{ID: "D-immutability-once-accepted", Family: "decision-immutability", Severity: "error", Description: "Rejects edits to a decision once it has been accepted."},
	{ID: "D-observed-consequences-append-only", Family: "decision-immutability", Severity: "error", Description: "Requires the Observed Consequences section to change append-only."},

	// Decisions-index lint rules.
	{ID: "DI-list-section-heading", Family: "decisions-index", Severity: "error", Description: "Requires the decisions index to use the canonical list section heading."},
	{ID: "DI-index-columns", Family: "decisions-index", Severity: "error", Description: "Requires the decisions index table to have the canonical columns."},
	{ID: "DI-status-excludes-archived", Family: "decisions-index", Severity: "error", Description: "Requires the active index to exclude archived decisions."},
	{ID: "DI-numeric-ordering", Family: "decisions-index", Severity: "error", Description: "Requires the decisions index rows to be in numeric order."},
	{ID: "DI-archived-index-chronological", Family: "decisions-index", Severity: "error", Description: "Requires the archived decisions index to be in chronological order."},
	{ID: "DI-completeness", Family: "decisions-index", Severity: "error", Description: "Requires the decisions index to list every decision."},
	{ID: "DI-archived-status-excludes-active", Family: "decisions-index", Severity: "error", Description: "Requires the archived index to exclude active decisions."},
	{ID: "DI-row-content-sync", Family: "decisions-index", Severity: "error", Description: "Requires active decision index rows to mirror artifact title, status, date, tags, and affected features; --fix reconciles drift."},

	// Issue lint rules.
	{ID: "I-001", Family: "issue", Severity: "error", Description: "Requires issue frontmatter to contain all mandatory fields and no unknown keys."},
	{ID: "I-002", Family: "issue", Severity: "error", Description: "Rejects issue status values outside the allowed set."},
	{ID: "I-003", Family: "issue", Severity: "error", Description: "Validates the shape of optional issue frontmatter fields."},
	{ID: "I-004", Family: "issue", Severity: "error", Description: "Requires the issue slug to match its filename."},
	{ID: "I-005", Family: "issue", Severity: "error", Description: "Requires severity to be set once an issue leaves the open status."},
	{ID: "I-006", Family: "issue", Severity: "error", Description: "Rejects rejection_reason values outside the allowed set."},
	{ID: "I-007", Family: "issue", Severity: "error", Description: "Requires a rejection reason when an issue is rejected."},
	{ID: "I-008", Family: "issue", Severity: "error", Description: "Validates the shape of the issue bugs list."},
	{ID: "I-009", Family: "issue", Severity: "error", Description: "Rejects an issue that exists in two locations at once."},
	{ID: "I-010", Family: "issue", Severity: "error", Description: "Requires the issue captured_at field to be a valid timestamp."},
	{ID: "I-011", Family: "issue", Severity: "error", Description: "Requires the issue captured_by field to be non-empty."},
	{ID: "I-012", Family: "issue", Severity: "error", Description: "Requires resolved issues to record resolution metadata."},
	{ID: "I-013", Family: "issue", Severity: "error", Description: "Requires issue cross-references to resolve."},
	{ID: "I-014", Family: "issue", Severity: "error", Description: "Requires the issue index to list every issue."},
	{ID: "I-015", Family: "issue", Severity: "error", Description: "Requires the issue body to contain its mandatory sections."},

	// Capability / platform-implementation lint rules.
	{ID: "implements-reference", Family: "capability", Severity: "error", Description: "Validates the Implements reference on an Implementation Feature."},
	{ID: "implementation-matrix", Family: "capability", Severity: "error", Description: "Validates the shape of a Capability's Implementation Matrix table."},
	{ID: "other-platforms-links-only", Family: "capability", Severity: "error", Description: "Rejects a parity status restated in an Implementation's Other Platforms section."},

	// Property lint rules.
	{ID: "property-location", Family: "property", Severity: "error", Description: "Requires property files to live in the expected location."},
	{ID: "property-slug-format", Family: "property", Severity: "error", Description: "Requires property filenames to use kebab-case slugs."},
	{ID: "property-single-file", Family: "property", Severity: "error", Description: "Requires each property to be a single file, not a directory."},
	{ID: "property-frontmatter-required", Family: "property", Severity: "error", Description: "Requires property files to carry frontmatter."},
	{ID: "property-frontmatter-required-fields", Family: "property", Severity: "error", Description: "Requires property frontmatter to declare all mandatory fields."},
	{ID: "property-id-equals-slug", Family: "property", Severity: "error", Description: "Requires the property id to match its filename slug."},
	{ID: "property-data-type-values", Family: "property", Severity: "error", Description: "Rejects property data_type values outside the allowed set."},
	{ID: "property-checks-shape", Family: "property", Severity: "error", Description: "Requires the property checks block to have the canonical shape."},
	{ID: "property-title-format", Family: "property", Severity: "error", Description: "Requires the property H1 title to follow the canonical format."},
	{ID: "property-required-sections", Family: "property", Severity: "error", Description: "Requires every mandatory property body section to be present."},
	{ID: "property-referenced-by-managed", Family: "property", Severity: "error", Description: "Requires the property Referenced by section to be managed."},

	// Lesson lint rules.
	{ID: "L-001", Family: "lesson", Severity: "error", Description: "Requires every Lesson body to declare the required section set for its canonical-directory or compatibility-flat layout."},
	{ID: "L-002", Family: "lesson", Severity: "error", Description: "Requires a Lesson's body **Status:** value to be one of the canonical enforcement-ladder statuses (Recorded, Stated, Enforced, Withdrawn, Superseded)."},
	{ID: "L-003", Family: "lesson", Severity: "error", Description: "Requires the lessons index to list every lesson; --fix inserts missing rows."},
	{ID: "L-004", Family: "lesson", Severity: "error", Description: "Requires each lessons index row to match the layout-specific Lesson projection; --fix regenerates drifted rows."},
	{ID: "L-005", Family: "lesson", Severity: "error", Description: "Requires canonical directory Lessons to carry ordered metadata and repository-controlled classifications."},
	{ID: "L-006", Family: "lesson", Severity: "error", Description: "Requires canonical Lesson Tracking to declare the append-only occurrence store, derived recurrence, and published schema URL."},
	{ID: "L-007", Family: "lesson", Severity: "error", Description: "Requires Enforced Lessons to declare deterministic control, verification, and stable evidence."},
	{ID: "L-008", Family: "lesson", Severity: "error", Description: "Requires duplicate and supersession relations to resolve without conflicts or cycles."},
	{ID: "L-009", Family: "lesson", Severity: "error", Description: "Requires every canonical Lesson occurrence child to satisfy the published append-only JSON contract."},

	// Entity lint rules.
	{ID: "entity-location", Family: "entity", Severity: "error", Description: "Requires entity files to live in the expected location."},
	{ID: "entity-slug-format", Family: "entity", Severity: "error", Description: "Requires entity filenames to use kebab-case slugs."},
	{ID: "entity-single-file", Family: "entity", Severity: "error", Description: "Requires each entity to be a single file, not a directory."},
	{ID: "entity-frontmatter-required", Family: "entity", Severity: "error", Description: "Requires entity files to carry frontmatter."},
	{ID: "entity-frontmatter-required-fields", Family: "entity", Severity: "error", Description: "Requires entity frontmatter to declare all mandatory fields."},
	{ID: "entity-id-equals-slug", Family: "entity", Severity: "error", Description: "Requires the entity id to match its filename slug."},
	{ID: "entity-properties-list-shape", Family: "entity", Severity: "error", Description: "Requires the entity properties list to have the canonical shape."},
	{ID: "entity-ref-target-exists", Family: "entity", Severity: "error", Description: "Requires every entity property reference target to exist."},
	{ID: "entity-inherits-additive-only", Family: "entity", Severity: "error", Description: "Requires entity inheritance to be additive only."},
	{ID: "entity-inherits-target-exists", Family: "entity", Severity: "error", Description: "Requires every entity inherits target to exist."},
	{ID: "entity-inherits-acyclic", Family: "entity", Severity: "error", Description: "Rejects cycles in the entity inheritance graph."},
	{ID: "entity-title-format", Family: "entity", Severity: "error", Description: "Requires the entity H1 title to follow the canonical format."},
	{ID: "entity-required-sections", Family: "entity", Severity: "error", Description: "Requires every mandatory entity body section to be present."},
	{ID: "entity-properties-table-managed", Family: "entity", Severity: "error", Description: "Requires the entity Properties table to be managed."},
	{ID: "entity-referenced-by-managed", Family: "entity", Severity: "error", Description: "Requires the entity Referenced by section to be managed."},
}

func init() {
	for _, r := range builtinRules {
		registerRule(r)
	}
}
