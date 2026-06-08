---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Feature Lint Rules

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/feature-rules?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/feature-rules?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/feature-rules?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/feature-rules?op=request-change) |
**Status:** Approved
**Date:** 2026-06-07
**Owner:** alexander.trakhimenok
**Source Ideas:** —
**Supersedes:** —

## Summary

Adds feature-level lint rules to `specscore spec lint`. The first rule, `feature-source-ideas-required`, enforces that every Feature README carries a `**Source Ideas:**` body-metadata line — with `—`/`none` as the explicit-empty sentinel — and `--fix` backfills the sentinel on Features that omit it. This is the CLI-side companion to the `specscore` repo's `Feature.source_ideas` required-with-explicit-`none` entity change (Feature 3 of the cross-repo-plan-composition Idea, referenced in prose only — cross-repo `source_ideas` does not exist yet).

## Problem

`Feature.source_ideas` was optional, so a Feature authored without an upstream Idea was indistinguishable on disk from one whose source was simply omitted. The entity now declares the field required-with-explicit-`none`, but nothing enforces presence: 32 of the 53 Feature READMEs in the `specscore` repo currently omit the `**Source Ideas:**` line entirely. Without a lint rule and a backfill migration, "specified without ideating" stays an implicit absence rather than a deliberate, checkable choice, and flipping the convention to required would break every Feature that omits the line.

## Behavior

### Source Ideas presence

#### REQ: rule-registered

The rule MUST be registered in the lint rule registry under the name `feature-source-ideas-required` (lowercase, hyphenated), at severity `error`, MUST execute as part of the default rule suite, and MUST be accepted by `--rules` / `--ignore` and `lint.AllRuleNames()`.

#### REQ: source-ideas-line-required

Every Feature README (`spec/features/**/README.md` recognized as a Feature by its `# Feature: <title>` H1) MUST carry a `**Source Ideas:**` body-metadata line. A Feature README that omits the line entirely MUST be reported as a violation citing the Feature path and the body-metadata block.

#### REQ: explicit-empty-sentinel

The `**Source Ideas:**` value MUST be one of: the explicit-empty sentinel `—` (em-dash) or `none` (case-insensitive), OR a non-empty comma-separated list of Idea slugs. A `**Source Ideas:**` line present but with an empty value (nothing after the marker) MUST be a violation — presence alone is insufficient; the choice must be explicit.

#### REQ: no-overlap-with-idea-sync

This rule validates **presence and sentinel only**. It MUST NOT resolve listed Idea slugs to existing Idea artifacts, validate feature↔idea bidirectional linkage, or duplicate findings already owned by the `idea-sync-lint-strict` / related-ideas rules. Slug resolution and drift detection remain those rules' responsibility.

#### REQ: autofix-backfill

`specscore spec lint --fix` MUST insert `**Source Ideas:** —` into any Feature README that omits the line, placing it in the body-metadata header block immediately after the `**Status:**` line (consistent with the `Status` → `Source Ideas` → `Grade` ordering enforced elsewhere). A Feature that already carries a `**Source Ideas:**` line (with any value, including a list) MUST be left untouched. The set of modified files MUST be reported in the `--fix` summary like every other autofix.

## Acceptance Criteria

### AC: missing-line-flagged

**Requirements:** cli/spec/lint/feature-rules#req:source-ideas-line-required, cli/spec/lint/feature-rules#req:rule-registered

**Given** a Feature README whose body-metadata block has no `**Source Ideas:**` line,
**When** `specscore spec lint` runs,
**Then** lint exits non-zero and a single `feature-source-ideas-required` violation is emitted citing that Feature's path.

### AC: explicit-empty-accepted

**Requirements:** cli/spec/lint/feature-rules#req:explicit-empty-sentinel

**Given** a Feature README carrying `**Source Ideas:** —` (and a second carrying `**Source Ideas:** none`),
**When** `specscore spec lint` runs,
**Then** neither Feature produces a `feature-source-ideas-required` violation.

### AC: slug-list-accepted

**Requirements:** cli/spec/lint/feature-rules#req:explicit-empty-sentinel, cli/spec/lint/feature-rules#req:no-overlap-with-idea-sync

**Given** a Feature README carrying `**Source Ideas:** some-idea, other-idea`,
**When** `specscore spec lint` runs,
**Then** no `feature-source-ideas-required` violation is emitted, and the rule does not itself attempt to resolve `some-idea` / `other-idea` to Idea artifacts.

### AC: empty-value-flagged

**Requirements:** cli/spec/lint/feature-rules#req:explicit-empty-sentinel

**Given** a Feature README with a `**Source Ideas:**` line whose value is empty (nothing after the marker),
**When** `specscore spec lint` runs,
**Then** a `feature-source-ideas-required` violation is emitted citing the empty value.

### AC: autofix-backfills-missing-only

**Requirements:** cli/spec/lint/feature-rules#req:autofix-backfill

**Given** a project with one Feature missing the `**Source Ideas:**` line and one Feature already carrying `**Source Ideas:** existing-idea`,
**When** `specscore spec lint --fix` runs,
**Then** the missing-line Feature gains `**Source Ideas:** —` immediately after its `**Status:**` line, the already-present Feature is left byte-for-byte unchanged, both files' modification status is reported in the `--fix` summary, and a subsequent `specscore spec lint` reports no `feature-source-ideas-required` violations.

### AC: rule-in-default-suite

**Requirements:** cli/spec/lint/feature-rules#req:rule-registered

**Given** a project with one Feature missing the `**Source Ideas:**` line,
**When** `specscore spec lint` runs with no `--rules` filter, and separately with `--rules feature-source-ideas-required`,
**Then** both runs surface the violation, its `Rule` field equals `feature-source-ideas-required`, its `Severity` is `error`, and an unknown-rule name still exits `2`.

## Open Questions

- **Sentinel canonicalization.** Both `—` and `none` are accepted as explicit-empty, but `--fix` backfills `—` (the established SpecScore em-dash sentinel, matching `**Supersedes:** —`). Whether to also normalize an authored `none` to `—` (or leave authored values untouched) is deferred — the MVP backfills only *missing* lines and never rewrites an existing value.
- **Migration scope.** The first `--fix` run against the `specscore` repo will backfill ~32 Feature READMEs in one pass. Whether that lands as a single mechanical migration commit or is split per-subtree is an operational choice for the implementing plan, not a spec concern.

---
*This document follows the https://specscore.md/feature-specification*
