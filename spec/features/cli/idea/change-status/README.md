---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: Idea Change-Status

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/idea/change-status?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/idea/change-status?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/idea/change-status?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/idea/change-status?op=request-change) |
>
> **AI skill:** _planned_ — a `skills/idea/references/change-status.md` reference in [`ai-plugin-specscore`](https://github.com/specscore/ai-plugin-specscore) will follow shipping this verb; the skill invokes `specscore idea change-status` directly per the [lifecycle-transitions](../../lifecycle-transitions/README.md) contract.

**Status:** Stable
**Source Ideas:** lifecycle-verbs-for-idea-and-feature

## Summary

`specscore idea change-status <slug> --to=<status>` transitions an Idea artifact from its current `**Status:**` to the target status named by `--to`. Implements the [lifecycle-transitions](../../lifecycle-transitions/README.md) shared contract. Archival is **not** a status: filing an Idea out of active view is the separate, orthogonal [`idea archive`](../archive/README.md) verb, which keeps the Idea's terminal `**Status:**` and adds an `**Archived:** true` axis. `change-status` never relocates a file.

## Synopsis

```
specscore idea change-status <slug> --to=<status> [--project <path>]
```

## Problem

Today, transitioning an Idea's status is a hand-edit of the `**Status:**` line in `spec/ideas/<slug>.md` plus `specscore spec lint --fix` for index sync. Hand-edits skip state-machine validation (a hand-edit can drop `Specified` back to `Draft` with no warning), forget the lint sync (leaving the index stale), and have no machine-readable contract. This verb closes the gap with a single command per kind. (Archival — moving an Idea out of active view — is a separate orthogonal concern handled by [`idea archive`](../archive/README.md), not a status transition.)

## Behavior

This verb inherits every cross-cutting rule from [lifecycle-transitions](../../lifecycle-transitions/README.md). The REQs below are the verb-specific declarations: the Idea legal-transition matrix, the `--to` flag, the kind-specific slug resolution, and the index-sync behavior. The verb performs an in-place `**Status:**` rewrite only — it never relocates a file (archival is the orthogonal [`idea archive`](../archive/README.md) verb).

### Legal-transition matrix

Only the transitions in the table below are accepted. Any other `(from, to)` pair exits `4` (InvalidTransition) per [lifecycle-transitions#req:state-machine-strictness](../../lifecycle-transitions/README.md#req-state-machine-strictness). Status values are governed by [status-vocabulary#req:per-artifact-status-sets](https://github.com/specscore/specscore/blob/main/spec/features/status-vocabulary/README.md#req-per-artifact-status-sets): the Idea legal set is `Draft, In Review, Approved, Specifying, Specified, Implementing, Implemented, Rejected, Stale` (no `Archived` — archival is not a status).

| From | To | Side effects |
|---|---|---|
| `Draft` | `In Review` | Status rewrite + ideas-index sync |
| `Draft` | `Approved` | Status rewrite + ideas-index sync |
| `Draft` | `Stale` | Status rewrite + ideas-index sync |
| `In Review` | `Approved` | Status rewrite + ideas-index sync |
| `In Review` | `Rejected` | Status rewrite + ideas-index sync |
| `In Review` | `Stale` | Status rewrite + ideas-index sync |
| `Approved` | `Rejected` | Status rewrite + ideas-index sync |
| `Approved` | `Specifying` | Status rewrite + ideas-index sync |
| `Approved` | `Stale` | Status rewrite + ideas-index sync |
| `Specifying` | `Specified` | Status rewrite + ideas-index sync |
| `Specifying` | `Rejected` | Status rewrite + ideas-index sync |
| `Specifying` | `Stale` | Status rewrite + ideas-index sync |
| `Specified` | `Implementing` | Status rewrite + ideas-index sync |
| `Specified` | `Rejected` | Status rewrite + ideas-index sync |
| `Specified` | `Stale` | Status rewrite + ideas-index sync |
| `Implementing` | `Implemented` | Status rewrite + ideas-index sync |
| `Implementing` | `Rejected` | Status rewrite + ideas-index sync |
| `Implementing` | `Stale` | Status rewrite + ideas-index sync |

The human-authored prep band (`Draft → In Review → Approved`) and the disposition transitions (`→ Rejected` from review or from any of `Approved`/`Specifying`/`Specified`/`Implementing`, `→ Stale` from any pre-terminal state including `Implementing`) are author-managed. For **feature-request** ideas, the forward specification band (`Approved → Specifying → Specified → Implementing → Implemented`) is derived from Feature status by the `idea-sync-lint-strict` lint rule; the `change-status` verb is the user-facing surface for manual override or when a transition is needed ahead of the derivation rule. For **change-request** ideas, all transitions are author-managed (not derived) — the lint derivation rules skip change-request ideas entirely.

`Approved → Rejected` closes a gap where an agreed-but-not-yet-specified Idea (`Approved`, no Feature created from it yet) could only decay to `Stale` — never be actively turned down. This completes the same principle across the rest of the pre-terminal lifecycle: `Specifying → Rejected`, `Specified → Rejected`, and `Implementing → Rejected` let an Idea be actively turned down mid-specification or mid-build, not only before specification starts; `Implementing → Stale` closes the matching passive-decay gap — before this row, `Implementing`'s only exit was `→ Implemented`, so a build that simply petered out (nobody decided against it, work just stopped) had no legal terminal status to record that at all. `Rejected` and `Stale` are not interchangeable at any of these stages: `Rejected` is an active decision against the Idea, `Stale` is passive decay that nobody decided against (see [idea#req:terminal-disposition-transitions](https://github.com/specscore/specscore/blob/main/spec/features/idea/README.md#req-terminal-disposition-transitions)). An Idea that is deliberately cancelled — whether before specification starts, mid-specification, or mid-build — is `Rejected`, not `Stale`.

#### REQ: legal-transition-matrix

The verb MUST accept only `(from, to)` pairs listed in the legal-transition matrix above. Any other pair MUST exit `4` (InvalidTransition) per the Meta contract, with a stderr message naming both the current status and the legal target statuses from the current state.

### Target-status flag

#### REQ: target-status-flag

The verb MUST accept the target status via a required `--to=<status>` flag. The flag value MUST be a recognized Idea status; unrecognized values exit `2` (InvalidArgs) BEFORE state-machine validation. Flag value matching is case-insensitive on input (`--to=approved`, `--to=Approved`, `--to=APPROVED` all parse identically); the canonical title-case value is what gets written to the file and to the success-output line. A multi-word value MUST be supplied with shell quoting or hyphenation per cobra conventions (`--to="In Review"` or `--to='In Review'`).

### Kind-specific slug resolution

#### REQ: slug-resolves-to-active-idea

The `<slug>` positional argument MUST resolve to a file at `spec/ideas/<slug>.md` within the project root. Already-archived files at `spec/ideas/archived/<slug>.md` MUST NOT be matched per [lifecycle-transitions#req:slug-resolves-to-existing-artifact](../../lifecycle-transitions/README.md#req-slug-resolves-to-existing-artifact). A missing file at the active path MUST exit `3` (NotFound).

#### REQ: no-relocation

`change-status` MUST mutate only the `**Status:**` line in place (per the Meta's [`status-line-rewrite`](../../lifecycle-transitions/README.md#req-status-line-rewrite)); it MUST NOT relocate the file, set the `**Archived:**` axis, or otherwise touch the archival state. Filing an Idea out of active view is the orthogonal [`idea archive`](../archive/README.md) verb. A terminal status (`Rejected`, `Stale`, `Implemented`) is written in place at the active path; the Idea is archived — if at all — by a subsequent `idea archive` call.

### Index sync targets

#### REQ: index-sync-by-target

The post-mutation `specscore spec lint --fix` invocation (per [lifecycle-transitions#req:index-sync-on-success](../../lifecycle-transitions/README.md#req-index-sync-on-success)) MUST cause `idea-index-row-sync` to rewrite the row's Status cell in `spec/ideas/README.md` from the prior value to the new status. The active-vs-archived index split keys off the archived axis (location/flag), not status, so a terminal status written here keeps the row in the active index until `idea archive` relocates it. The rule already exists in `pkg/lint/`; no new lint rule is required for the Idea kind.

## Parameters

| Name | Required | Description |
|---|---|---|
| `slug` | Yes | Idea slug — identifies the active file at `spec/ideas/<slug>.md`. |

## Flags

| Flag | Required | Description |
|---|---|---|
| `--to` | Yes | Target status. Legal values (case-insensitive): `In Review`, `Approved`, `Specifying`, `Specified`, `Implementing`, `Implemented`, `Rejected`, `Stale`. |
| `--project` | No | Project root. Autodetected per [CLI#req:project-autodetect](../../README.md#req-project-autodetect). |

## Exit codes

| Code | Condition |
|---|---|
| `0` | Transition succeeded; `**Status:**` rewritten in place; index synced. |
| `2` | Missing or malformed `<slug>`, missing `--to`, or unrecognized `--to` value. |
| `3` | No Idea file at `spec/ideas/<slug>.md`. |
| `4` | `(current_status, --to)` is not a legal transition per the matrix above. |
| `10` | I/O failure during rewrite or `spec lint --fix` (rollback applied). |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [lifecycle-transitions](../../lifecycle-transitions/README.md) | Defines every cross-cutting REQ this verb satisfies. The verb performs the Meta's `status-line-rewrite` in place, with no relocation side effect. |
| [cli/idea/archive](../archive/README.md) | The orthogonal verb that files an Idea out of active view (`**Archived:** true` + relocation). `change-status` writes the terminal status; `archive` files it away. |
| [idea (CLI group)](../README.md) | Parent group. Contents table includes this sub-feature. |
| [cli/feature/change-status](../../feature/change-status/README.md) | Sibling verb for the Feature kind. Same shared contract; the differences are the legal-transition matrix, the identifier name (`<feature_id>` vs `<slug>`), and the kind-specific path. |
| [spec lint](../../spec/lint/README.md) | Invoked internally by the shared contract. For Idea, `idea-index-row-sync` fires after the mutation. |
| [status-vocabulary](https://github.com/specscore/specscore/blob/main/spec/features/status-vocabulary/README.md) | Source of truth for the Idea legal status set and the archival-not-a-status rule. |
| [idea](../../../idea/README.md), [ideas-index](https://github.com/specscore/specscore/blob/main/spec/features/ideas-index/README.md) | Sources of truth for the Idea document structure and the active-vs-archived index split. |
| Source Idea: [lifecycle-verbs-for-idea-and-feature](../../../../ideas/lifecycle-verbs-for-idea-and-feature.md) | Specifies `change-status` as the single Idea-kind status-transition verb. |

## Acceptance Criteria

### AC: draft-to-approved-happy-path

**Requirements:** [cli/idea/change-status#req:legal-transition-matrix](#req-legal-transition-matrix), [cli/idea/change-status#req:target-status-flag](#req-target-status-flag), [lifecycle-transitions#req:status-line-rewrite](../../lifecycle-transitions/README.md#req-status-line-rewrite), [lifecycle-transitions#req:index-sync-on-success](../../lifecycle-transitions/README.md#req-index-sync-on-success), [lifecycle-transitions#req:success-output-format](../../lifecycle-transitions/README.md#req-success-output-format)

Given `spec/ideas/foo.md` containing `**Status:** Draft`, running `specscore idea change-status foo --to=approved` exits `0`, writes exactly `foo: Draft → Approved\n` to stdout, rewrites the Status line to `Approved`, and syncs the ideas-index row.

### AC: terminal-status-written-in-place

**Requirements:** [cli/idea/change-status#req:legal-transition-matrix](#req-legal-transition-matrix), [cli/idea/change-status#req:no-relocation](#req-no-relocation), [cli/idea/change-status#req:index-sync-by-target](#req-index-sync-by-target)

Given `spec/ideas/foo.md` containing `**Status:** In Review`, running `specscore idea change-status foo --to=rejected` exits `0`, writes `foo: In Review → Rejected\n` to stdout, rewrites the Status line to `Rejected` **in place** (the file remains at `spec/ideas/foo.md` — no relocation, no `**Archived:**` axis), and syncs the ideas-index row. Filing it away is a separate `specscore idea archive foo`.

### AC: approved-to-rejected-happy-path

**Requirements:** [cli/idea/change-status#req:legal-transition-matrix](#req-legal-transition-matrix)

Given `spec/ideas/foo.md` containing `**Status:** Approved`, running `specscore idea change-status foo --to=rejected` exits `0`, writes `foo: Approved → Rejected\n` to stdout, and rewrites the Status line to `Rejected` in place. An agreed-but-not-yet-specified Idea can be actively turned down rather than left to passively decay to `Stale` — the two dispositions are not interchangeable.

### AC: specifying-to-rejected-happy-path

**Requirements:** [cli/idea/change-status#req:legal-transition-matrix](#req-legal-transition-matrix)

Given `spec/ideas/foo.md` containing `**Status:** Specifying`, running `specscore idea change-status foo --to=rejected` exits `0`, writes `foo: Specifying → Rejected\n` to stdout. An Idea can be actively turned down mid-specification, not only before specification starts.

### AC: specified-to-rejected-happy-path

**Requirements:** [cli/idea/change-status#req:legal-transition-matrix](#req-legal-transition-matrix)

Given `spec/ideas/foo.md` containing `**Status:** Specified`, running `specscore idea change-status foo --to=rejected` exits `0`, writes `foo: Specified → Rejected\n` to stdout.

### AC: implementing-to-rejected-happy-path

**Requirements:** [cli/idea/change-status#req:legal-transition-matrix](#req-legal-transition-matrix)

Given `spec/ideas/foo.md` containing `**Status:** Implementing`, running `specscore idea change-status foo --to=rejected` exits `0`, writes `foo: Implementing → Rejected\n` to stdout. A build that is actively called off is `Rejected`.

### AC: implementing-to-stale-happy-path

**Requirements:** [cli/idea/change-status#req:legal-transition-matrix](#req-legal-transition-matrix)

Given `spec/ideas/foo.md` containing `**Status:** Implementing`, running `specscore idea change-status foo --to=stale` exits `0`, writes `foo: Implementing → Stale\n` to stdout. Before this row, `Implementing`'s only exit was `→ Implemented`; a build that simply petered out — nobody decided against it — had no legal terminal status. A build that decayed rather than was cancelled is `Stale`, not `Rejected`.

### AC: case-insensitive-to-flag

**Requirements:** [cli/idea/change-status#req:target-status-flag](#req-target-status-flag)

`specscore idea change-status foo --to=APPROVED` and `--to=Approved` behave identically to `--to=approved`. The file is written with canonical title-case (`**Status:** Approved`) regardless of input case.

### AC: illegal-target-rejected

**Requirements:** [cli/idea/change-status#req:legal-transition-matrix](#req-legal-transition-matrix), [lifecycle-transitions#req:state-machine-strictness](../../lifecycle-transitions/README.md#req-state-machine-strictness)

Given `spec/ideas/foo.md` containing `**Status:** Draft`, running `specscore idea change-status foo --to=implementing` exits `4` with a stderr message naming `Draft` as the current status and `Approved`, `In Review`, `Stale` as the legal targets from `Draft`. No file change.

### AC: already-approved-rejected

**Requirements:** [cli/idea/change-status#req:legal-transition-matrix](#req-legal-transition-matrix), [lifecycle-transitions#req:not-idempotent](../../lifecycle-transitions/README.md#req-not-idempotent)

Given the Idea is already in `**Status:** Approved`, running `specscore idea change-status foo --to=approved` exits `4` (not `0`) — re-running on the target state is an illegal transition per the strict state-machine.

### AC: unrecognized-to-value-rejected

**Requirements:** [cli/idea/change-status#req:target-status-flag](#req-target-status-flag)

`specscore idea change-status foo --to=banana` exits `2` (InvalidArgs) BEFORE any state-machine check, with a stderr message that `banana` is not a recognized Idea status.

### AC: missing-slug-rejected

**Requirements:** [lifecycle-transitions#req:slug-positional](../../lifecycle-transitions/README.md#req-slug-positional)

Running `specscore idea change-status --to=approved` with no positional argument exits `2`. No filesystem change.

### AC: missing-to-flag-rejected

**Requirements:** [cli/idea/change-status#req:target-status-flag](#req-target-status-flag)

Running `specscore idea change-status foo` (no `--to`) exits `2` with stderr stating that `--to` is required.

### AC: slug-not-found

**Requirements:** [cli/idea/change-status#req:slug-resolves-to-active-idea](#req-slug-resolves-to-active-idea)

Running `specscore idea change-status nonexistent --to=approved` where `spec/ideas/nonexistent.md` does not exist exits `3`. An already-archived file at `spec/ideas/archived/nonexistent.md` does NOT satisfy the lookup.

### AC: lint-failure-rolls-back

**Requirements:** [lifecycle-transitions#req:rollback-on-lint-failure](../../lifecycle-transitions/README.md#req-rollback-on-lint-failure)

Given a transition whose post-mutation `spec lint --fix` surfaces an error-severity violation, the verb exits `10` and restores `spec/ideas/foo.md` with its original `**Status:**` line (byte-identical to pre-invocation).

## Open Questions

- Should `change-status` accept `--note "<text>"` to capture the disposition rationale on a `→ Rejected`/`→ Stale` transition? (The archive action has its own optional `--note`; see [idea/archive](../archive/README.md).) The note plumbing exists at the lifecycle layer; surfacing a flag here is deferred.
- Should `change-status --help` render the legal-transition matrix as a table? Lean: yes. Validation of the "discoverability via `--help`" assumption depends on it.

---
*This document follows the https://specscore.md/feature-specification*
