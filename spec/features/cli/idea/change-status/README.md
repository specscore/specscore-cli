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

`specscore idea change-status <slug> --to=<status>` transitions an Idea artifact from its current `**Status:**` to the target status named by `--to`. Implements the [lifecycle-transitions](../../lifecycle-transitions/README.md) shared contract; extends it with a kind-specific file-relocation side effect when `--to=archived`.

## Synopsis

```
specscore idea change-status <slug> --to=<status> [--project <path>]
```

## Problem

Today, transitioning an Idea's status is a hand-edit of the `**Status:**` line in `spec/ideas/<slug>.md` plus `specscore spec lint --fix` for index sync. Archiving is the same plus a `git mv` to `spec/ideas/archived/`. Hand-edits skip state-machine validation (a hand-edit can drop `Specified` back to `Draft` with no warning), forget the lint sync (leaving the index stale), and have no machine-readable contract. This verb closes the gap with a single command per kind.

## Behavior

This verb inherits every cross-cutting rule from [lifecycle-transitions](../../lifecycle-transitions/README.md). The REQs below are the verb-specific declarations: the Idea legal-transition matrix, the `--to` flag, the kind-specific slug resolution, the `--to=archived` file-relocation side effect, and the two index-sync rules that fire on archive.

### Legal-transition matrix

