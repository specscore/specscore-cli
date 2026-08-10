# Lint Rule Catalog

Generated from the lint rule registry. Do not edit by hand.

Total rules: 159

## capability

| Rule | Severity | Description |
| --- | --- | --- |
| implementation-matrix | error | Validates the shape of a Capability's Implementation Matrix table. |
| implements-reference | error | Validates the Implements reference on an Implementation Feature. |
| other-platforms-links-only | error | Rejects a parity status restated in an Implementation's Other Platforms section. |

## core

| Rule | Severity | Description |
| --- | --- | --- |
| adherence-footer | error | Requires the spec-adherence footer on managed documents. |
| config-user-scoped-key | error | Rejects user-scoped keys in the project-scoped config file. |
| dogfood-version-bump | warning | Warns when a workflow pins a stale specscore CLI version. |
| index-entries | error | Requires index README files to list every child entry. |
| oq-not-empty | error | Warns when the Open Questions section is empty. |
| oq-section | error | Requires an Open Questions section in feature README files. |
| plan-hierarchy | error | Requires plan tasks to form a valid hierarchy. |
| plan-roi-metadata | warning | Warns when a plan is missing ROI metadata. |
| readme-exists | error | Requires a README.md in every spec directory. |
| studio-toolbar | error | Requires the SpecStudio toolbar block on managed documents. |

## decision

| Rule | Severity | Description |
| --- | --- | --- |
| D-affected-features-target-exists | error | Requires every Affected Features target to exist. |
| D-archived-location | error | Requires archived decisions to live under the archived directory. |
| D-declined-alternatives-non-empty | error | Requires the Declined Alternatives section to be non-empty. |
| D-filename-format | error | Requires decision filenames to follow the canonical format. |
| D-header-fields | error | Requires the decision header to declare all mandatory fields. |
| D-number-assignment | error | Requires each decision to have a unique sequential number. |
| D-observed-consequences-placeholder | error | Requires the Observed Consequences section to use the placeholder until filled. |
| D-required-sections | error | Requires every mandatory decision body section to be present. |
| D-single-file | error | Requires each decision to be a single file, not a directory. |
| D-source-idea-optional | error | Validates the optional Source Idea reference when present. |
| D-status-values | error | Rejects decision Status values outside the allowed set. |
| D-superseded-requires-successor | error | Requires a superseded decision to name its successor. |
| D-supersedes-bidirectional | error | Requires supersedes references to be bidirectional. |
| D-supersedes-target-exists | error | Requires a superseding decision target to exist. |
| D-title-format | error | Requires the decision H1 title to follow the canonical format. |

## decision-immutability

| Rule | Severity | Description |
| --- | --- | --- |
| D-immutability-once-accepted | error | Rejects edits to a decision once it has been accepted. |
| D-observed-consequences-append-only | error | Requires the Observed Consequences section to change append-only. |

## decisions-index

| Rule | Severity | Description |
| --- | --- | --- |
| DI-archived-index-chronological | error | Requires the archived decisions index to be in chronological order. |
| DI-archived-status-excludes-active | error | Requires the archived index to exclude active decisions. |
| DI-completeness | error | Requires the decisions index to list every decision. |
| DI-index-columns | error | Requires the decisions index table to have the canonical columns. |
| DI-list-section-heading | error | Requires the decisions index to use the canonical list section heading. |
| DI-numeric-ordering | error | Requires the decisions index rows to be in numeric order. |
| DI-row-content-sync | error | Requires active decision index rows to mirror artifact title, status, date, tags, and affected features; --fix reconciles drift. |
| DI-status-excludes-archived | error | Requires the active index to exclude archived decisions. |

## entity

| Rule | Severity | Description |
| --- | --- | --- |
| entity-frontmatter-required | error | Requires entity files to carry frontmatter. |
| entity-frontmatter-required-fields | error | Requires entity frontmatter to declare all mandatory fields. |
| entity-id-equals-slug | error | Requires the entity id to match its filename slug. |
| entity-inherits-acyclic | error | Rejects cycles in the entity inheritance graph. |
| entity-inherits-additive-only | error | Requires entity inheritance to be additive only. |
| entity-inherits-target-exists | error | Requires every entity inherits target to exist. |
| entity-location | error | Requires entity files to live in the expected location. |
| entity-properties-list-shape | error | Requires the entity properties list to have the canonical shape. |
| entity-properties-table-managed | error | Requires the entity Properties table to be managed. |
| entity-ref-target-exists | error | Requires every entity property reference target to exist. |
| entity-referenced-by-managed | error | Requires the entity Referenced by section to be managed. |
| entity-required-sections | error | Requires every mandatory entity body section to be present. |
| entity-single-file | error | Requires each entity to be a single file, not a directory. |
| entity-slug-format | error | Requires entity filenames to use kebab-case slugs. |
| entity-title-format | error | Requires the entity H1 title to follow the canonical format. |

