# Plan: Issue Rules Implementation

**Status:** Implemented
**Source Feature:** cli/spec/lint/issue-rules
**Date:** 2026-05-22
**Owner:** alexandertrakhimenok
**Supersedes:** —

## Summary

Implements the [`cli/spec/lint/issue-rules`](../features/cli/spec/lint/issue-rules/README.md) Feature in eight linearly-ordered tasks: artifact-type registry and parser scaffold, the four frontmatter/schema rules, two body-structure rules, the slug helper and two slug rules, the cross-artifact `affected_component` rule, and the three index rules (the last with `--fix` autofix scaffolding). All 22 source-Feature ACs are covered; none deferred.

## Approach

Task order mirrors the upstream contract Plan's discovery-first sequencing: Task 1 lands the `pkg/issue/` parser and path-pattern matchers so subsequent tasks have a working artifact-discovery substrate. Frontmatter rules (Tasks 2–4) land before body-structure rules (Task 5) so the test fixtures from earlier tasks can be reused with minimal extension. The slug helper (Task 6) precedes the affected-component cross-artifact rule (Task 7) because the latter relies on slug identity established by global-uniqueness in Task 6. Index rules consolidate into Task 8 because they share the existing Index Artifact lint infrastructure and the `--fix` scaffolder for `I-013`/`I-014` lives in one helper. The 1:1 mapping between this Feature's rule REQs (`I-001`–`I-015`) and the upstream contract REQs is preserved exactly — no consolidation or splitting.

## Tasks

### Task 1: Scaffold `pkg/issue/` parser, path patterns, registry registration, and rule `I-009`

**Status:** done
**Verifies:** cli/spec/lint/issue-rules#ac:type-registered, cli/spec/lint/issue-rules#ac:default-suite-includes-i-rules, cli/spec/lint/issue-rules#ac:rules-filter-by-id, cli/spec/lint/issue-rules#ac:dual-location-violation

Create `pkg/issue/parser.go` and `pkg/issue/types.go` mirroring `pkg/idea/` and `pkg/feature/`. Implement YAML frontmatter parsing for `type: issue` artifacts. Register the `issue` artifact type in the CLI's artifact-type registry alongside `idea`, `feature`, `plan`, `sidekick-seed`, `index` — the entry binds the type to the two path patterns (`spec/issues/*.md`, `spec/features/*/issues/*.md`) and the `I-` rule family. Register all 15 `I-` rule IDs as stubs (each returning no violations) so the default-suite and filter-by-id ACs pass before any rule logic is implemented. Implement rule `I-009` (dual-location) in this task because path-pattern enforcement is inherent to artifact discovery: any file declaring `type: issue` outside the two patterns emits a violation. Feature-scoped issues additionally require the parent Feature directory to exist (checked at path-match time via the existing Feature-directory listing helper). Add fixture tests under `pkg/lint/rules/issue/testdata/` for the four ACs.

### Task 2: Rules `I-001` (required fields) and `I-002` (status enum), plus unknown-frontmatter-key handling

**Status:** done
**Verifies:** cli/spec/lint/issue-rules#ac:valid-minimal-issue-passes, cli/spec/lint/issue-rules#ac:missing-required-field-violation, cli/spec/lint/issue-rules#ac:invalid-status-enum-violation, cli/spec/lint/issue-rules#ac:unknown-frontmatter-key-violation

Create `pkg/lint/rules/issue/schema.go`. Implement rule `I-001` to validate the five always-required frontmatter fields (`type`, `slug`, `status`, `captured_at`, `captured_by`); missing field → violation with a "missing required field" message template naming the field. Implement rule `I-002` to validate `status` is one of `{open, investigating, resolved, rejected}`; invalid value → violation listing the four valid values. Implement strict unknown-key handling: unknown frontmatter keys → rule `I-001` violation under a distinct "unknown field" message template (separate from missing-field) so violation taxonomy stays unambiguous when both occur on the same artifact. Validate that the frontmatter `slug` field equals the filename slug (basename minus `.md`) — this is part of `I-001`'s field-validity check, not yet rule `I-010` (which handles the same invariant via the slug helper in Task 6; both produce equivalent diagnostics). Extend the fixture tests to cover the four ACs.

### Task 3: Rules `I-003` (optional-field shapes) and `I-004` (`bugs` opaque)

**Status:** done
**Verifies:** cli/spec/lint/issue-rules#ac:optional-field-shape-violation, cli/spec/lint/issue-rules#ac:bugs-opaque-non-string-violation

Create `pkg/lint/rules/issue/optional.go` and `pkg/lint/rules/issue/bugs.go`. Implement rule `I-003` per-optional-field shape validation: `severity` enum (`low|medium|high|critical|unset`), `affected_component` / `first_seen` / `github_issue` as non-empty strings, `rejection_reason` / `rejection_notes` shape (presence/absence enforced by `I-006` in Task 4 — `I-003` only validates the type and non-emptiness when present). Absence of any optional field is valid; presence with malformed shape → violation naming the field and the violated constraint. Implement rule `I-004` to validate `bugs`: absence valid, empty list valid, non-list or list-with-non-string-element → violation. Lint MUST NOT resolve the string elements to bug artifacts (the `bug` artifact type does not yet exist in this MVP); the field is opaque by design. Extend fixture tests.

### Task 4: Rules `I-005` (severity-on-transition) and `I-006` (rejection reason)

**Status:** done
**Verifies:** cli/spec/lint/issue-rules#ac:severity-required-on-transition-violation, cli/spec/lint/issue-rules#ac:rejection-reason-enum-violation