Only the transitions in the table below are accepted. Any other `(from, to)` pair exits `4` (InvalidTransition) per [lifecycle-transitions#req:state-machine-strictness](../../lifecycle-transitions/README.md#req-state-machine-strictness).

| From | To | Side effects |
|---|---|---|
| `Draft` | `Approved` | Status rewrite + ideas-index sync |
| `Draft` | `Archived` | Status rewrite + file move + active-index + archived-index sync |
| `Under Review` | `Archived` | Status rewrite + file move + active-index + archived-index sync |
| `Approved` | `Specifying` | Status rewrite + ideas-index sync |
| `Approved` | `Archived` | Status rewrite + file move + active-index + archived-index sync |
| `Specifying` | `Specified` | Status rewrite + ideas-index sync |
| `Specifying` | `Archived` | Status rewrite + file move + active-index + archived-index sync |
| `Specified` | `Implementing` | Status rewrite + ideas-index sync |
| `Specified` | `Archived` | Status rewrite + file move + active-index + archived-index sync |
| `Implementing` | `Implemented` | Status rewrite + ideas-index sync |
| `Implementing` | `Archived` | Status rewrite + file move + active-index + archived-index sync |
| `Implemented` | `Archived` | Status rewrite + file move + active-index + archived-index sync |

For **feature-request** ideas, the forward transitions (`Approved → Specifying → Specified → Implementing → Implemented`) are derived from Feature status by the `idea-sync-lint-strict` lint rule. The `change-status` verb is the user-facing surface for manual override or when a transition is needed ahead of the derivation rule. For **change-request** ideas, all transitions are author-managed (not derived) — the lint derivation rules skip change-request ideas entirely.

#### REQ: legal-transition-matrix

The verb MUST accept only `(from, to)` pairs listed in the legal-transition matrix above. Any other pair MUST exit `4` (InvalidTransition) per the Meta contract, with a stderr message naming both the current status and the legal target statuses from the current state.

### Target-status flag

#### REQ: target-status-flag

The verb MUST accept the target status via a required `--to=<status>` flag. The flag value MUST be a recognized Idea status; unrecognized values exit `2` (InvalidArgs) BEFORE state-machine validation. Flag value matching is case-insensitive on input (`--to=approved`, `--to=Approved`, `--to=APPROVED` all parse identically); the canonical title-case value is what gets written to the file and to the success-output line. A multi-word value MUST be supplied with shell quoting or hyphenation per cobra conventions (`--to="Under Review"` or `--to='Under Review'`).

### Kind-specific slug resolution

#### REQ: slug-resolves-to-active-idea

The `<slug>` positional argument MUST resolve to a file at `spec/ideas/<slug>.md` within the project root. Already-archived files at `spec/ideas/archived/<slug>.md` MUST NOT be matched per [lifecycle-transitions#req:slug-resolves-to-existing-artifact](../../lifecycle-transitions/README.md#req-slug-resolves-to-existing-artifact). A missing file at the active path MUST exit `3` (NotFound).

### Archive side effect

When `--to=archived` is supplied, the verb extends the Meta's [`status-line-rewrite`](../../lifecycle-transitions/README.md#req-status-line-rewrite) with a filesystem effect.

#### REQ: archive-relocation

For `--to=archived` only, after a successful `**Status:** ... → **Status:** Archived` rewrite, the verb MUST move the file from `spec/ideas/<slug>.md` to `spec/ideas/archived/<slug>.md` (creating the `archived/` directory if absent — mkdir-p semantics). If `spec/ideas/archived/<slug>.md` already exists, the verb MUST exit `1` (Conflict) and restore the original status line at the source path before returning.

#### REQ: rollback-includes-relocation

The Meta's [REQ: rollback-on-lint-failure](../../lifecycle-transitions/README.md#req-rollback-on-lint-failure) applies and is extended for archive transitions: on any failure after the status rewrite (collision, file-move failure, lint failure, I/O error), the verb MUST restore the on-disk state to its pre-invocation form — file at `spec/ideas/<slug>.md`, original `**Status:**` value. Partial state MUST NOT be observable after the command returns. Exit codes: `1` for collision, `10` for I/O or post-relocation lint failure.

### Index sync targets

#### REQ: index-sync-by-target

The post-mutation `specscore spec lint --fix` invocation (per [lifecycle-transitions#req:index-sync-on-success](../../lifecycle-transitions/README.md#req-index-sync-on-success)) MUST cause:

- For `--to=approved`: `idea-index-row-sync` rewrites the row's Status cell in `spec/ideas/README.md` from the prior value to `Approved`.
- For `--to=archived`: `idea-index-row-sync` removes the row from `spec/ideas/README.md` (per [ideas-index#req:status-excludes-archived](https://github.com/specscore/specscore/blob/main/spec/features/ideas-index/README.md#req-status-excludes-archived)), AND `idea-archived-index-chronological` adds the row to `spec/ideas/archived/README.md` in chronological order by `**Date:**`.

Both rules already exist in `pkg/lint/`; no new lint rule is required for the Idea kind.

## Parameters

| Name | Required | Description |
|---|---|---|
| `slug` | Yes | Idea slug — identifies the active file at `spec/ideas/<slug>.md`. |

## Flags

| Flag | Required | Description |
|---|---|---|
| `--to` | Yes | Target status. Legal values: `approved`, `archived` (case-insensitive). |
| `--project` | No | Project root. Autodetected per [CLI#req:project-autodetect](../../README.md#req-project-autodetect). |

## Exit codes

| Code | Condition |
|---|---|
| `0` | Transition succeeded; file rewritten (and, for archive, relocated); indexes synced. |
| `1` | Archive collision: `spec/ideas/archived/<slug>.md` already exists. |
| `2` | Missing or malformed `<slug>`, missing `--to`, or unrecognized `--to` value. |
| `3` | No Idea file at `spec/ideas/<slug>.md`. |
| `4` | `(current_status, --to)` is not a legal transition per the matrix above. |
| `10` | I/O failure during rewrite, file move, or `spec lint --fix` (rollback applied). |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [lifecycle-transitions](../../lifecycle-transitions/README.md) | Defines every cross-cutting REQ this verb satisfies. The verb extends the Meta's `status-line-rewrite` with a relocation side effect for `--to=archived` (see [REQ: archive-relocation](#req-archive-relocation)). |
| [idea (CLI group)](../README.md) | Parent group. Contents table includes this sub-feature. |
| [cli/feature/change-status](../../feature/change-status/README.md) | Sibling verb for the Feature kind. Same shared contract; the differences are the legal-transition matrix, the identifier name (`<feature_id>` vs `<slug>`), the kind-specific path, and the archive side effect (Feature has none). |
| [spec lint](../../spec/lint/README.md) | Invoked internally by the shared contract. For Idea, `idea-index-row-sync` and (for archive) `idea-archived-index-chronological` fire after the mutation. |
| [idea](../../../idea/README.md), [ideas-index](https://github.com/specscore/specscore/blob/main/spec/features/ideas-index/README.md) | Sources of truth for the Idea document structure, the legal status enumeration, and the active-vs-archived index split. |
| Source Idea: [lifecycle-verbs-for-idea-and-feature](../../../../ideas/lifecycle-verbs-for-idea-and-feature.md) | Specifies `change-status` as the single Idea-kind lifecycle verb. |

## Acceptance Criteria

### AC: draft-to-approved-happy-path

**Requirements:** [cli/idea/change-status#req:legal-transition-matrix](#req-legal-transition-matrix), [cli/idea/change-status#req:target-status-flag](#req-target-status-flag), [lifecycle-transitions#req:status-line-rewrite](../../lifecycle-transitions/README.md#req-status-line-rewrite), [lifecycle-transitions#req:index-sync-on-success](../../lifecycle-transitions/README.md#req-index-sync-on-success), [lifecycle-transitions#req:success-output-format](../../lifecycle-transitions/README.md#req-success-output-format)

Given `spec/ideas/foo.md` containing `**Status:** Draft`, running `specscore idea change-status foo --to=approved` exits `0`, writes exactly `foo: Draft → Approved\n` to stdout, rewrites the Status line to `Approved`, and syncs the ideas-index row.

### AC: archive-from-approved-happy-path

**Requirements:** [cli/idea/change-status#req:legal-transition-matrix](#req-legal-transition-matrix), [cli/idea/change-status#req:archive-relocation](#req-archive-relocation), [cli/idea/change-status#req:index-sync-by-target](#req-index-sync-by-target)

Given `spec/ideas/foo.md` containing `**Status:** Approved`, running `specscore idea change-status foo --to=archived` exits `0`, writes `foo: Approved → Archived\n` to stdout, rewrites the Status line to `Archived`, moves the file to `spec/ideas/archived/foo.md`, removes the row from `spec/ideas/README.md`, and inserts a chronologically-ordered row in `spec/ideas/archived/README.md`.

### AC: case-insensitive-to-flag

**Requirements:** [cli/idea/change-status#req:target-status-flag](#req-target-status-flag)

`specscore idea change-status foo --to=APPROVED` and `--to=Approved` behave identically to `--to=approved`. The file is written with canonical title-case (`**Status:** Approved`) regardless of input case.

### AC: illegal-target-rejected

**Requirements:** [cli/idea/change-status#req:legal-transition-matrix](#req-legal-transition-matrix), [lifecycle-transitions#req:state-machine-strictness](../../lifecycle-transitions/README.md#req-state-machine-strictness)

Given `spec/ideas/foo.md` containing `**Status:** Draft`, running `specscore idea change-status foo --to=implementing` exits `4` with a stderr message naming `Draft` as the current status and `Approved`, `Archived` as the legal targets from `Draft`. No file change.

### AC: already-approved-rejected

**Requirements:** [cli/idea/change-status#req:legal-transition-matrix](#req-legal-transition-matrix), [lifecycle-transitions#req:not-idempotent](../../lifecycle-transitions/README.md#req-not-idempotent)

Given the Idea is already in `**Status:** Approved`, running `specscore idea change-status foo --to=approved` exits `4` (not `0`) — re-running on the target state is an illegal transition per the strict state-machine.

### AC: unrecognized-to-value-rejected

**Requirements:** [cli/idea/change-status#req:target-status-flag](#req-target-status-flag)

`specscore idea change-status foo --to=banana` exits `2` (InvalidArgs) BEFORE any state-machine check, with a stderr message that `banana` is not a recognized Idea status.

### AC: archive-collision

**Requirements:** [cli/idea/change-status#req:archive-relocation](#req-archive-relocation), [cli/idea/change-status#req:rollback-includes-relocation](#req-rollback-includes-relocation)

Given both `spec/ideas/foo.md` (active, any source status) and `spec/ideas/archived/foo.md` (stale archived from a prior slug reuse), running `specscore idea change-status foo --to=archived` exits `1` with a stderr message naming the collision target, leaves `spec/ideas/foo.md` with its original status (status rewrite rolled back), and leaves `spec/ideas/archived/foo.md` untouched.

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

**Requirements:** [cli/idea/change-status#req:rollback-includes-relocation](#req-rollback-includes-relocation), [lifecycle-transitions#req:rollback-on-lint-failure](../../lifecycle-transitions/README.md#req-rollback-on-lint-failure)

Given a transition that exercises the archive path and a corrupted `spec/ideas/archived/README.md` that fails `idea-archived-index-chronological`, the verb exits `10`, restores `spec/ideas/foo.md` at the active path with the original status line, and leaves no file at `spec/ideas/archived/foo.md`.

## Open Questions

- Should `change-status` accept `--reason "<text>"` to capture the rationale (especially valuable when archiving)? Deferred per the source Idea.
- When the Idea status enumeration grows beyond `Draft`/`Approved`/`Archived` to first-class `Specified`/`Implementing` values (today they're managed externally by lint and plan tools), is the legal-transition matrix updated? Lock down at the time the new statuses become user-facing.
- Should `change-status --help` render the legal-transition matrix as a table? Lean: yes. Validation of the "discoverability via `--help`" assumption depends on it.

---
*This document follows the https://specscore.md/feature-specification*
