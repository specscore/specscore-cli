---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: Task New

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/task/new?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/task/new?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/task/new?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/task/new?op=request-change) |
>
> **AI skill:** [GitHub](https://github.com/specscore/ai-plugin-specscore/blob/main/skills/task/references/new.md) · [local](../../../../../../ai-plugin-specscore/skills/task/references/new.md) — if this command's CLI signature or behavior changes, update the linked skill to keep agents in sync.

**Status:** Stable
**Source Ideas:** —

## Summary

`specscore task new` creates a new task: writes `tasks/<slug>/README.md` with the required sections and appends a row to the task board at `tasks/README.md`. New tasks are always created in `planning` status.

## Synopsis

```
specscore task new --task <slug> --title <text> [--description <text>] [--depends-on <slugs>] [--format <yaml|json>] [--project <path>]
```

## Problem

Board and task-file creation need to stay in lock-step: a row without a file is an orphan, a file without a row is invisible. Doing both by hand is a two-step ritual that is easy to half-complete. A single command uses a durable prepared marker so any interruption is either pre-publication or exactly retryable.

## Behavior

### Required inputs

`--task` and `--title` are the only required flags.

#### REQ: slug-and-title-required

`--task` and `--title` MUST both be supplied. Missing either MUST exit `2` (InvalidArgs) with a message naming the missing flag(s).

#### REQ: slug-format

The value of `--task` MUST satisfy the task slug-format rule: lowercase, hyphen-separated, URL-safe. Invalid slugs MUST exit `2`.

### Created artifacts

The command produces exactly two changes in the working tree:

1. A new file `tasks/<slug>/README.md` with required sections populated.
2. A new row on `tasks/README.md` referencing the slug with status `planning`.

#### REQ: planning-only

`task new` MUST set status to `planning`. Creating tasks in other statuses is out of scope (see [parent cli/task#req:no-lifecycle-in-mvp](../README.md#req-no-lifecycle-in-mvp)). Callers cannot override this via a flag.

#### REQ: status-scaffolded

The written `tasks/<slug>/README.md` MUST include a body `**Status:** planning` line immediately after its title, in the same fixed position `task change-status` expects when it later rewrites the field in place. A task created without this line can never transition through the sanctioned CLI (`task change-status` has no path that initializes an absent line — see [`cli/task/change-status`](../change-status/README.md)); this REQ is what keeps every NEW task from ever reaching that state. An existing board predating this REQ is backfilled once by [`specscore migrate`](../../spec/migrate/README.md#req-task-board-status-backfill), never by `task new` retroactively.

#### REQ: atomic-board-update

The command MUST acquire the board artifact's fail-fast lock before it reads the board, checks the target path, or allocates the task identity. Only after proving both board row and target path absent may it publish an exclusive durable prepared marker binding task id, target path, exact README digest, and the exact pre- and intended post-mutation board digests. It then publishes the README exclusively and commits that bound postimage through one atomic durable board transaction.

Finalization first creates an exclusive hard-linked committed receipt for the exact prepared-marker bytes and durably fences its parent before removing the prepared name. The already-synced marker inode supplies the receipt's file durability; the hard link adds no new content write. Failure before prepared-name cleanup leaves at least one exact receipt for retry. Receipt collision or byte mismatch fails closed. A retry accepts only a matching receipt, exact README bytes, the exact committed board row, and the exact full board postimage digest bound by the receipt; it can idempotently finish prepared-name/receipt cleanup without recreating or duplicating the task. A missing/mutated committed row, an unrelated row added after the bound postimage, or changed README bytes is recovery-required conflict and is never reconstructed or rewritten from the receipt.

A failure before prepared-marker publication leaves no new state. A failure after the marker or README becomes visible MUST retain the exact marker and any owned bytes for explicit retry; it MUST NOT recursively delete or blindly roll back visible state. A retry adopts state only when marker, requested content, README bytes, and board preimage/committed row match exactly. A committed row finalizes its matching marker idempotently. A foreign replacement, duplicate row, changed board, or different retry intent exits `1` and is never overwritten or deleted.

### Collision handling

`tasks/<slug>/` existing is a hard conflict. The command MUST NOT overwrite.

#### REQ: no-clobber

If `tasks/<slug>/` already exists OR if the board already has a row with that slug, the command MUST exit `1` (Conflict) with a message naming the collision. No `--force` exists for `task new`.

### Dependencies

`--depends-on` accepts a comma-separated list of task slugs that the new task depends on.

#### REQ: depends-on-validation

Every value supplied to `--depends-on` MUST resolve to an existing task (either in the board or as a directory under `tasks/`). Unresolved slugs MUST exit `3` (NotFound) BEFORE any file is written.

## Parameters

None. All inputs are flags.

## Exit codes

| Code | Condition |
|---|---|
| `0` | Task created (file + board row written) |
| `1` | Slug collision, artifact-lock contention, duplicate row, changed recovery preimage, or prepared ownership mismatch |
| `2` | Missing `--task`/`--title`, invalid flag value, bad slug |
| `3` | `--depends-on` names a non-existent task |
| `10` | Unexpected I/O/durability failure; if publication became visible, the exact prepared marker is retained and the error reports recovery required |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [task](../../../task/README.md) | Defines required task file sections, board format, and allowed status values. `task new` emits a task that conforms. |
| [CLI Task group](../README.md) | Inherits the MVP "no lifecycle" scope rule. |

## Acceptance Criteria

### AC: creates-file-and-board-row

**Requirements:** cli/task/new#req:slug-and-title-required, cli/task/new#req:atomic-board-update

`specscore task new --task my-task --title "My Task"` creates `tasks/my-task/README.md` with the required sections and appends a row to `tasks/README.md` with status `planning`. On success the marker has been durably finalized; at every failure boundary the operation is either write-free or exactly retryable.

### AC: collision-exits-1

**Requirements:** cli/task/new#req:no-clobber

Running the command twice with the same slug exits `1` on the second run. No state is mutated on the second run.

### AC: missing-dep-exits-3

**Requirements:** cli/task/new#req:depends-on-validation

`specscore task new --task t --title T --depends-on does-not-exist` exits `3` with a message naming the missing dependency. Neither the new file nor the new board row is created.

### AC: status-fixed-to-planning

**Requirements:** cli/task/new#req:planning-only

Every task created by this command has status `planning`. The command exposes no flag for overriding this.

### AC: new-task-carries-status-line

**Requirements:** [cli/task/new#req:status-scaffolded](#req-status-scaffolded)

**Given** `specscore task new --task auth --title Auth`
**When** the command completes
**Then** `tasks/auth/README.md` contains `**Status:** planning` immediately after its title, and `specscore task change-status auth --to=queued` succeeds on the very first attempt.

## Open Questions

- Should `task new` accept a `--commit` / `--push` flag pair for parity with `feature new`, enabling one-shot agent workflows that create and push a task in a single call?
- Should the board row be inserted at a sorted position (by slug or by creation date) rather than appended, to keep the board diff-friendly across concurrent creators?

---
*This document follows the https://specscore.md/feature-specification*
