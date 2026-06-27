---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Task Change-Status

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/task/change-status?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/task/change-status?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/task/change-status?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/task/change-status?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Summary

`specscore task change-status <task> --to=<status>` transitions a Task's status and, on completion, OPTIONALLY records **implementation-commit provenance** (`--repo`/`--commit`/`--branch`) onto the task. It is the capture vehicle for the cross-repo [implementation-commit-provenance](https://github.com/specscore/specscore/blob/main/spec/features/implementation-commit-provenance/README.md) feature (specscore meta-spec): the actor that finished the work supplies the commit reference, and the verb writes it into the Task's `implementation_commit` property (surfaced as `**Implemented-by:** <repo>@<sha>`).

The verb is **single-actor file mutation** — it performs no claim/release, locking, or conflict resolution. That deliberately narrow shape is what lets it exist within `specscore`'s local-file-mutation model without reopening the coordination concerns that keep full task-board orchestration out (see [single-actor-task-lifecycle-permitted](#req-single-actor-task-lifecycle-permitted)). It resolves a task in **either** of two stores: the `tasks/` board (the existing `task` group's target) or a plan-inline task block addressed by its stable `**Id:**` (whose completion feeds the plan execution-band rollup).

## Synopsis

```
specscore task change-status <task> --to=<status> \
  [--plan <plan-slug>] \
  [--repo <repo>] [--commit <sha>] [--branch <branch>] \
  [--project <path>]
```

## Problem