## feature

| Rule | Severity | Description |
| --- | --- | --- |
| ac-heading-format | error | Requires Acceptance Criteria headings to read `### AC: <id>` with one space after the colon and a kebab-case id; --fix repairs whitespace-only deviations. |
| feature-source-ideas-required | error | Requires every Feature README to carry a **Source Ideas:** line with an explicit sentinel (— / none) or a slug list; --fix backfills the sentinel. |

## feature-index

| Rule | Severity | Description |
| --- | --- | --- |
| feature-index-row-sync | error | Requires each feature index row to match its feature file. |

## frontmatter

| Rule | Severity | Description |
| --- | --- | --- |
| footer-format-mirror | error | Requires the adherence-footer URL to match the frontmatter format: URL; --fix derives the footer from format:. |
| format-field | error | Requires every artifact to carry a frontmatter format: field matching its type's spec URL. |
| status-mirror | error | Requires a status-bearing artifact's frontmatter status: to mirror its body **Status:** line, and forbids status: on status-less types. |

## grade

| Rule | Severity | Description |
| --- | --- | --- |
| grade-placement | error | Requires the grade field to appear in the correct position. |
| grade-single-value | error | Requires exactly one grade value per artifact. |
| grade-value | error | Rejects grade values outside the allowed A-F set. |
| grade-values-shape | error | Requires the grade metadata field to have the canonical shape. |

## graph

| Rule | Severity | Description |
| --- | --- | --- |
| graph-id-equals-filename-stem | error | Requires a graph artifact id to equal its filename stem (a module id equals its directory name). |
| graph-id-kebab-case | error | Requires graph artifact ids to be bare lowercase kebab-case. |
| graph-no-module-prefix-in-id | error | Rejects a module prefix (dot) in a graph artifact id; the qualified form is computed, not stored. |
| graph-kind-valid | error | Requires a readable kind: that is one of module|entity|relationship|command|event|policy and matches its collection directory. |
| graph-no-owner-field | error | Rejects an owner: field; ownership is derived from placement. |
| graph-no-inline-structure | error | Rejects inline fields:/properties: in a graph artifact; structure lives in ModelSpec. |
| graph-reference-resolves | error | Requires qualified graph references (from/to/subject/actors/participants/inputs.ref/possibleEvents) to resolve to existing artifacts. |
| graph-model-ref-resolves | error | Requires modelspec:// and HCL concept references to resolve per decisions 0007/0010/0011 (unknown module / unknown concept / kind mismatch / unavailable repository / bad grammar). |
| graph-model-legacy-form | error | Rejects the legacy modelspec://x.Y reference form (authority present, empty path); carries the exact modelspec:///x.Y rewrite that `graph lint --fix` applies (decision 0010). |
| graph-model-reserved-name | error | Rejects a ModelSpec concept named with a reserved kind token (entities, components, enums, collections, recordsets) (decision 0011). |
| graph-dependency-direction | error | Requires every cross-module reference (graph or model level) to target a module in the owning module's dependsOn. |
| graph-relationship-owner-covers-endpoints | error | Requires a relationship's owning module to cover both endpoint modules in its dependsOn closure. |
| graph-metadata-shape | error | Requires relationship metadata to be a flat map of scalar, qualified graph, or modelspec:// values. |
| graph-inputs-shape | error | Requires command inputs items to carry a kebab-case name plus exactly one of ref/model. |
| graph-rules-shape | error | Requires a Tier-1 rules: block on an entity/relationship/command artifact to be a list of {id, text, refs} maps with artifact-unique kebab-case ids, non-empty text, and resolvable refs (decision 0013). |
| graph-policy-shape | error | Requires a policy artifact to carry an applies: block naming exactly one of command|entity|relationship plus well-formed when/requires/invariant clauses whose operands resolve against the graph — inputs, lifecycle states, roles, and ModelSpec model properties (decision 0013). |
| graph-event-sources | warning | Warns on an explicitly empty sources: [] and rejects command references in event sources. |
| graph-lifecycle-states | error | Requires a declared lifecycle.states list to be non-empty and free of duplicates. |
| graph-duplicate-id | error | Rejects duplicate qualified graph IDs across the graph roots. |
| graph-duplicate-module-id | error | Rejects a GraphSpec module id declared by more than one graph root in the union (decision 0009). |
| graph-model-duplicate-concept | error | Rejects duplicate concept names within a module's ModelSpec sources. |
| graph-model-enum-values | error | Requires ModelSpec enum values to be non-empty and free of duplicates. |
| graph-role-labels | error | Requires role-labeled endpoints and participants to be well-formed {ref, role} maps with kebab-case role tokens (decision 0012). |
| graph-ambiguous-endpoints | warning | Warns on a self-referential relationship without endpoint role labels and on same-reference event participants lacking distinguishing roles (decision 0012). |
| graph-unknown-key | warning | Warns on frontmatter keys the artifact's placement-derived kind does not define (silently ignored by tooling, e.g. lifecycle: on a relationship). |
| graph-event-reachability | info | Reports an event that is in no command's possibleEvents and declares no sources — nothing can produce it. |

