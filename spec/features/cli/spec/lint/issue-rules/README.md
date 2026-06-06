---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Issue Lint Rules

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/issue-rules?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/issue-rules?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/issue-rules?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/issue-rules?op=request-change) |
**Status:** Approved

## Summary

Adds 15 lint rules (`I-001`–`I-015`) and the underlying `issue` artifact parser to `specscore spec lint`, implementing the contract reserved by the SpecStudio [`issue-artifact-type` Feature](https://github.com/specscore/specstudio-skills/blob/main/spec/features/issue-artifact-type/README.md) in the `specstudio-skills` repo. Companion to that contract: it adds parser detection for the new `issue` artifact, registers the `I-` rule family in the default lint suite, and emits violations with the rule IDs and messages the contract defines.

## Problem

The SpecStudio `issue-artifact-type` Feature introduces `issue` as a top-level SpecScore artifact (parallel to Ideas, Features, Plans, Tasks) with a fixed frontmatter schema, required body sections, dual-location convention (`spec/issues/<slug>.md` root + `spec/features/<feature-slug>/issues/<issue-slug>.md` Feature-scoped), a four-state lifecycle (`open|investigating|resolved|rejected`), and a forward-compatible reserved `bugs: []` field. None of that machinery exists in `specscore-cli` today: lint does not recognize `type: issue`, the `I-` rule namespace is unallocated, and the path patterns are unindexed. The upstream Feature is approved and its Plan ([`issue-artifact-type`](https://github.com/specscore/specstudio-skills/blob/main/spec/plans/issue-artifact-type.md)) is approved, but no code can ship until this CLI work lands.

This Feature is the CLI-side companion to the upstream contract. It adds: (a) the `issue` artifact-type parser, (b) the two path-pattern matchers, (c) the 15 lint rules covering schema / lifecycle / body / path / slug / cross-artifact / index / reserved-field validation, and (d) registration of the rules so `specscore spec lint` runs them as part of the default rule suite.

## Behavior

### Artifact detection

The lint engine must recognize files that declare themselves as `type: issue` in YAML frontmatter and route them through the `I-` rule family.

#### REQ: issue-artifact-type-registered

A new artifact type `issue` MUST be registered in the CLI's artifact-type registry alongside the existing types (`idea`, `feature`, `plan`, `sidekick-seed`, `index`). The registry entry MUST associate the type with the two path patterns below and with the `I-` rule family.

#### REQ: issue-path-patterns

The parser MUST consider two glob patterns when discovering issue artifacts: `spec/issues/*.md` (root-level) and `spec/features/*/issues/*.md` (Feature-scoped). Files outside these patterns that declare `type: issue` MUST trigger rule `I-009` (dual-location). Files inside these patterns that do NOT declare `type: issue` MUST be ignored by the `I-` rule family.

#### REQ: issue-default-rule-suite

All 15 `I-` rules MUST be registered such that `specscore spec lint` (no flags) executes them by default. The rules MUST be filterable via the existing `--rules` and `--ignore` flags using their canonical IDs (`I-001` through `I-015`).

### Rule family

The 15 rules correspond 1:1 to the REQs in the upstream contract. Each rule has a stable canonical ID, a human-readable message template, and a deterministic fix policy (`--fix` either resolves the violation or it doesn't; never partial).

#### REQ: rule-i-001-required-fields

Rule `I-001` MUST validate the always-required frontmatter fields per upstream REQ `issue-frontmatter-required-fields`: `type` (string, value `issue`), `slug` (string matching the filename slug), `status` (one of the four enum values), `captured_at` (RFC 3339 / ISO 8601 timestamp), `captured_by` (string). Missing field → violation naming the field. NOT auto-fixable.

#### REQ: rule-i-002-status-enum

Rule `I-002` MUST validate that the `status` field value is one of `open`, `investigating`, `resolved`, `rejected` per upstream REQ `issue-lifecycle-state-values`. Any other value (including legacy values like `triaged`, `closed`, `fixed`) → violation listing the four valid values. NOT auto-fixable.

#### REQ: rule-i-003-optional-field-shapes

Rule `I-003` MUST validate the shape of optional frontmatter fields per upstream REQ `issue-frontmatter-optional-fields`: `severity` enum, `affected_component` / `first_seen` / `github_issue` as non-empty strings, `rejection_reason` / `rejection_notes` shape. Absence is valid; presence with malformed shape → violation naming the field and the violated constraint. NOT auto-fixable.

#### REQ: rule-i-004-bugs-opaque

Rule `I-004` MUST validate the reserved `bugs` field per upstream REQ `issue-bugs-field-opaque`: YAML list whose every element is a string. Absence valid; empty list valid; non-list or list-with-non-string-element → violation. Lint MUST NOT resolve the strings to bug artifacts in this MVP (the `bug` artifact does not yet exist). NOT auto-fixable.

#### REQ: rule-i-005-severity-on-transition

Rule `I-005` MUST enforce upstream REQ `issue-severity-required-on-transition`: when `status` is `investigating`, `resolved`, or `rejected`, `severity` MUST be set to `low|medium|high|critical` (not absent, not `unset`). When `status: open`, severity is optional. NOT auto-fixable.

#### REQ: rule-i-006-rejection-reason

Rule `I-006` MUST enforce the three-part rejection-reason rule per upstream REQ `issue-rejection-reason-required`: (a) `status: rejected` requires `rejection_reason`; (b) `rejection_reason` MUST be absent when `status` is not `rejected`; (c) `rejection_reason` value MUST be one of `not-a-defect|wont-fix|duplicate|not-reproducible|by-design|deferred`. `rejection_notes` follows `rejection_reason` absence/presence. NOT auto-fixable.

#### REQ: rule-i-007-h1-title

Rule `I-007` MUST validate that the first H1 of every `issue` artifact matches `^# Issue: .+$` per upstream REQ `issue-h1-title`. NOT auto-fixable.

#### REQ: rule-i-008-body-sections

Rule `I-008` MUST validate body H2 sections per upstream REQ `issue-body-required-h2-sections`: exactly three required sections in canonical order (`## Description`, `## Steps to Reproduce`, `## Expected vs Actual`), each appearing exactly once with non-empty content. Additional H2 sections after the third allowed. NOT auto-fixable.

#### REQ: rule-i-009-dual-location

Rule `I-009` MUST enforce dual-location placement per upstream REQ `issue-dual-location`. Any file declaring `type: issue` outside the two patterns triggers a violation. Feature-scoped issues additionally require the parent Feature directory to exist — this derives from the upstream REQ's "where `<feature-slug>` is the directory name of an existing Feature" phrasing. The check happens at path-match time, separate from REQ `affected_component` (which validates the frontmatter field value, not the file's enclosing path). NOT auto-fixable.

#### REQ: rule-i-010-slug-mismatch

Rule `I-010` MUST validate that the filename slug equals the frontmatter `slug` field per upstream REQ `issue-slug-derivation`. NOT auto-fixable. The slug-derivation algorithm itself MUST be exposed as a pure helper in `pkg/slug` (reused with the existing sidekick-seed slug code) so external tooling can compute the canonical slug from a one-liner.

#### REQ: rule-i-011-slug-globally-unique

Rule `I-011` MUST enforce global slug uniqueness across both location patterns per upstream REQ `issue-slug-globally-unique`. Implementation: build a slug→paths map in one corpus pass; emit a violation per duplicate naming all colliding paths. NOT auto-fixable (manual merge or rename required).

#### REQ: rule-i-012-affected-component-ref

Rule `I-012` MUST validate that, when `affected_component` is present, `spec/features/<value>/README.md` exists per upstream REQ `issue-affected-component-validation`. Directory present without README is treated as nonexistent (matching existing Source-Ideas reference-validation behavior; reuses the Feature-reference helper in the lint engine). NOT auto-fixable.

#### REQ: rule-i-013-root-index-required

Rule `I-013` MUST require `spec/issues/README.md` when `spec/issues/` contains ≥1 issue artifact per upstream REQ `issue-root-index-required`. The README MUST conform to the SpecScore Index Artifact schema. Auto-fixable: `--fix` MAY scaffold a minimal lint-clean index README.

#### REQ: rule-i-014-feature-scoped-index-required

Rule `I-014` MUST require `spec/features/<feature-slug>/issues/README.md` when that directory contains ≥1 issue artifact per upstream REQ `issue-feature-scoped-index-required`. Same Index Artifact conformance. Same `--fix` policy as `I-013`.

#### REQ: rule-i-015-index-columns

Rule `I-015` MUST validate that each `issues/README.md` Contents table has the five required columns in order: `Slug`, `Title`, `Status`, `Severity`, `Captured` per upstream REQ `issue-index-contents-columns`. NOT auto-fixable (column-shape errors usually mean the author chose a different schema; auto-fix would silently destroy intent).

### Parser & file layout

#### REQ: parser-package-layout

The `issue` artifact parser MUST live at `pkg/issue/` (mirroring the existing `pkg/idea/`, `pkg/feature/`, `pkg/plan/` packages). The 15 rules MUST live at `pkg/lint/rules/issue/` and register through the existing rule-registration mechanism in `pkg/lint`. The slug helper MUST live in `pkg/slug/` (extending the existing package; do not duplicate).

#### REQ: parser-frontmatter-strict

The parser MUST reject unknown frontmatter keys on `issue` artifacts. Unknown keys raise rule `I-001` under a distinct "unknown field" message template (separate from the "missing required field" template) so violation taxonomy stays unambiguous when both occur on the same artifact. This mirrors how `sidekick-seed` lint handles unknown keys.

## Acceptance Criteria

### AC: type-registered

**Given** the CLI is built with this Feature's changes
**When** `specscore spec lint --help` is run and the implementation queries the artifact-type registry
**Then** the registry contains an entry for `issue` associated with the two path patterns (`spec/issues/*.md`, `spec/features/*/issues/*.md`) and the `I-` rule family

### AC: default-suite-includes-i-rules

**Given** a spec tree with at least one valid `issue` artifact
**When** `specscore spec lint` is run with no flags
**Then** lint executes all 15 `I-` rules against the tree

### AC: rules-filter-by-id

**Given** a spec tree with an issue that violates `I-005` and `I-007`
**When** `specscore spec lint --rules I-005` is run
**Then** only the `I-005` violation is emitted; `I-007` is suppressed

### AC: valid-minimal-issue-passes

**Given** a fixture issue at `spec/issues/foo.md` with the minimal required frontmatter (`type: issue`, `slug: foo`, `status: open`, `captured_at`, `captured_by`), valid H1 (`# Issue: Foo`), and the three required H2 sections each non-empty
**When** `specscore spec lint` is run
**Then** zero violations are emitted and the exit code is 0

### AC: missing-required-field-violation

**Given** a fixture issue missing the `captured_by` frontmatter field
**When** `specscore spec lint` is run
**Then** rule `I-001` emits a violation naming `captured_by`, exit code is 1

### AC: invalid-status-enum-violation

**Given** a fixture issue with `status: triaged`
**When** `specscore spec lint` is run
**Then** rule `I-002` emits a violation listing the four valid values

### AC: optional-field-shape-violation

**Given** a fixture issue with `severity: extreme` (a value outside the documented enum)
**When** `specscore spec lint` is run
**Then** rule `I-003` emits a violation listing the five valid `severity` values (`low`, `medium`, `high`, `critical`, `unset`)

### AC: severity-required-on-transition-violation

**Given** a fixture issue with `status: investigating` and no `severity` field
**When** `specscore spec lint` is run
**Then** rule `I-005` emits a violation naming severity-required-on-transition

### AC: rejection-reason-enum-violation

**Given** a fixture issue with `status: rejected`, valid `severity: low`, and `rejection_reason: not-real-enough`
**When** `specscore spec lint` is run
**Then** rule `I-006` emits a violation listing the six valid `rejection_reason` values

### AC: h1-prefix-violation

**Given** a fixture issue with H1 `# Bug: Menu crashes`
**When** `specscore spec lint` is run
**Then** rule `I-007` emits a violation stating the H1 must match `^# Issue: .+$`

### AC: body-section-order-violation

**Given** a fixture issue with the three required H2 sections in non-canonical order
**When** `specscore spec lint` is run
**Then** rule `I-008` emits a violation stating the required sections must appear in canonical order

### AC: dual-location-violation

**Given** a fixture file at `spec/random-dir/foo.md` declaring `type: issue`
**When** `specscore spec lint` is run
**Then** rule `I-009` emits a violation stating the artifact must live under `spec/issues/` or `spec/features/<slug>/issues/`

### AC: slug-mismatch-violation

**Given** a fixture issue at `spec/issues/foo.md` whose frontmatter `slug` is `bar`
**When** `specscore spec lint` is run
**Then** rule `I-010` emits a violation naming the mismatch

### AC: slug-globally-unique-violation

**Given** two fixture issues at `spec/issues/foo.md` and `spec/features/example/issues/foo.md`, both lint-valid in isolation
**When** `specscore spec lint` is run
**Then** rule `I-011` emits a violation naming both paths and the colliding slug `foo`

### AC: slug-helper-truncation

**Given** the one-liner `"The application crashes intermittently when the user navigates between menus quickly"`
**When** the `pkg/slug` helper's `IssueSlug(s)` function is invoked
**Then** it returns `"the-application-crashes-intermittently-when-the-user"` (truncated at the nearest preceding `-` boundary ≤ 60 chars)

### AC: affected-component-ref-violation

**Given** a fixture issue with `affected_component: nonexistent-feature` and no `spec/features/nonexistent-feature/README.md` exists
**When** `specscore spec lint` is run
**Then** rule `I-012` emits a violation stating the Feature reference does not resolve

### AC: bugs-opaque-non-string-violation

**Given** a fixture issue with `bugs: [123, "valid-slug"]`
**When** `specscore spec lint` is run
**Then** rule `I-004` emits a violation stating every element of `bugs` must be a string

### AC: root-index-required-violation

**Given** at least one issue in `spec/issues/` and no `spec/issues/README.md`
**When** `specscore spec lint` is run
**Then** rule `I-013` emits a violation requesting the index README

### AC: root-index-fix-scaffolds-readme

**Given** the same fixture as the previous AC
**When** `specscore spec lint --fix` is run
**Then** `spec/issues/README.md` is created with a lint-clean minimal index (type: index, Status: Stable, empty Contents table, Open Questions: None at this time., adherence footer) and the violation no longer appears on a follow-up lint run

### AC: feature-scoped-index-required-violation

**Given** at least one issue in `spec/features/example/issues/` and no `spec/features/example/issues/README.md`
**When** `specscore spec lint` is run
**Then** rule `I-014` emits a violation requesting the per-Feature index README

### AC: index-columns-violation

**Given** a `spec/issues/README.md` Contents table missing the `Severity` column
**When** `specscore spec lint` is run
**Then** rule `I-015` emits a violation listing the five required columns in order

### AC: unknown-frontmatter-key-violation

**Given** a fixture issue with an unknown frontmatter key `priority: high` alongside valid required fields
**When** `specscore spec lint` is run
**Then** rule `I-001` emits a violation under the "unknown field" category naming `priority`

## Open Questions

- Whether the slug-uniqueness corpus scan in rule `I-011` should be backed by an in-memory index file at scale (>1000 issues) — defer until performance becomes an observed issue.
- Whether `--fix` for `I-013` and `I-014` should also append a row to the index Contents table for every existing issue in the directory, or only scaffold the empty README — leaning empty README (matching the existing index-autofix behavior).

---
*This document follows the https://specscore.md/feature-specification*