The Task doc kind had **no CLI status verb**: `specscore task` exposed only `info`/`list`/`new`, and both [`cli/task#req:no-lifecycle-in-mvp`](../README.md#req-no-lifecycle-in-mvp) and the Stable [`cli/lifecycle-transitions#req:scope-no-task-lifecycle`](../../lifecycle-transitions/README.md#req-scope-no-task-lifecycle) deliberately excluded task-status mutation, reserving task lifecycle for external orchestrators because *full* task coordination (claim/release, conflict-aware exit codes) doesn't fit `specscore`'s single-actor model.

That left two gaps. First, tasks reached `complete` only by hand-editing Markdown — the exact anti-pattern `idea`/`feature`/`plan change-status` eliminate. Second, and the trigger for this feature: the [implementation-commit-provenance](https://github.com/specscore/specscore/blob/main/spec/features/implementation-commit-provenance/README.md) feature needs a **capture point** — a moment, at task completion, when the actor records the code commit that did the work, so a plan that shows `Implemented` retains durable evidence even if the code is later lost to a rebase/merge. Recording a commit is a contention-free, single-actor annotation — it does **not** need the coordination machinery the exclusion was protecting. This feature therefore narrows the exclusion to admit a single-actor `task change-status` (no claim/release) and makes it the provenance capture vehicle.

## Behavior

This verb satisfies the cross-cutting [lifecycle-transitions](../../lifecycle-transitions/README.md) contract — `status-line-rewrite`, `index-sync-on-success` (post-mutation `spec lint --fix`), `rollback-on-lint-failure` (exit `10`), `success-output-format`, `error-to-stderr`, `exit-code-fidelity`, and strict non-idempotent state-machine semantics. The REQs below are the task-specific declarations. (The `--amend-provenance` path is **not** a status transition: it is exempt from `success-output-format` — emitting `<task>: provenance amended` instead of the `<from> → <to>` line — and from the matrix; see [provenance-corrective-restamp](#req-provenance-corrective-restamp).)

### Scope amendment: a single-actor task lifecycle is permitted

#### REQ: single-actor-task-lifecycle-permitted

This feature **narrows** two previously-blanket exclusions: [`cli/lifecycle-transitions#req:scope-no-task-lifecycle`](../../lifecycle-transitions/README.md#req-scope-no-task-lifecycle) and [`cli/task#req:no-lifecycle-in-mvp`](../README.md#req-no-lifecycle-in-mvp). A **single-actor** `task change-status` verb is permitted: it MUST perform pure status-field mutation (plus optional provenance write) with **no** claim/release, locking, sync policy, or conflict-aware exit codes. Multi-agent coordination of the task board — claim races, contention resolution, distributed terminal-state agreement — remains **out of scope** and orchestrator-owned. The two amended REQs are updated to reference this exception; this REQ is the canonical statement of the narrowed boundary.

### Task status transitions

#### REQ: task-legal-transition-matrix

`--to` accepts a [Task entity](https://github.com/specscore/specscore/blob/main/spec/features/task/task.entity.md) status. The verb MUST accept only the `(from, to)` pairs below; any other pair exits `4` (InvalidTransition) per [lifecycle-transitions#req:state-machine-strictness](../../lifecycle-transitions/README.md#req-state-machine-strictness). Re-running on the current status exits `4` ([not-idempotent](../../lifecycle-transitions/README.md#req-not-idempotent)).

| From | To |
|---|---|
| `planning` | `queued`, `aborted` |
| `queued` | `in_progress`, `aborted` |
| `in_progress` | `blocked`, `complete`, `failed`, `aborted` |
| `blocked` | `in_progress`, `aborted` |

`complete`, `failed`, and `aborted` are terminal (no outgoing transitions). This MVP matrix is intentionally minimal; refinement is an [Open Question](#open-questions).

#### REQ: target-status-flag

A required `--to=<status>` flag names the target (one of the seven Task statuses; matching case-insensitive, canonical lowercase written). A missing `--to` exits `2` (InvalidArgs); a value that is not a Task status exits `2`.

### Task resolution across both stores

#### REQ: task-dual-target-resolution

The `<task>` positional MUST resolve in one of two modes:

- **Board mode** (default, no `--plan`): resolves to `tasks/<task>/README.md`, the store the existing [`task`](../README.md) group operates on.
- **Plan-inline mode** (`--plan <plan-slug>` supplied): resolves `<task>` to a task block inside `spec/plans/<plan-slug>.md` by its **explicit stable id** — the `**Id:** <id>` field declared on the task block (see [plan task `**Id:**`](https://github.com/specscore/specscore/blob/main/spec/features/plan/README.md)). The id is stable across task renumbering and title edits (unlike an ordinal or a title-derived slug), so a recorded reference never silently re-points. Completion of a plan-inline task feeds the plan execution-band rollup ([plan#req:status-rollup](https://specscore.md/plan-specification)).

A `<task>` that resolves in neither the requested mode exits `3` (NotFound). In plan-inline mode, a `<task>` that matches no task block's `**Id:**` (or a plan whose target block declares no `**Id:**`) exits `3`.

### Implementation-commit provenance capture

#### REQ: provenance-flags-optional-and-complete-only

`--repo`, `--commit`, and `--branch` are **optional** and, on a status transition, meaningful **only** with `--to=complete` (they are also accepted on the `--amend-provenance` path — see [provenance-corrective-restamp](#req-provenance-corrective-restamp)). Supplying any of them with a non-`complete` `--to` exits `2` (InvalidArgs). A `--to=complete` with no provenance flags is valid — provenance is never required ([implementation-commit-provenance#req:optional-provenance](https://github.com/specscore/specscore/blob/main/spec/features/implementation-commit-provenance/README.md#req-optional-provenance)).

#### REQ: provenance-ref-assembly

When provenance flags are present, `--commit <sha>` is REQUIRED (its absence while `--repo`/`--branch` are given exits `2`). The verb assembles the reference as `<repo>@<sha>` with an optional trailing `(<branch>)`; when `--repo` is omitted it writes a bare `<sha>` (optionally `<sha> (<branch>)`). The assembled value MUST satisfy [implementation-commit-provenance#req:provenance-ref-format](https://github.com/specscore/specscore/blob/main/spec/features/implementation-commit-provenance/README.md#req-provenance-ref-format) and is written to the resolved task's `implementation_commit` property (surfaced as `**Implemented-by:**`).

#### REQ: provenance-not-derived-not-verified

The verb MUST source the provenance reference **only** from flags — it MUST NOT auto-derive `repo`/`commit`/`branch` from the ambient `git HEAD` (realizing [implementation-commit-provenance#req:provenance-is-actor-supplied-not-derived](https://github.com/specscore/specscore/blob/main/spec/features/implementation-commit-provenance/README.md#req-provenance-is-actor-supplied-not-derived)). It MUST NOT attempt to verify that `<sha>` exists or is reachable in `<repo>` — validation is syntactic only ([provenance-validated-syntactically](https://github.com/specscore/specscore/blob/main/spec/features/implementation-commit-provenance/README.md#req-provenance-validated-syntactically)).

#### REQ: provenance-corrective-restamp

A wrong or missing provenance reference on an already-`complete` task MUST be correctable without a status transition. The verb accepts an `--amend-provenance` flag that, on a task **already** in `complete`, overwrites `implementation_commit` from the supplied `--repo`/`--commit`/`--branch` (or clears it when no provenance flags are given), then exits `0` with a `<task>: provenance amended` line — distinct from a transition. `--amend-provenance` requires the task already be `complete` (else exit `4`), MUST NOT be combined with `--to` (else exit `2`), and follows the same `--commit`-required and syntactic-only rules as a completion write. Provenance remains a single reference; this is correction, not history. (Outside `--amend-provenance`, the strict matrix still forbids `complete → complete`.)

## Parameters

| Name | Required | Description |
|---|---|---|
| `task` | Yes | Task identifier. Board mode: `tasks/<task>/README.md`. Plan-inline mode (with `--plan`): the task block whose `**Id:**` equals `<task>` in `spec/plans/<plan>.md`. |

## Flags

| Flag | Required | Description |
|---|---|---|
| `--to` | Conditional | Target Task status (case-insensitive). Required unless `--amend-provenance` is set (the two are mutually exclusive). Illegal `(from,to)` exits `4`; unknown value exits `2`. |
| `--plan` | No | Plan slug — switches resolution to plan-inline mode (resolves `<task>` by its `**Id:**`). |
| `--repo` | No | Repo of the implementing commit — a repo slug or a full clone URL. Provenance flag; `--to=complete` or `--amend-provenance` only. |
| `--commit` | Conditional | Implementing commit sha. **Required when any provenance flag is present.** Provenance context only. |
| `--branch` | No | Branch the commit landed on. Provenance flag; provenance context only. |
| `--amend-provenance` | No | Correct/overwrite `implementation_commit` on an already-`complete` task without a status transition. Mutually exclusive with `--to`. |
| `--project` | No | Project root. Autodetected per [CLI#req:project-autodetect](../../README.md#req-project-autodetect). |

## Exit codes

| Code | Condition |
|---|---|
| `0` | Transition succeeded (status rewritten; provenance written if supplied; index synced) — OR — provenance amended via `--amend-provenance`. |
| `2` | Missing/unknown `--to`; a provenance flag with non-`complete` `--to`; a provenance flag set without `--commit`; `--amend-provenance` combined with `--to`. |
| `3` | `<task>` resolves to no task in the requested store (plan-inline: no block with matching `**Id:**`). |
| `4` | `(current_status, --to)` not a legal transition; or `--amend-provenance` on a task not in `complete`. |
| `10` | I/O failure, or `spec lint --fix` failed after a successful rewrite (rollback applied). |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [implementation-commit-provenance](https://github.com/specscore/specscore/blob/main/spec/features/implementation-commit-provenance/README.md) (specscore) | The data-model contract this verb writes: reference format, optionality, actor-supplied/syntactic-only rules. This verb is its capture vehicle. |
| [lifecycle-transitions](../../lifecycle-transitions/README.md) | Cross-cutting contract this verb satisfies; its `scope-no-task-lifecycle` REQ is narrowed by [single-actor-task-lifecycle-permitted](#req-single-actor-task-lifecycle-permitted). |
| [cli/task](../README.md) | Parent group (`info`/`list`/`new`); its `no-lifecycle-in-mvp` REQ is narrowed to admit this verb. |
| [cli/plan/change-status](../../plan/change-status/README.md) | Sibling. The plan execution band stays lint-derived; this verb sets **task** status, which is the rollup *input* (`lint --fix` then derives the plan's `Executing`/`Implemented`). |
| [spec lint](../../spec/lint/README.md) | Invoked post-mutation for index/rollup sync; also owns syntactic validation of the provenance reference format. |

## Not Doing / Out of Scope

- Claim/release, locking, sync policy, conflict-aware exit codes, multi-agent coordination — remain orchestrator-owned ([single-actor-task-lifecycle-permitted](#req-single-actor-task-lifecycle-permitted)).
- Auto-deriving the commit from ambient `git HEAD`, and verifying the sha exists in the repo ([provenance-not-derived-not-verified](#req-provenance-not-derived-not-verified)).
- Recording more than one commit per task (single reference — MVP).
- Reachability detection / lost-commit recovery — the meta-spec feature's Not Doing carries.
- A `--note`/`## Resolution` mechanism for tasks — deferred; not part of this MVP.

## Rehearse Integration

CLI behavior is testable. Rehearse stubs SHOULD be scaffolded for the happy-path transition, the provenance-on-complete write, the provenance-flag-without-complete rejection (exit `2`), the illegal-transition rejection (exit `4`), not-found resolution (exit `3`), and the `--amend-provenance` corrective re-stamp. Both board-mode and plan-inline (by `**Id:**`) scenarios are fully specified.

## Acceptance Criteria

### AC: complete-with-provenance

**Requirements:** [cli/task/change-status#req:provenance-flags-optional-and-complete-only](#req-provenance-flags-optional-and-complete-only), [cli/task/change-status#req:provenance-ref-assembly](#req-provenance-ref-assembly)

**Given** a board task `tasks/auth/README.md` in `**Status:** in_progress`
**When** the user runs `specscore task change-status auth --to=complete --repo backstage --commit a1b2c3d --branch feat/auth`
**Then** the command exits `0`, rewrites the status to `complete`, writes `**Implemented-by:** backstage@a1b2c3d (feat/auth)` (the `implementation_commit` property) onto the task, and prints `auth: in_progress → complete`.

### AC: complete-without-provenance-is-valid

**Requirements:** [cli/task/change-status#req:provenance-flags-optional-and-complete-only](#req-provenance-flags-optional-and-complete-only)

**Given** a board task `tasks/auth/README.md` in `**Status:** in_progress`
**When** the user runs `specscore task change-status auth --to=complete` with no provenance flags
**Then** the command exits `0` and sets status `complete` with no `**Implemented-by:**` field written (provenance is optional).

### AC: provenance-flag-without-complete-rejected

**Requirements:** [cli/task/change-status#req:provenance-flags-optional-and-complete-only](#req-provenance-flags-optional-and-complete-only)

**Given** a board task `tasks/auth/README.md` in `**Status:** queued`
**When** the user runs `specscore task change-status auth --to=in_progress --commit a1b2c3d`
**Then** the command exits `2` (InvalidArgs) stating provenance flags are valid only with `--to=complete`, and the task is unchanged.

### AC: provenance-flag-without-commit-rejected

**Requirements:** [cli/task/change-status#req:provenance-ref-assembly](#req-provenance-ref-assembly)

**Given** a board task `tasks/auth/README.md` in `**Status:** in_progress`
**When** the user runs `specscore task change-status auth --to=complete --repo backstage` (no `--commit`)
**Then** the command exits `2` (InvalidArgs) stating `--commit` is required when provenance flags are supplied, and the task is unchanged.

### AC: provenance-not-derived-from-head

**Requirements:** [cli/task/change-status#req:provenance-not-derived-not-verified](#req-provenance-not-derived-not-verified)

**Given** a board task in `**Status:** in_progress`, run inside a git working tree whose `HEAD` is some commit
**When** the user runs `specscore task change-status auth --to=complete` with no provenance flags
**Then** no `**Implemented-by:**` value is written — the verb never reads ambient `HEAD` to populate provenance.

### AC: to-flag-validation

**Requirements:** [cli/task/change-status#req:target-status-flag](#req-target-status-flag)

**Given** a board task `tasks/auth/README.md` in `**Status:** in_progress`
**When** the user runs `specscore task change-status auth` with no `--to` (or with `--to=shipped`, a value that is not one of the seven Task statuses)
**Then** the command exits `2` (InvalidArgs) — naming the missing flag, or the unrecognized status value — and the task is unchanged.

### AC: bare-sha-same-repo-assembly

**Requirements:** [cli/task/change-status#req:provenance-ref-assembly](#req-provenance-ref-assembly)

**Given** a board task `tasks/auth/README.md` in `**Status:** in_progress` in a project where spec and code share a repo
**When** the user runs `specscore task change-status auth --to=complete --commit a1b2c3d` (no `--repo`)
**Then** the command exits `0` and writes a bare `**Implemented-by:** a1b2c3d` (no `<repo>@` prefix), a valid same-repo reference.

### AC: illegal-transition-rejected

**Requirements:** [cli/task/change-status#req:task-legal-transition-matrix](#req-task-legal-transition-matrix)

**Given** a board task in `**Status:** planning`
**When** the user runs `specscore task change-status auth --to=complete`
**Then** the command exits `4` (InvalidTransition) — `planning → complete` is not a legal pair — and the task is unchanged.

### AC: single-actor-no-coordination

**Requirements:** [cli/task/change-status#req:single-actor-task-lifecycle-permitted](#req-single-actor-task-lifecycle-permitted)

**Given** the verb is invoked on a task
**When** it mutates the status field
**Then** it performs a pure file rewrite (plus optional provenance) with no claim/release, lock acquisition, or conflict-resolution step — coordination is not attempted.

### AC: plan-inline-target-resolves

**Requirements:** [cli/task/change-status#req:task-dual-target-resolution](#req-task-dual-target-resolution)

**Given** a plan `spec/plans/auth.md` containing a task block with `**Id:** setup`
**When** the user runs `specscore task change-status setup --plan auth --to=complete --commit a1b2c3d`
**Then** the command resolves the task by its `**Id:**` inside `spec/plans/auth.md` (not the board), sets it `complete`, writes the provenance reference, and the subsequent `lint --fix` recomputes the plan's execution-band status from the task rollup.

### AC: corrective-restamp

**Requirements:** [cli/task/change-status#req:provenance-corrective-restamp](#req-provenance-corrective-restamp)

**Given** a board task `tasks/auth/README.md` already in `**Status:** complete` carrying `**Implemented-by:** backstage@wrongsha`
**When** the user runs `specscore task change-status auth --amend-provenance --repo backstage --commit a1b2c3d`
**Then** the command exits `0`, overwrites `implementation_commit` to `backstage@a1b2c3d` without changing the `complete` status, and prints `auth: provenance amended` — and the same invocation with `--to` set instead exits `2`, while `--amend-provenance` on a non-`complete` task exits `4`.

## Open Questions

- ~~Plan-inline task addressing~~ — **Resolved:** an explicit stable `**Id:**` on the task block (see [task-dual-target-resolution](#req-task-dual-target-resolution)); the plan task-block `**Id:**` field is added upstream in the `plan` Feature.
- ~~Corrective re-stamp of an already-`complete` task~~ — **Resolved:** the `--amend-provenance` path ([provenance-corrective-restamp](#req-provenance-corrective-restamp)).
- Is the MVP transition matrix complete? Should `depends_on` gating (`queued → in_progress` only when dependencies are `complete`) be enforced here or left to orchestrators? (Still open.)

---
*This document follows the https://specscore.md/feature-specification*
