---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: Feature Change-Status

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/feature/change-status?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/feature/change-status?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/feature/change-status?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/feature/change-status?op=request-change) |
>
> **AI skill:** _planned_ — a `skills/feature/references/change-status.md` reference in [`ai-plugin-specscore`](https://github.com/specscore/ai-plugin-specscore) will follow shipping this verb; the skill invokes `specscore feature change-status` directly per the [lifecycle-transitions](../../lifecycle-transitions/README.md) contract.

**Status:** Stable
**Source Ideas:** lifecycle-verbs-for-idea-and-feature

## Summary

`specscore feature change-status <feature_id> --to=<status>` transitions a Feature artifact from its current `**Status:**` to the target status named by `--to`. Implements the [lifecycle-transitions](../../lifecycle-transitions/README.md) shared contract. Unlike the Idea kind's `change-status`, this verb has no file-relocation side effects — all transitions are pure status rewrites with index sync.

## Synopsis

```
specscore feature change-status <feature_id> --to=<status> [--project <path>]
```

## Problem

Today, transitioning a Feature through its lifecycle (Draft → In Review → Approved → Implementing → Stable → Deprecated) is a sequence of hand-edits with no validation. A hand-edit can drop a Stable feature to Draft, skip the review phase, or implement an unapproved feature with no warning. This verb closes the gap with a single command per kind, enforcing the legal-transition matrix.

## Behavior

This verb inherits every cross-cutting rule from [lifecycle-transitions](../../lifecycle-transitions/README.md). The REQs below are verb-specific: the Feature legal-transition matrix, the `--to` flag, the kind-specific identifier resolution, and the index-sync rule.

### Legal-transition matrix

Only the transitions in the table below are accepted. Any other `(from, to)` pair exits `4` (InvalidTransition).

| From | To | Side effects |
|---|---|---|
| `Draft` | `In Review` | Status rewrite + features-index sync |
| `Draft` | `Approved` | Status rewrite + features-index sync |
| `Draft` | `Deprecated` | Status rewrite + features-index sync |
| `In Review` | `Approved` | Status rewrite + features-index sync |
| `In Review` | `Rejected` | Status rewrite + features-index sync |
| `Approved` | `Implementing` | Status rewrite + features-index sync |
| `Approved` | `Amending` | Status rewrite + features-index sync |
| `Approved` | `Rejected` | Status rewrite + features-index sync |
| `Approved` | `Deprecated` | Status rewrite + features-index sync |
| `Implementing` | `Stable` | Status rewrite + features-index sync |
| `Implementing` | `Deprecated` | Status rewrite + features-index sync |
| `Stable` | `Amending` | Status rewrite + features-index sync |
| `Amending` | `Stable` | Status rewrite + features-index sync |
| `Amending` | `Approved` | Status rewrite + features-index sync |
| `Stable` | `Deprecated` | Status rewrite + features-index sync |

The `Draft → Approved` direct path is permitted: not every Feature requires a review phase. `Draft → Deprecated` and `Implementing → Deprecated` are also direct paths: a Feature abandoned before or during implementation MUST be honestly retirable without first detouring through `Stable` — the only prior route to `Deprecated`.

`Approved → Amending`, `Amending → Approved`, `Approved → Rejected`, and `Approved → Deprecated` close a gap where an agreed-but-unbuilt Feature (`Approved`, no code written from it yet) had exactly one legal exit (`→ Implementing`): a design that was approved and then needed to change, be cancelled, or be retired before any code existed had nowhere legal to go. `Approved → Amending` covers reworking that design in place; `Amending → Approved` returns it to the band it came from once the rework is resolved — `Amending` MUST NOT force a return through `Stable`, which would claim an implementation that was never built. (`Stable → Amending → Stable` remains the parallel arc for a design that already has a working implementation.) `Approved → Rejected` covers cancelling an agreed-but-unbuilt design outright; `Approved → Deprecated` covers retiring one. Reverse transitions into the pre-approval bands (e.g., `Approved → Draft`, `Deprecated → Stable`) are still NOT in the matrix and exit `4`. They MAY land in a follow-on revision once concrete reuse patterns surface.

#### REQ: legal-transition-matrix

The verb MUST accept only `(from, to)` pairs listed above. Any other pair MUST exit `4` (InvalidTransition) per [lifecycle-transitions#req:state-machine-strictness](../../lifecycle-transitions/README.md#req-state-machine-strictness), with a stderr message naming both the current status and the legal target statuses from the current state.

### Target-status flag

#### REQ: target-status-flag

The verb MUST accept the target status via a required `--to=<status>` flag. The flag value MUST be a recognized Feature status (`Draft`, `In Review`, `Approved`, `Implementing`, `Stable`, `Amending`, `Rejected`, `Deprecated`); unrecognized values exit `2` (InvalidArgs). Flag value matching is case-insensitive; the canonical title-case value is what gets written. Multi-word values use shell quoting: `--to="In Review"`.

### Kind-specific identifier resolution

#### REQ: feature-id-resolves-to-spec-features

The `<feature_id>` positional argument MUST resolve to a directory at `spec/features/<feature_id>/` containing a `README.md` file within the project root. The identifier may include slashes for nested features (e.g., `cli/idea/change-status`), matching `feature info <feature_id>` precedent. Missing → exit `3`. The Feature kind has no archived-equivalent location; the Meta's [REQ: slug-resolves-to-existing-artifact](../../lifecycle-transitions/README.md#req-slug-resolves-to-existing-artifact) reduces to the canonical-path lookup.

### Index sync target

#### REQ: features-index-sync

The post-mutation `specscore spec lint --fix` invocation MUST rely on the `feature-index-row-sync` rule (per [features-index#req:index-row-tracks-feature](https://github.com/specscore/specscore/blob/main/spec/features/features-index/README.md#req-index-row-tracks-feature) in the meta-spec) to rewrite the row for `<feature_id>` in `spec/features/README.md` to the new status. The lint rule does not exist in `pkg/lint/` today (only `idea-index-row-sync` does); implementing this verb requires landing an analogous `feature-index-row-sync` rule. Implementation of the verb and the rule MUST land together.

## Parameters

| Name | Required | Description |
|---|---|---|
| `feature_id` | Yes | Feature identifier — identifies `spec/features/<feature_id>/README.md`. Slashes allowed for nested features. |

## Flags

| Flag | Required | Description |
|---|---|---|
| `--to` | Yes | Target status. Legal values: `in review`, `approved`, `implementing`, `stable`, `amending`, `rejected`, `deprecated` (case-insensitive; `draft` is not a legal target — there is no transition INTO `Draft`). |
| `--project` | No | Project root. Autodetected per [CLI#req:project-autodetect](../../README.md#req-project-autodetect). |

## Exit codes

| Code | Condition |
|---|---|
| `0` | Transition succeeded; file rewritten; features-index synced. |
| `2` | Missing or malformed `<feature_id>`, missing `--to`, or unrecognized `--to` value. |
| `3` | No Feature directory or README at `spec/features/<feature_id>/`. |
| `4` | `(current_status, --to)` is not a legal transition per the matrix above. |
| `10` | I/O failure or `spec lint --fix` failed after a successful rewrite (rollback applied). |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [lifecycle-transitions](../../lifecycle-transitions/README.md) | Defines every cross-cutting REQ this verb satisfies. Also implements the Meta's [`--dry-run`](../../lifecycle-transitions/README.md#req-dry-run-mode) and [`transitions`](../../lifecycle-transitions/README.md#req-transitions-query-verb) verbs. |
| [feature (CLI group)](../README.md) | Parent group. Contents table includes this sub-feature. |
| [cli/idea/change-status](../../idea/change-status/README.md) | Sibling verb for the Idea kind. Same shared contract; Feature has no file-relocation side effect. |
| [spec lint](../../spec/lint/README.md) | Invoked internally. The `feature-index-row-sync` lint rule (to be added to `pkg/lint/`) is the specific rule that syncs this verb's effect. |
| [feature](../../../feature/README.md) | Source of truth for the Feature document structure and status enumeration. |
| Source Idea: [lifecycle-verbs-for-idea-and-feature](../../../../ideas/lifecycle-verbs-for-idea-and-feature.md) | Specifies `change-status` as the single Feature-kind lifecycle verb. |

## Acceptance Criteria

### AC: draft-to-under-review-happy-path

**Requirements:** [cli/feature/change-status#req:legal-transition-matrix](#req-legal-transition-matrix), [cli/feature/change-status#req:target-status-flag](#req-target-status-flag), [lifecycle-transitions#req:status-line-rewrite](../../lifecycle-transitions/README.md#req-status-line-rewrite), [lifecycle-transitions#req:index-sync-on-success](../../lifecycle-transitions/README.md#req-index-sync-on-success), [lifecycle-transitions#req:success-output-format](../../lifecycle-transitions/README.md#req-success-output-format)

Given `spec/features/auth/README.md` containing `**Status:** Draft`, running `specscore feature change-status auth --to="in review"` exits `0`, writes exactly `auth: Draft → In Review\n` to stdout, rewrites the Status line, and syncs the features-index row.

### AC: draft-direct-to-approved-happy-path

**Requirements:** [cli/feature/change-status#req:legal-transition-matrix](#req-legal-transition-matrix)

Given `spec/features/auth/README.md` containing `**Status:** Draft`, running `specscore feature change-status auth --to=approved` exits `0`. The direct `Draft → Approved` transition is legal; the review phase is optional.

### AC: under-review-to-approved-happy-path

**Requirements:** [cli/feature/change-status#req:legal-transition-matrix](#req-legal-transition-matrix)

Given `spec/features/auth/README.md` containing `**Status:** In Review`, running `specscore feature change-status auth --to=approved` exits `0`, with stdout `auth: In Review → Approved\n`.

### AC: implementing-to-stable-happy-path

**Requirements:** [cli/feature/change-status#req:legal-transition-matrix](#req-legal-transition-matrix)

Given `spec/features/auth/README.md` containing `**Status:** Implementing`, running `specscore feature change-status auth --to=stable` exits `0`, with stdout `auth: Implementing → Stable\n`.

### AC: stable-to-deprecated-happy-path

**Requirements:** [cli/feature/change-status#req:legal-transition-matrix](#req-legal-transition-matrix)

Given `spec/features/auth/README.md` containing `**Status:** Stable`, running `specscore feature change-status auth --to=deprecated` exits `0`, with stdout `auth: Stable → Deprecated\n`.

### AC: draft-to-deprecated-happy-path

**Requirements:** [cli/feature/change-status#req:legal-transition-matrix](#req-legal-transition-matrix)

Given `spec/features/auth/README.md` containing `**Status:** Draft`, running `specscore feature change-status auth --to=deprecated` exits `0`, with stdout `auth: Draft → Deprecated\n`. A Feature abandoned before review or approval can be honestly retired without a detour through `Stable`.

### AC: implementing-to-deprecated-happy-path

**Requirements:** [cli/feature/change-status#req:legal-transition-matrix](#req-legal-transition-matrix)

Given `spec/features/auth/README.md` containing `**Status:** Implementing`, running `specscore feature change-status auth --to=deprecated` exits `0`, with stdout `auth: Implementing → Deprecated\n`. A Feature abandoned mid-implementation can be honestly retired without first reaching `Stable`.

### AC: approved-to-amending-happy-path

**Requirements:** [cli/feature/change-status#req:legal-transition-matrix](#req-legal-transition-matrix)

Given `spec/features/auth/README.md` containing `**Status:** Approved`, running `specscore feature change-status auth --to=amending` exits `0`, with stdout `auth: Approved → Amending\n`. An agreed-but-unbuilt design can be reworked in place without first detouring through `Implementing`.

### AC: amending-to-approved-happy-path

**Requirements:** [cli/feature/change-status#req:legal-transition-matrix](#req-legal-transition-matrix)

Given `spec/features/auth/README.md` containing `**Status:** Amending` (reached via `Approved → Amending`), running `specscore feature change-status auth --to=approved` exits `0`, with stdout `auth: Amending → Approved\n`. `Amending` returns to the band it was entered from; a Feature amended from `Approved` returns to `Approved`, not `Stable` — returning through `Stable` would claim an implementation that does not exist.

### AC: approved-to-rejected-happy-path

**Requirements:** [cli/feature/change-status#req:legal-transition-matrix](#req-legal-transition-matrix)

Given `spec/features/auth/README.md` containing `**Status:** Approved`, running `specscore feature change-status auth --to=rejected` exits `0`, with stdout `auth: Approved → Rejected\n`. An agreed-but-unbuilt design can be cancelled outright, not just driven forward into `Implementing`.

### AC: approved-to-deprecated-happy-path

**Requirements:** [cli/feature/change-status#req:legal-transition-matrix](#req-legal-transition-matrix)

Given `spec/features/auth/README.md` containing `**Status:** Approved`, running `specscore feature change-status auth --to=deprecated` exits `0`, with stdout `auth: Approved → Deprecated\n`. An agreed-but-unbuilt design can be retired without ever reaching `Implementing`.

### AC: nested-feature-id-resolves

**Requirements:** [cli/feature/change-status#req:feature-id-resolves-to-spec-features](#req-feature-id-resolves-to-spec-features)

Given `spec/features/cli/idea/change-status/README.md` in `**Status:** Draft`, running `specscore feature change-status cli/idea/change-status --to=approved` exits `0`. Slashes in `<feature_id>` are accepted and resolve as a path within `spec/features/`.

### AC: case-insensitive-to-flag

**Requirements:** [cli/feature/change-status#req:target-status-flag](#req-target-status-flag)

`specscore feature change-status auth --to=STABLE` and `--to=Stable` and `--to=stable` behave identically when the source state is `Implementing`. The file is always written with canonical title-case (`**Status:** Stable`).

### AC: illegal-transition-rejected

**Requirements:** [cli/feature/change-status#req:legal-transition-matrix](#req-legal-transition-matrix), [lifecycle-transitions#req:state-machine-strictness](../../lifecycle-transitions/README.md#req-state-machine-strictness)

Given `spec/features/auth/README.md` in `**Status:** Draft`, running `specscore feature change-status auth --to=implementing` (skipping `Approved`) exits `4` with a stderr message naming `Draft` as the current status and `In Review`, `Approved` as the legal targets. No file change. The same applies to `Draft → Stable`, `Approved → Stable`, `Stable → Approved`, and other illegal pairs.

### AC: reverse-transition-rejected

**Requirements:** [cli/feature/change-status#req:legal-transition-matrix](#req-legal-transition-matrix)

Given `spec/features/auth/README.md` in `**Status:** Stable`, running `specscore feature change-status auth --to=implementing` exits `4`. Reverse transitions are not in the matrix.

### AC: already-at-target-rejected

**Requirements:** [cli/feature/change-status#req:legal-transition-matrix](#req-legal-transition-matrix), [lifecycle-transitions#req:not-idempotent](../../lifecycle-transitions/README.md#req-not-idempotent)

Given `spec/features/auth/README.md` in `**Status:** Approved`, running `specscore feature change-status auth --to=approved` exits `4`. Re-running on the target state is illegal per the strict state-machine.

### AC: unrecognized-to-value-rejected

**Requirements:** [cli/feature/change-status#req:target-status-flag](#req-target-status-flag)

`specscore feature change-status auth --to=banana` exits `2` BEFORE any state-machine check, with stderr that `banana` is not a recognized Feature status. The same applies to `--to=archived` (Idea-only status, not in the Feature enumeration).

### AC: missing-feature-id-rejected

**Requirements:** [lifecycle-transitions#req:slug-positional](../../lifecycle-transitions/README.md#req-slug-positional)

Running `specscore feature change-status --to=approved` with no positional argument exits `2`.

### AC: missing-to-flag-rejected

**Requirements:** [cli/feature/change-status#req:target-status-flag](#req-target-status-flag)

Running `specscore feature change-status auth` (no `--to`) exits `2` with stderr stating that `--to` is required.

### AC: feature-not-found

**Requirements:** [cli/feature/change-status#req:feature-id-resolves-to-spec-features](#req-feature-id-resolves-to-spec-features)

Running `specscore feature change-status nonexistent --to=approved` where `spec/features/nonexistent/README.md` does not exist exits `3` with stderr naming the expected path.

### AC: lint-failure-rolls-back

**Requirements:** [lifecycle-transitions#req:rollback-on-lint-failure](../../lifecycle-transitions/README.md#req-rollback-on-lint-failure)

A `feature-index-row-sync` failure after a successful rewrite triggers rollback and exit `10`.

## Open Questions

- Should `feature change-status --to=approved` enforce a prior `In Review` for projects that nominally require review, via a `--require-review` flag or repo-config? Deferred; today both `Draft → Approved` and `In Review → Approved` are unconditionally accepted.
- Should `feature change-status --to=deprecated` require `--reason "<text>"` and/or `--successor <feature_id>` to record why and what supersedes? The source Idea Open Question. Still unresolved even after `Draft → Deprecated` and `Implementing → Deprecated` widened the matrix (2026-07-22): the verb has no `--note` flag today (unlike `sidekick change-status`, which designates its own negative arc reason-required via `lifecycle.GuardReason`), so all three routes into `Deprecated` remain reason-free. Lean: defer to a follow-on revision once usage demands it — adding `--note` is an additive, non-breaking change to this verb whenever it lands.
- Should reverse transitions (`feature undeprecate`, `feature unstabilize`) be added as legal pairs in a future revision, or remain hand-edit territory? Lean: defer.
- `change-status --help` rendering of the legal-transition matrix: same UX question as the Idea sibling's Outstanding Question. Lean: render it.

---
*This document follows the https://specscore.md/feature-specification*