Create `pkg/lint/rules/issue/lifecycle.go` — these two rules share state-conditional logic and naturally co-locate. Implement rule `I-005`: when `status ∈ {investigating, resolved, rejected}`, `severity` MUST be set to a value in `{low, medium, high, critical}` (not absent, not `unset`); violation when missing or `unset`. Implement rule `I-006` as a three-part check: (a) `status: rejected` requires `rejection_reason`; (b) `rejection_reason` MUST be absent when `status` is not `rejected`; (c) `rejection_reason` value MUST be one of the six valid enum values. `rejection_notes` MUST be absent when `rejection_reason` is absent (orphan-notes check). Extend fixture tests to exercise both rules including the disambiguation case where `status: rejected` with valid `rejection_reason` but missing `severity` triggers only `I-005` (not `I-006`).

### Task 5: Rules `I-007` (H1 title) and `I-008` (body sections)

**Status:** done
**Verifies:** cli/spec/lint/issue-rules#ac:h1-prefix-violation, cli/spec/lint/issue-rules#ac:body-section-order-violation

Create `pkg/lint/rules/issue/body.go`. Implement rule `I-007` to match the first H1 against `^# Issue: .+$` using the existing markdown-tree parser already used by other body-checks; non-match → violation. Implement rule `I-008` to validate the three required H2 sections (`## Description`, `## Steps to Reproduce`, `## Expected vs Actual`) in canonical order, each appearing exactly once with non-empty content (≥ 1 non-whitespace character below the heading). Additional H2 sections after the third are unconstrained. The rule emits distinct violation sub-types (missing, out-of-order, empty, duplicated) so future advisory grouping in the lint output is possible without rule-id changes. Extend fixture tests.

### Task 6: Slug helper (`pkg/slug.IssueSlug`) + rules `I-010` (slug-mismatch) and `I-011` (global slug uniqueness)

**Status:** done
**Verifies:** cli/spec/lint/issue-rules#ac:slug-helper-truncation, cli/spec/lint/issue-rules#ac:slug-mismatch-violation, cli/spec/lint/issue-rules#ac:slug-globally-unique-violation

Extend `pkg/slug/` with `IssueSlug(s string) string` — the canonical slug-derivation algorithm (lowercase Unicode casefolding → `[^a-z0-9]→-` → collapse `-` runs → trim → truncate-at-≤60-at-nearest-`-`-boundary). Reuse internal helpers from the existing sidekick-seed slug code where possible; do NOT duplicate. Implement rule `I-010` (filename-vs-frontmatter slug match) — equivalent to the field-validity check from Task 2 but registered under the `I-010` rule ID for consistent diagnostics. Implement rule `I-011` (global slug uniqueness): perform one corpus pass collecting `slug → []path` for every `issue` artifact across both location patterns; for any slug appearing in more than one path, emit a violation naming all colliding paths. The corpus scan integrates with the lint engine's existing pre-rule pass infrastructure. Add a unit test for `IssueSlug` exercising the truncation fixture from the contract Plan. Extend lint fixture tests for the two rules.

### Task 7: Rule `I-012` (affected_component cross-artifact validation)

**Status:** done
**Verifies:** cli/spec/lint/issue-rules#ac:affected-component-ref-violation

Create `pkg/lint/rules/issue/affected_component.go`. When `affected_component` is present on an `issue` artifact, validate that `spec/features/<value>/README.md` exists. Directory present without `README.md` is treated as nonexistent (matching the existing Source-Ideas reference-validation behavior). Reuse the existing Feature-reference lookup helper in the lint engine; do not duplicate the file-existence logic. Absence of `affected_component` is valid (Task 3 already covers shape; this task adds the cross-artifact reference check). Add fixture tests.

### Task 8: Rules `I-013`/`I-014` (index required) and `I-015` (index columns) + `--fix` scaffolder

**Status:** done
**Verifies:** cli/spec/lint/issue-rules#ac:root-index-required-violation, cli/spec/lint/issue-rules#ac:root-index-fix-scaffolds-readme, cli/spec/lint/issue-rules#ac:feature-scoped-index-required-violation, cli/spec/lint/issue-rules#ac:index-columns-violation

Create `pkg/lint/rules/issue/index.go` and `pkg/lint/rules/issue/index_fix.go`. Implement rule `I-013` requiring `spec/issues/README.md` when `spec/issues/` contains ≥ 1 `issue` artifact; the README MUST conform to the SpecScore Index Artifact schema (existing validator). Implement rule `I-014` requiring `spec/features/<feature-slug>/issues/README.md` under the same conformance contract. Implement rule `I-015` validating the Contents table columns in canonical order: `Slug`, `Title`, `Status`, `Severity`, `Captured`. Implement the `--fix` autofix for `I-013` and `I-014`: scaffold a minimal lint-clean README (`type: index`, `**Status:** Stable`, empty `## Contents` table with the canonical column headers, `## Open Questions: None at this time.`, adherence footer). Do NOT autofix `I-015` — column-shape errors usually mean the author chose a different schema; autofix would silently destroy intent. Extend fixture tests.

## Open Questions

- Whether to ship the `IssueSlug` helper as a public package method (allowing external SpecScore tooling — including `specstudio:sidekick` — to compute canonical slugs without re-implementing the algorithm) or keep it internal to the lint engine. Lean: public, mirroring the existing `pkg/slug` API.
- Whether `--fix` for `I-013`/`I-014` should additionally populate the Contents table with rows for any pre-existing issue artifacts in the directory, or scaffold only an empty table. Lean: empty table — matches the existing index-autofix behavior elsewhere and avoids partial-update risk.
- Performance characterization of the `I-011` corpus scan once large issue trees exist in the wild — deferred until observed.

---
*This document follows the https://specscore.md/plan-specification*