## idea

| Rule | Severity | Description |
| --- | --- | --- |
| idea-archive-reason | error | Requires an archived idea to record an archive reason. |
| idea-archived-index-chronological | error | Requires the archived idea index to be in chronological order. |
| idea-archived-location | error | Requires archived ideas to live under the archived directory. |
| idea-change-request-location | error | Requires change-request ideas to live in the expected location. |
| idea-feature-cross-reference | error | Requires idea-to-feature cross-references to resolve. |
| idea-header-fields | error | Requires the idea header to declare all mandatory fields. |
| idea-hmw-framing | error | Requires the idea to use How-Might-We framing. |
| idea-id-is-slug | error | Requires the idea Id field to match its filename slug. |
| idea-index-completeness | error | Requires the idea index to list every idea. |
| idea-index-row-sync | error | Requires each idea index row to match its idea file. |
| idea-location | error | Requires idea files to live under spec/ideas/. |
| idea-must-be-true-present | error | Requires the Must Be True section to be present. |
| idea-not-doing-non-empty | error | Requires the Not Doing section to be non-empty. |
| idea-phase-non-empty | error | Requires the idea Phase field to be non-empty. |
| idea-related-ideas-format | error | Requires the Related Ideas list to use the canonical format. |
| idea-related-ideas-target-exists | error | Requires every Related Ideas target to exist. |
| idea-required-sections | error | Requires every mandatory idea body section to be present. |
| idea-single-file | error | Requires each idea to be a single file, not a directory. |
| idea-slug-format | error | Requires idea filenames to use kebab-case slugs. |
| idea-specified-requires-promotion | error | Requires a Specified idea to record its promotion target. |
| idea-status-values | error | Rejects idea Status values outside the allowed set. |
| idea-supersedes-target-archived | error | Requires a superseded idea target to be archived. |
| idea-sync-lint-strict | error | Enforces strict synchronization between idea fields and index. |
| idea-targets-exists | error | Requires every declared idea target to exist. |
| idea-targets-required | error | Requires a change-request idea to declare its targets. |
| idea-title-format | error | Requires the idea H1 title to follow the canonical format. |
| idea-type-title-consistency | error | Requires the idea Type to be consistent with its title. |
| idea-type-values | error | Rejects idea Type values outside the allowed set. |

## issue

| Rule | Severity | Description |
| --- | --- | --- |
| I-001 | error | Requires issue frontmatter to contain all mandatory fields and no unknown keys. |
| I-002 | error | Rejects issue status values outside the allowed set. |
| I-003 | error | Validates the shape of optional issue frontmatter fields. |
| I-004 | error | Requires the issue slug to match its filename. |
| I-005 | error | Requires severity to be set once an issue leaves the open status. |
| I-006 | error | Rejects rejection_reason values outside the allowed set. |
| I-007 | error | Requires a rejection reason when an issue is rejected. |
| I-008 | error | Validates the shape of the issue bugs list. |
| I-009 | error | Rejects an issue that exists in two locations at once. |
| I-010 | error | Requires the issue captured_at field to be a valid timestamp. |
| I-011 | error | Requires the issue captured_by field to be non-empty. |
| I-012 | error | Requires resolved issues to record resolution metadata. |
| I-013 | error | Requires issue cross-references to resolve. |
| I-014 | error | Requires the issue index to list every issue. |
| I-015 | error | Requires the issue body to contain its mandatory sections. |

