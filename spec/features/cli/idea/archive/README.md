---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: Idea Archive / Unarchive

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/idea/archive?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/idea/archive?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/idea/archive?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/idea/archive?op=request-change) |

**Status:** Draft
**Source Ideas:** —

## Summary

`specscore idea archive <slug>` files an Idea out of active view, and `specscore idea unarchive <slug>` returns it. Archival is an axis **orthogonal to lifecycle status** (per [status-vocabulary#req:archival-not-a-status](https://github.com/specscore/specscore/blob/main/spec/features/status-vocabulary/README.md#req-archival-not-a-status)): an archived Idea keeps its real terminal `**Status:**` (`Rejected`, `Stale`, or `Implemented`) and is additionally marked archived by two coordinated facts — an `**Archived:** true` header line and relocation to `spec/ideas/archived/<slug>.md`. These verbs are the counterpart to [change-status](../change-status/README.md), which only rewrites the `**Status:**` line and never relocates a file.

## Synopsis

```
specscore idea archive <slug> [--note <text>] [--project <path>]
specscore idea unarchive <slug> [--project <path>]
```

## Problem

Historically an Idea was "archived" by overwriting its `**Status:**` with the value `Archived`, which conflated two orthogonal questions — *why* the lifecycle ended (rejected? shipped? decayed?) and *whether* the artifact is filed out of active view. An `Archived` idea had lost its real terminal outcome. The canonical [status-vocabulary](https://github.com/specscore/specscore/blob/main/spec/features/status-vocabulary/README.md) forbids `Archived` as a status and makes archival an orthogonal storage axis. These verbs implement that axis: the disposition (`Rejected`/`Stale`/`Implemented`) is set by [change-status](../change-status/README.md); filing the artifact away — preserving that disposition — is `idea archive`.

## Behavior

This verb pair inherits the cross-cutting mutation/rollback rules from [lifecycle-transitions](../../lifecycle-transitions/README.md) (atomic mutation, post-mutation `spec lint --fix`, full rollback on failure) but is NOT a status transition — it does not consult the Idea state-machine matrix and does not touch the `**Status:**` line.

### The archived axis

#### REQ: archived-axis-two-facts

An archived Idea MUST be represented by BOTH: (a) an `**Archived:** true` header line in the artifact, and (b) location under `spec/ideas/archived/<slug>.md`. The two MUST agree; the [`idea-archived-location`](../../spec/lint/README.md) lint rule (error severity) flags any disagreement. An absent or non-`true` `**Archived:**` value means not archived.

#### REQ: status-preserved

Neither `archive` nor `unarchive` may modify the `**Status:**` line. An archived Idea retains the terminal status it carried when archived (e.g. a `Rejected` idea filed away stays `Rejected`, archived). Archiving an Idea whose status is not terminal (`Implemented`, `Rejected`, or `Stale`) is a lint error (`idea-archived-location`); the canonical flow is `change-status --to=<terminal>` first, then `archive`.

### Archive

#### REQ: archive-sets-flag-and-relocates

`specscore idea archive <slug>` MUST: resolve `<slug>` to an active file at `spec/ideas/<slug>.md` (a missing active file exits `3`); set `**Archived:** true` in the header (preserving `**Status:**` and every other byte); and relocate the file to `spec/ideas/archived/<slug>.md` (creating the `archived/` directory and its lint-clean index stub if absent — mkdir-p semantics). If `spec/ideas/archived/<slug>.md` already exists, the verb MUST exit `1` (Conflict) without overwriting it and leave the active file untouched.

#### REQ: archive-note-optional

`idea archive` MAY accept a `--note <text>` flag. When supplied (non-empty after trimming), it is written as an `**Archive Note:**` header line tied to the archive action. The note is OPTIONAL — the *disposition* reason lives in the terminal `change-status` transition, not here. When the `**Archive Note:**` line is present it MUST be non-empty (`idea-archive-note`, format-if-present); an empty/em-dash value is a lint error.

### Unarchive

#### REQ: unarchive-clears-flag-and-relocates

`specscore idea unarchive <slug>` MUST: resolve `<slug>` to an archived file at `spec/ideas/archived/<slug>.md` (a missing archived file exits `3`); remove the `**Archived:**` and `**Archive Note:**` header lines (preserving `**Status:**`); and relocate the file back to `spec/ideas/<slug>.md`. If `spec/ideas/<slug>.md` already exists, the verb MUST exit `1` (Conflict) without overwriting it and leave the archived file untouched.

### Atomic mutation and rollback

#### REQ: rollback-on-failure

Per [lifecycle-transitions#req:rollback-on-lint-failure](../../lifecycle-transitions/README.md#req-rollback-on-lint-failure): on any failure after the relocation (post-mutation lint failure, I/O error), the verb MUST restore the on-disk state to its byte-identical pre-invocation form — file back at its original path with its original content, and any index stub the verb itself materialized removed. Partial state MUST NOT be observable after the command returns.

### Index sync

#### REQ: index-sync-on-archive

The post-mutation `specscore spec lint --fix` invocation MUST resync both idea indexes: `idea-index-row-sync` removes the now-archived slug's row from the active `spec/ideas/README.md`, and `idea-archived-index-chronological` adds (archive) or removes (unarchive) the row in `spec/ideas/archived/README.md` ordered by `**Date:**`. The active-vs-archived split keys off the archived axis (location/flag), not status. Both rules already exist in `pkg/lint/`.

## Parameters

| Name | Required | Description |
|---|---|---|
| `slug` | Yes | Idea slug. For `archive`, identifies the active file at `spec/ideas/<slug>.md`; for `unarchive`, the archived file at `spec/ideas/archived/<slug>.md`. |

## Flags

| Flag | Required | Description |
|---|---|---|
| `--note` | No | (`archive` only) Optional `**Archive Note:**` tied to the archive action. |
| `--project` | No | Project root. Autodetected per [CLI#req:project-autodetect](../../README.md#req-project-autodetect). |

## Exit codes

| Code | Condition |
|---|---|
| `0` | Archived/unarchived; flag set/cleared; file relocated; indexes synced. |
| `1` | Collision: the destination path already exists. |
| `2` | Missing or malformed `<slug>`. |
| `3` | No file at the expected source path. |
| `10` | I/O failure during the relocation or `spec lint --fix` (rollback applied). |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [cli/idea/change-status](../change-status/README.md) | Sets the terminal `**Status:**` in place. `archive`/`unarchive` then file the Idea away/back along the orthogonal axis, preserving that status. |
| [lifecycle-transitions](../../lifecycle-transitions/README.md) | Source of the atomic-mutation, post-mutation-lint, and rollback contract these verbs reuse (minus the state-machine check, which does not apply). |
| [status-vocabulary](https://github.com/specscore/specscore/blob/main/spec/features/status-vocabulary/README.md) | Defines that archival is orthogonal to status and that `Archived` is not a status value. |
| [spec lint](../../spec/lint/README.md) | `idea-archived-location`, `idea-archive-note`, `idea-index-row-sync`, and `idea-archived-index-chronological` enforce/sync the archived axis. |
| [idea (CLI group)](../README.md) | Parent group. Contents table includes this sub-feature. |

## Acceptance Criteria

### AC: archive-happy-path

**Requirements:** [cli/idea/archive#req:archive-sets-flag-and-relocates](#req-archive-sets-flag-and-relocates), [cli/idea/archive#req:status-preserved](#req-status-preserved), [cli/idea/archive#req:index-sync-on-archive](#req-index-sync-on-archive)

Given `spec/ideas/foo.md` containing `**Status:** Stale`, running `specscore idea archive foo` exits `0`, sets `**Archived:** true` while leaving `**Status:** Stale` unchanged, moves the file to `spec/ideas/archived/foo.md`, removes the row from `spec/ideas/README.md`, and inserts a chronologically-ordered row in `spec/ideas/archived/README.md`.

### AC: archive-note-optional

**Requirements:** [cli/idea/archive#req:archive-note-optional](#req-archive-note-optional)

Running `specscore idea archive foo --note "abandoned after the v2 pivot"` writes a non-empty `**Archive Note:** abandoned after the v2 pivot` header line. Running `specscore idea archive foo` with no `--note` writes no `**Archive Note:**` line and is lint-clean (the note is optional).

### AC: archive-collision

**Requirements:** [cli/idea/archive#req:archive-sets-flag-and-relocates](#req-archive-sets-flag-and-relocates), [cli/idea/archive#req:rollback-on-failure](#req-rollback-on-failure)

Given both `spec/ideas/foo.md` (active) and a stale `spec/ideas/archived/foo.md`, running `specscore idea archive foo` exits `1` naming the collision target, leaves the active file untouched, and leaves the stale archived file untouched.

### AC: archive-slug-not-found

**Requirements:** [cli/idea/archive#req:archive-sets-flag-and-relocates](#req-archive-sets-flag-and-relocates)

Running `specscore idea archive nonexistent` where `spec/ideas/nonexistent.md` does not exist exits `3`.

### AC: unarchive-happy-path

**Requirements:** [cli/idea/archive#req:unarchive-clears-flag-and-relocates](#req-unarchive-clears-flag-and-relocates), [cli/idea/archive#req:status-preserved](#req-status-preserved)

Given an archived `spec/ideas/archived/foo.md` with `**Status:** Stale` and `**Archived:** true`, running `specscore idea unarchive foo` exits `0`, removes the `**Archived:**` (and any `**Archive Note:**`) header lines while preserving `**Status:** Stale`, and moves the file back to `spec/ideas/foo.md`.

### AC: unarchive-collision

**Requirements:** [cli/idea/archive#req:unarchive-clears-flag-and-relocates](#req-unarchive-clears-flag-and-relocates)

Given an archived `spec/ideas/archived/foo.md` AND a pre-existing active `spec/ideas/foo.md`, running `specscore idea unarchive foo` exits `1` naming the collision target and leaves the archived file untouched.

### AC: lint-failure-rolls-back

**Requirements:** [cli/idea/archive#req:rollback-on-failure](#req-rollback-on-failure)

Given an archive/unarchive whose post-mutation `spec lint --fix` surfaces an error-severity violation, the verb exits `10` and restores the on-disk state byte-identically to its pre-invocation form (file back at its original path, original content; no orphaned index stub).

### AC: archived-axis-agreement-enforced

**Requirements:** [cli/idea/archive#req:archived-axis-two-facts](#req-archived-axis-two-facts), [cli/idea/archive#req:status-preserved](#req-status-preserved)

An Idea carrying `**Archived:** true` outside `spec/ideas/archived/`, a file under `archived/` lacking `**Archived:** true`, or an archived Idea whose `**Status:**` is not terminal (`Implemented`/`Rejected`/`Stale`) each fires the `idea-archived-location` lint rule (error severity).

## Open Questions

- Should `archive` accept a `--reason` distinct from `--note`, or is the terminal `change-status` note the canonical place for the disposition rationale? Current lean: one optional `--note` on `archive`, disposition reason on `change-status`.
- Should `unarchive` of a `Stale` Idea optionally re-open it (e.g. back to `Draft`) in one step, or is that strictly a follow-on `change-status`? Deferred — keep the axes independent for now.

---
*This document follows the https://specscore.md/feature-specification*
