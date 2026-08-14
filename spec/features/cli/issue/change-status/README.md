---
format: https://specscore.md/feature-specification
status: Implementing
---

# Feature: Issue Change-Status

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/issue/change-status?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/issue/change-status?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/issue/change-status?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/issue/change-status?op=request-change) |

**Status:** Implementing
**Source Ideas:** —

## Summary

`specscore issue change-status <slug> --to=<status>` transitions an Issue artifact from its current `status` frontmatter field to the target status named by `--to`. Implements the [lifecycle-transitions](../../lifecycle-transitions/README.md) shared contract; extends it with severity-gating and rejection-reason enforcement.

## Synopsis

```
specscore issue change-status <slug> --to=<status> [--severity <level>] [--reason <enum>] [--notes <text>] [--project <path>]
```

## Problem

Today, transitioning an Issue's status is a hand-edit of the `status` frontmatter field plus `specscore spec lint --fix` for index sync. Hand-edits skip the transition-graph validation (you can go `resolved → open` with no warning), forget severity-gating (lint will catch it post-hoc but not at transition time), and have no machine-readable contract. This verb closes the gap with a single command.

## Behavior

This verb inherits every cross-cutting rule from [lifecycle-transitions](../../lifecycle-transitions/README.md). The REQs below are the verb-specific declarations.

### Legal-transition matrix

Only the transitions in the table below are accepted. Any other `(from, to)` pair exits `4` (InvalidTransition).

| From | To |
|---|---|
| `open` | `investigating` |
| `open` | `resolved` |
| `open` | `rejected` |
| `investigating` | `resolved` |
| `investigating` | `rejected` |

`resolved` and `rejected` are terminal states with no outbound transitions.

#### REQ: legal-transition-matrix

The verb MUST accept only `(from, to)` pairs listed in the table above. Any other pair MUST exit `4` (InvalidTransition), with a stderr message naming both the current status and the legal target statuses from the current state.

### Target-status flag

#### REQ: target-status-flag

The verb MUST accept the target status via a required `--to=<status>` flag. The flag value MUST be one of `investigating`, `resolved`, `rejected`. Unrecognized values MUST exit `2` (InvalidArgs) BEFORE state-machine validation.

### Slug resolution

#### REQ: slug-resolves-to-existing-issue

The `<slug>` positional argument MUST resolve to a file at `spec/issues/<slug>.md` or `spec/features/*/issues/<slug>.md` within the project root. The command MUST search both locations. If found in neither, MUST exit `3` (NotFound). If found in both (which would be a lint violation per `I-011`), the command MUST exit `2` with an ambiguity error.

### Severity gating

#### REQ: severity-required-on-transition

When transitioning to `investigating`, `resolved`, or `rejected`: if the Issue's current `severity` frontmatter field is absent or `unset`, the `--severity <level>` flag MUST be required. Valid values: `low`, `medium`, `high`, `critical`. If the Issue already has a non-`unset` severity, the flag is optional (and if supplied, overwrites the existing value). Missing severity when required MUST exit `2` with a message stating that severity must be specified.

### Rejection gating

#### REQ: rejection-reason-required

When `--to=rejected`, the `--reason <enum>` flag MUST be required. Valid values: `not-a-defect`, `wont-fix`, `duplicate`, `not-reproducible`, `by-design`, `deferred`. Invalid values MUST exit `2`. The optional `--notes <text>` flag, when supplied, sets the `rejection_notes` frontmatter field. When `--to` is NOT `rejected`, `--reason` and `--notes` MUST be rejected with exit `2` if supplied.

### Mutation

#### REQ: frontmatter-rewrite

On valid transition, the verb MUST rewrite the `status` field in YAML frontmatter to the target value. If `--severity` is supplied, the `severity` field MUST be set (or added). If `--reason` is supplied, `rejection_reason` MUST be set (or added). If `--notes` is supplied, `rejection_notes` MUST be set (or added). All other frontmatter and body content MUST remain byte-identical.

### Post-mutation lint sync

#### REQ: lint-fix-after-transition

After a successful frontmatter rewrite, the verb MUST run `specscore spec lint --fix` to sync the corresponding index. On lint failure, the verb MUST roll back the file to its pre-invocation state and exit `10`.

## Parameters

| Name | Required | Description |
|---|---|---|
| `slug` | Yes | Issue slug — identifies the file across both location patterns. |

## Flags

| Flag | Required | Description |
|---|---|---|
| `--to` | Yes | Target status: `investigating`, `resolved`, `rejected`. |
| `--severity` | Conditional | Required when transitioning away from `open` and current severity is absent/unset. Values: `low`, `medium`, `high`, `critical`. |
| `--reason` | Conditional | Required when `--to=rejected`. One of: `not-a-defect`, `wont-fix`, `duplicate`, `not-reproducible`, `by-design`, `deferred`. |
| `--notes` | No | Free-form text for `rejection_notes`. Only valid with `--to=rejected`. |
| `--project` | No | Project root. Autodetected per CLI conventions. |

## Exit codes