## lesson

| Rule | Severity | Description |
| --- | --- | --- |
| L-001 | error | Requires every Lesson body to declare the required section set for its canonical-directory or compatibility-flat layout. |
| L-002 | error | Requires a Lesson's body **Status:** value to be one of the canonical enforcement-ladder statuses (Recorded, Stated, Enforced, Withdrawn, Superseded). |
| L-003 | error | Requires the lessons index to list every lesson; --fix inserts missing rows. |
| L-004 | error | Requires each lessons index row to match the layout-specific Lesson projection; --fix regenerates drifted rows. |
| L-005 | error | Requires canonical directory Lessons to carry ordered metadata and repository-controlled classifications. |
| L-006 | error | Requires canonical Tracking to declare the append-only occurrence store, derived recurrence, and published schema URL. |
| L-007 | error | Requires Enforced Lessons to declare deterministic control, verification, and stable evidence. |
| L-008 | error | Requires duplicate and supersession relations to resolve without conflicts or cycles. |
| L-009 | error | Requires every canonical occurrence child to satisfy the published append-only JSON contract. |

## plan

| Rule | Severity | Description |
| --- | --- | --- |
| P-001 | error | Requires every plan task to reference at least one feature AC. |
| P-002 | error | Requires plan tasks to declare valid dependency references. |
| P-003 | error | Requires the plan to follow the single-file structure contract. |
| P-004 | error | Requires plan metadata to be complete and well-formed. |
| P-005 | error | Validates a plan's **Parent:** reference — same-repo parents resolve, are acyclic, and not self-referential; cross-repo <repo>:<slug> refs are checked syntactically only. |
| P-006 | error | Requires a single-file plan's body **Status:** value to be one of the canonical Plan statuses (Draft, In Review, Approved, Executing, Blocked, Implemented, Failed, Rejected, Withdrawn, Superseded, Deprecated). |
| P-007 | error | Derives a single-file plan's execution-band **Status:** (Executing/Blocked/Implemented/Failed) from its task-status rollup when the body status is Approved or an execution band; --fix reconciles drift. Prep and disposition statuses are never overwritten. |
| P-008 | error | Validates a task's optional **Implemented-by:** implementation-commit provenance value against the provenance-ref-format (<repo>@<sha> or bare <sha>, optional trailing branch; <sha> = 7-40 hex chars) — syntactic only, never scans the referenced repo. |
| P-009 | error | Validates optional same-repository **Prerequisite Plans:** references: canonical slugs, existence, no duplicates or self-references, and an acyclic dependency graph. |
| P-010 | error | Validates a plan's optional **Coordination:** reference against coordination-branch-format (<owner>/<repo>@<branch>) — syntactic only, never resolves or scans the named repo/branch. |
| plan-index-sync | error | Requires the canonical plans index table to match single-file Plan metadata; --fix regenerates drifted rows. |

## property

| Rule | Severity | Description |
| --- | --- | --- |
| property-checks-shape | error | Requires the property checks block to have the canonical shape. |
| property-data-type-values | error | Rejects property data_type values outside the allowed set. |
| property-frontmatter-required | error | Requires property files to carry frontmatter. |
| property-frontmatter-required-fields | error | Requires property frontmatter to declare all mandatory fields. |
| property-id-equals-slug | error | Requires the property id to match its filename slug. |
| property-location | error | Requires property files to live in the expected location. |
| property-referenced-by-managed | error | Requires the property Referenced by section to be managed. |
| property-required-sections | error | Requires every mandatory property body section to be present. |
| property-single-file | error | Requires each property to be a single file, not a directory. |
| property-slug-format | error | Requires property filenames to use kebab-case slugs. |
| property-title-format | error | Requires the property H1 title to follow the canonical format. |

## sidekick

| Rule | Severity | Description |
| --- | --- | --- |
| sidekick-seed | error | Requires sidekick seed files to be well-formed. |