| Code | Condition |
|---|---|
| `0` | Transition succeeded; frontmatter rewritten; index synced. |
| `2` | Missing or malformed `<slug>`, missing `--to`, unrecognized `--to` value, missing required `--severity` or `--reason`, `--reason`/`--notes` supplied when `--to` is not `rejected`. |
| `3` | No Issue file found at either location for the given slug. |
| `4` | `(current_status, --to)` is not a legal transition per the matrix. |
| `10` | I/O failure during rewrite or `spec lint --fix` (rollback applied). |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [lifecycle-transitions](../../lifecycle-transitions/README.md) | Defines every cross-cutting REQ this verb satisfies (atomicity, rollback, output format, exit codes). Also implements the Meta's [`--dry-run`](../../lifecycle-transitions/README.md#req-dry-run-mode) and [`transitions`](../../lifecycle-transitions/README.md#req-transitions-query-verb) verbs, wired by hand (Issue keeps its own small matrix rather than a `pkg/lifecycle.Kind`). |
| [issue (CLI group)](../README.md) | Parent group. Contents table includes this sub-feature. |
| [issue-artifact-type](https://github.com/specscore/specstudio-skills/blob/main/spec/features/issue-artifact-type/README.md) | Source of truth for the four-state lifecycle, severity-on-transition rule, and rejection-reason enum. |
| [cli/spec/lint/issue-rules](../../spec/lint/issue-rules/README.md) | Rules `I-005` (severity-on-transition) and `I-006` (rejection-reason) enforce the same constraints at lint time. This verb enforces them at mutation time as gate checks. |

## Acceptance Criteria

### AC: open-to-investigating-happy-path

**Requirements:** cli/issue/change-status#req:legal-transition-matrix, cli/issue/change-status#req:severity-required-on-transition

**Given** `spec/issues/foo.md` with `status: open` and no `severity` field
**When** `specscore issue change-status foo --to=investigating --severity high` is run
**Then** the command exits `0`, rewrites `status: investigating` and `severity: high` in frontmatter, and syncs the index

### AC: severity-already-set-no-flag-needed

**Requirements:** cli/issue/change-status#req:severity-required-on-transition

**Given** `spec/issues/bar.md` with `status: open` and `severity: medium`
**When** `specscore issue change-status bar --to=investigating` is run (no `--severity` flag)
**Then** the command exits `0` — existing severity satisfies the gate

### AC: missing-severity-rejected

**Requirements:** cli/issue/change-status#req:severity-required-on-transition

**Given** `spec/issues/baz.md` with `status: open` and no `severity` field
**When** `specscore issue change-status baz --to=investigating` is run (no `--severity` flag)
**Then** the command exits `2` with a message stating that `--severity` is required

### AC: rejected-requires-reason

**Requirements:** cli/issue/change-status#req:rejection-reason-required

**Given** `spec/issues/foo.md` with `status: open` and `severity: low`
**When** `specscore issue change-status foo --to=rejected` is run (no `--reason`)
**Then** the command exits `2` with a message stating that `--reason` is required when transitioning to rejected

### AC: rejected-happy-path-with-notes

**Requirements:** cli/issue/change-status#req:rejection-reason-required, cli/issue/change-status#req:frontmatter-rewrite

**Given** `spec/issues/foo.md` with `status: investigating` and `severity: high`
**When** `specscore issue change-status foo --to=rejected --reason duplicate --notes "duplicate of bar"` is run
**Then** the command exits `0`, frontmatter contains `status: rejected`, `rejection_reason: duplicate`, `rejection_notes: duplicate of bar`

### AC: reason-flag-rejected-when-not-rejecting

**Requirements:** cli/issue/change-status#req:rejection-reason-required

**Given** `spec/issues/foo.md` with `status: open` and `severity: low`
**When** `specscore issue change-status foo --to=investigating --reason wont-fix` is run
**Then** the command exits `2` with a message stating that `--reason` is only valid with `--to=rejected`

### AC: illegal-transition-rejected

**Requirements:** cli/issue/change-status#req:legal-transition-matrix

**Given** `spec/issues/foo.md` with `status: resolved`
**When** `specscore issue change-status foo --to=investigating` is run
**Then** the command exits `4` with a stderr message stating that `resolved` is a terminal state with no legal transitions

### AC: slug-not-found

**Requirements:** cli/issue/change-status#req:slug-resolves-to-existing-issue

**Given** no file exists at `spec/issues/nonexistent.md` or any `spec/features/*/issues/nonexistent.md`
**When** `specscore issue change-status nonexistent --to=investigating --severity low` is run
**Then** the command exits `3`

### AC: unrecognized-to-value-rejected

**Requirements:** cli/issue/change-status#req:target-status-flag

**Given** any valid project
**When** `specscore issue change-status foo --to=banana` is run
**Then** the command exits `2` with a message that `banana` is not a recognized Issue status

### AC: lint-failure-rolls-back

**Requirements:** cli/issue/change-status#req:lint-fix-after-transition

**Given** a transition that would succeed but the subsequent `specscore spec lint --fix` fails (e.g., corrupted index README)
**When** the verb runs
**Then** the command exits `10`, restores the original file content, and no frontmatter change persists

## Open Questions

- Should `change-status` support a `--reopen` escape-hatch for transitioning `resolved`/`rejected` back to `open` in exceptional circumstances?
- Should the verb accept `--to=open` from `investigating` for "false alarm" scenarios, or should the user reject with `--reason not-reproducible` instead?

---
*This document follows the https://specscore.md/feature-specification*
