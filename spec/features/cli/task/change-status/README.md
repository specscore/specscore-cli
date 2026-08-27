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

The verb also accepts two optional annotation flags, `--note` and `--evidence`, valid on **any** transition (not restricted to `--to=complete` the way the provenance flags are). `--note` records a free-text justification; `--evidence` records a comma-separated list of unstructured supporting references (commit SHAs, PR URLs, file paths, deploy/monitoring links) — distinct from `implementation_commit`, which is a single, syntactically validated code reference. Both are written as their own field (`**Note:**` / `**Evidence:**`) adjacent to `**Status:**`, in the same atomic write as the transition (see [task-annotation-fields](#req-task-annotation-fields)).

The verb is a **single-artifact local transaction** — it performs no distributed claim/release or task orchestration, but it does take a fail-fast advisory lock on the resolved artifact so cooperating writers cannot validate stale bytes. That deliberately narrow shape keeps it within `specscore`'s local-file-mutation model (see [single-actor-task-lifecycle-permitted](#req-single-actor-task-lifecycle-permitted)). It resolves a task in **either** of two stores: the `tasks/` board (the existing `task` group's target) or a plan-inline task block addressed by its stable `**Id:**` (whose completion feeds the plan execution-band rollup).

`specscore task amend` is the separate corrective surface for annotations that
became stale after a transition. It has no status-transition flags and cannot
change `**Status:**` or `**Implemented-by:**`.

## Synopsis

```
specscore task change-status <task> --to=<status> \
  [--plan <plan-slug>] \
  [--repo <repo>] [--commit <sha>] [--branch <branch>] \
  [--note <text>] [--evidence <ref>[,<ref>...]] \
  [--project <path>]
```

## Problem

The Task doc kind had **no CLI status verb**: `specscore task` exposed only `info`/`list`/`new`, and both [`cli/task#req:no-lifecycle-in-mvp`](../README.md#req-no-lifecycle-in-mvp) and the Stable [`cli/lifecycle-transitions#req:scope-no-task-lifecycle`](../../lifecycle-transitions/README.md#req-scope-no-task-lifecycle) deliberately excluded task-status mutation, reserving task lifecycle for external orchestrators because *full* task coordination (claim/release, conflict-aware exit codes) doesn't fit `specscore`'s single-actor model.

That left two gaps. First, tasks reached `complete` only by hand-editing Markdown — the exact anti-pattern `idea`/`feature`/`plan change-status` eliminate. Second, and the trigger for this feature: the [implementation-commit-provenance](https://github.com/specscore/specscore/blob/main/spec/features/implementation-commit-provenance/README.md) feature needs a **capture point** — a moment, at task completion, when the actor records the code commit that did the work, so a plan that shows `Implemented` retains durable evidence even if the code is later lost to a rebase/merge. Recording a commit is a contention-free, single-actor annotation — it does **not** need the coordination machinery the exclusion was protecting. This feature therefore narrows the exclusion to admit a single-actor `task change-status` (no claim/release) and makes it the provenance capture vehicle.

## Behavior

This verb satisfies the cross-cutting lifecycle contract's local transaction and strict non-idempotent state-machine semantics. Board Task mutation has no derived index callback. Plan-inline mutation changes the Task block only; a later explicit lint pass may reconcile the Plan execution-band status.

### Scope amendment: a single-actor task lifecycle is permitted

#### REQ: single-actor-task-lifecycle-permitted

This feature **narrows** two previously-blanket exclusions: [`cli/lifecycle-transitions#req:scope-no-task-lifecycle`](../../lifecycle-transitions/README.md#req-scope-no-task-lifecycle) and [`cli/task#req:no-lifecycle-in-mvp`](../README.md#req-no-lifecycle-in-mvp). A **single-actor** `task change-status` verb is permitted: it MUST perform one fail-fast local artifact transaction for status plus optional provenance/annotations, with contention or a changed preimage exiting `1`. It has no claim/release, sync policy, remote publication, or distributed terminal-state agreement. Those multi-agent coordination concerns remain orchestrator-owned.

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

### Task annotation: note and evidence

#### REQ: annotation-corrective-amendment

`specscore task amend <task>` corrects `**Note:**` and/or `**Evidence:**`
without a status transition. It is valid for every recognized task state: the
task status and implementation-commit provenance remain byte-for-byte
unchanged. A caller explicitly chooses each affected singleton with either a
non-blank replacement (`--note` / `--evidence`) or removal (`--clear-note` /
`--clear-evidence`); at least one operation, plus a single-line `--actor` and
`--reason`, are required. Empty replacement values are rejected rather than
being interpreted as removal.

Before mutation the command MUST reject a target with duplicate or empty
`**Note:**`/`**Evidence:**` fields. A successful amendment leaves no more than
one of each corrected field, preserves unknown text and formatting, and adds a
new append-only `**Annotation Amendment:**` record containing actor, UTC time,
reason, and a SHA-256 digest of the exact pre-amendment artifact. The command
acquires the artifact's fail-fast advisory lock, reads and resolves the target
under that lock, composes the amendment and audit in memory, and makes one
atomic durable replacement. Immediately before rename it compares the current
bytes with the transaction's expected bytes to detect a non-cooperating edit;
this is an expected-byte fence, not a claim of filesystem compare-and-swap
semantics. Contention or changed bytes exit `1` without overwriting the other
writer. Board task files may
be directory (`tasks/<id>/README.md`) or legacy flat (`tasks/<id>.md`); plan
tasks may be in `spec/plans/<id>.md` or legacy
`spec/plans/<id>/README.md`. Plan-inline amendments retain the existing
coordination-branch precondition.

#### REQ: task-annotation-fields

`--note <text>` and `--evidence <ref>[,<ref>...]` are OPTIONAL annotations, independent of `--to`'s value — unlike the provenance flags they are NOT restricted to `--to=complete`; either or both MAY be supplied on any legal transition (e.g. a `--note` explaining why a task moved to `blocked`). Neither is required, and re-running with neither flag writes nothing (matching the existing no-provenance-supplied behavior).

- `--note` is written verbatim (after trimming surrounding whitespace) as a `**Note:**` field. A blank/whitespace-only `--note` writes nothing (treated as absent).
- `--evidence` is split on commas, each entry trimmed, empty entries dropped, and the result written as a single `**Evidence:**` field with entries rejoined by `", "` — mirroring [`plan reconcile`](../../plan/reconcile/README.md)'s `--evidence` flag. An `--evidence` value that reduces to zero entries after trimming writes nothing.
- Both fields are written in the SAME atomic rewrite as the status transition (or, for `--to=complete`, the same write as `**Implemented-by:**`), in the fixed order `**Implemented-by:**` → `**Note:**` → `**Evidence:**`, immediately after `**Status:**`.
- `--note`/`--evidence` are syntactically UNVALIDATED free text/refs — unlike `implementation_commit` ([provenance-ref-assembly](#req-provenance-ref-assembly)), which is a single, format-checked code reference. This is a deliberate distinction: `**Implemented-by:**` answers "which commit did the work"; `**Note:**`/`**Evidence:**` answer "what backs the claim that it's actually done" (e.g. a live-URL check, a manual QA note) — a broader, unstructured category that a strict commit-ref format cannot carry.
- `--note`/`--evidence` MUST NOT be combined with `--amend-provenance` (exit `2`) — use `task amend` for explicit Note/Evidence correction.
- Neither field is part of the [KindTask](https://github.com/specscore/specscore/blob/main/spec/features/plan/README.md#optional-task-id) transition matrix or the plan execution-band rollup: `**Note:**`/`**Evidence:**` are pure annotations that never influence [task-legal-transition-matrix](#req-task-legal-transition-matrix) or [plan#req:status-rollup](https://specscore.md/plan-specification#req-status-rollup). They are parsed by `pkg/plan` (`Task.Note`/`Task.Evidence`) but are NOT yet surfaced by `plan info`'s structured output — a human or tool reads them directly from the plan file today; surfacing them in `plan info` is a deferred [Open Question](#open-questions).

#### REQ: plan-inline-coordination-branch-enforcement

In plan-inline mode (`--plan`), when the resolved plan declares `**Coordination:** <owner>/<repo>@<branch>` (upstream [plan#coordination-branch](https://github.com/specscore/specscore/blob/main/spec/features/plan/README.md#coordination-branch)), the verb MUST enforce the same check as [`plan change-status#req:coordination-branch-enforcement`](../../plan/change-status/README.md#req-coordination-branch-enforcement): resolve the CURRENT invocation's ambient git identity and compare it against the declared reference BEFORE any mutation (including a status transition or a corrective `--amend-provenance` re-stamp), exiting `1` (Conflict) on a mismatch, fail-closed on an unresolvable remote/branch. This is a DIFFERENT concept from the `single-actor-no-coordination` AC below: that AC excludes a distributed actor claim/release protocol, while every local artifact mutation still uses its fail-fast advisory transaction lock. This REQ determines which repo/branch is authoritative for mutating the plan DOCUMENT itself. Board-mode tasks (no `--plan`) resolves to `tasks/<task>/README.md`, which carries no `**Coordination:**` field, so this REQ never applies to them.

#### REQ: plan-inline-coordination-branch-override

The verb MUST accept a `--force-coordination` boolean flag, honored only in plan-inline mode, with the same bypass-and-warn semantics as [`plan change-status#req:coordination-branch-override`](../../plan/change-status/README.md#req-coordination-branch-override). It has no effect in board mode.

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
| `--note` | No | Optional free-text annotation written as `**Note:**`. Valid on ANY transition. NOT supported with `--amend-provenance`. |
| `--evidence` | No | Optional comma-separated supporting references written as `**Evidence:**`. Valid on ANY transition. NOT supported with `--amend-provenance`. |
| `--project` | No | Project root. Autodetected per [CLI#req:project-autodetect](../../README.md#req-project-autodetect). |
| `--force-coordination` | No | Plan-inline mode only. Bypasses the target plan's `**Coordination:**` repo/branch check for this invocation. Prints a `warning:` line to stderr; does not modify the plan's `**Coordination:**` field. No effect in board mode. |

## Exit codes

| Code | Condition |
|---|---|
| `0` | Transition succeeded (status rewritten; provenance/note/evidence written if supplied) — OR — provenance amended via `--amend-provenance`. Board Task mutation has no derived index callback. |
| `1` | Artifact-lock contention or a final expected-byte mismatch; additionally, in plan-inline mode, a coordination repo/branch mismatch or malformed declaration refused before mutation. `--force-coordination` bypasses only that declaration mismatch, never the local transaction conflict. |
| `2` | Missing/unknown `--to`; a provenance flag with non-`complete` `--to`; a provenance flag set without `--commit`; `--amend-provenance` combined with `--to`; `--note`/`--evidence` combined with `--amend-provenance`. |
| `3` | `<task>` resolves to no task in the requested store (plan-inline: no block with matching `**Id:**`). |
| `4` | `(current_status, --to)` not a legal transition; or `--amend-provenance` on a task not in `complete`. |
| `10` | I/O failure or derived-work failure after a committed transaction (recovery required). |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [implementation-commit-provenance](https://github.com/specscore/specscore/blob/main/spec/features/implementation-commit-provenance/README.md) (specscore) | The data-model contract this verb writes: reference format, optionality, actor-supplied/syntactic-only rules. This verb is its capture vehicle. |
| [lifecycle-transitions](../../lifecycle-transitions/README.md) | Cross-cutting contract this verb satisfies; its `scope-no-task-lifecycle` REQ is narrowed by [single-actor-task-lifecycle-permitted](#req-single-actor-task-lifecycle-permitted). This verb does NOT implement the Meta's `--dry-run` ([REQ: dry-run-mode](../../lifecycle-transitions/README.md#req-dry-run-mode) explains why: no shared per-kind `ChangeStatus` orchestrator to sandbox, plus provenance stamping and plan-inline resolution). `task transitions` (the companion read-only verb) covers board-mode tasks only, per [REQ: transitions-query-verb](../../lifecycle-transitions/README.md#req-transitions-query-verb). |
| [cli/task](../README.md) | Parent group (`info`/`list`/`new`); its `no-lifecycle-in-mvp` REQ is narrowed to admit this verb. |
| [cli/plan/change-status](../../plan/change-status/README.md) | Sibling. The plan execution band stays lint-derived; this verb sets **task** status, which is the rollup *input* (`lint --fix` then derives the plan's `Executing`/`Implemented`). It also shares this verb's `--force-coordination`-bypassable coordination-branch enforcement (REQ:plan-inline-coordination-branch-enforcement mirrors its REQ:coordination-branch-enforcement). |
| [spec lint](../../spec/lint/README.md) | Not invoked by this command — board Task mutation writes ONLY `tasks/<task>/README.md`, never the `tasks/README.md` board index, mirroring how feature/idea/decision `change-status` leave their indexes to a later `spec lint --fix` (per the `specscore` meta-spec's [Index feature — REQ: file-authoritative-over-index](https://github.com/specscore/specscore/blob/main/spec/features/index/README.md#req-file-authoritative-over-index)). A later `spec lint --fix` run (the `task-index-row-sync` rule) reconciles the board row's Status from the task file, reporting every reconciled row rather than rewriting it silently. Lint also reconciles a plan-inline Task's derived Plan execution band and validates provenance/P-010 syntax. |
| [plan (upstream Feature)](https://specscore.md/plan-specification) | [plan#coordination-branch](https://specscore.md/plan-specification#coordination-branch) is the source of truth for the `**Coordination:**` field this verb enforces in plan-inline mode. |

## Not Doing / Out of Scope

- Distributed claim/release, sync policy, and multi-agent orchestration remain orchestrator-owned ([single-actor-task-lifecycle-permitted](#req-single-actor-task-lifecycle-permitted)). Local artifact safety does not: every writer takes the same fail-fast advisory artifact lock, and contention exits `1`. [plan-inline-coordination-branch-enforcement](#req-plan-inline-coordination-branch-enforcement) is a separate deterministic precondition (ambient git identity vs. a static `**Coordination:**` field value already on disk), not distributed state.
- Auto-deriving the commit from ambient `git HEAD`, and verifying the sha exists in the repo ([provenance-not-derived-not-verified](#req-provenance-not-derived-not-verified)).
- Recording more than one commit per task (single reference — MVP).
- Reachability detection / lost-commit recovery — the meta-spec feature's Not Doing carries.
- Syntactic validation of `--note`/`--evidence` values — unlike `implementation_commit`, they are free text / unstructured refs by design ([task-annotation-fields](#req-task-annotation-fields)).
- Surfacing `**Note:**`/`**Evidence:**` in `plan info`'s structured output — parsed by `pkg/plan` today but not yet queryable through the CLI; deferred, see [Open Questions](#open-questions).

## Rehearse Integration

CLI behavior is testable. Rehearse stubs SHOULD be scaffolded for the happy-path transition, the provenance-on-complete write, the provenance-flag-without-complete rejection (exit `2`), the illegal-transition rejection (exit `4`), not-found resolution (exit `3`), the `--amend-provenance` corrective re-stamp, and the `--note`/`--evidence` annotation write (including on a non-`complete` transition, and its rejection alongside `--amend-provenance`). Both board-mode and plan-inline (by `**Id:**`) scenarios are fully specified.

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
**Then** it performs one local artifact transaction (fail-fast advisory lock, exact read and resolution under lock, one atomic durable write) with no distributed claim/release or orchestration step.

### AC: plan-inline-target-resolves

**Requirements:** [cli/task/change-status#req:task-dual-target-resolution](#req-task-dual-target-resolution)

**Given** a plan `spec/plans/auth.md` containing a task block with `**Id:** setup`
**When** the user runs `specscore task change-status setup --plan auth --to=complete --commit a1b2c3d`
**Then** the command resolves the task by its `**Id:**` inside `spec/plans/auth.md` (not the board), sets it `complete`, and writes the provenance reference in one artifact transaction. A later explicit `specscore spec lint --fix` may recompute the Plan's execution-band status from the task rollup; this command does not run a second body write.

### AC: corrective-restamp

**Requirements:** [cli/task/change-status#req:provenance-corrective-restamp](#req-provenance-corrective-restamp)

**Given** a board task `tasks/auth/README.md` already in `**Status:** complete` carrying `**Implemented-by:** backstage@wrongsha`
**When** the user runs `specscore task change-status auth --amend-provenance --repo backstage --commit a1b2c3d`
**Then** the command exits `0`, overwrites `implementation_commit` to `backstage@a1b2c3d` without changing the `complete` status, and prints `auth: provenance amended` — and the same invocation with `--to` set instead exits `2`, while `--amend-provenance` on a non-`complete` task exits `4`.

### AC: note-and-evidence-any-transition

**Requirements:** [cli/task/change-status#req:task-annotation-fields](#req-task-annotation-fields)

**Given** a plan-inline task block `**Id:** setup` in `spec/plans/auth.md` at `**Status:** in_progress`
**When** the user runs `specscore task change-status setup --plan auth --to=blocked --note "waiting on Firebase console access"`
**Then** the command exits `0`, sets the block's status to `blocked`, and writes `**Note:** waiting on Firebase console access` immediately after `**Status:**` — demonstrating `--note` is valid on a non-`complete` transition, unlike the provenance flags.

### AC: note-and-evidence-with-provenance-ordering

**Requirements:** [cli/task/change-status#req:task-annotation-fields](#req-task-annotation-fields)

**Given** a board task `tasks/auth/README.md` in `**Status:** in_progress`
**When** the user runs `specscore task change-status auth --to=complete --repo sneat-co/chess --commit cfabf5e --note "shipped to production, verified live" --evidence cfabf5e,https://example.com/live`
**Then** the command exits `0` and writes, in ONE atomic rewrite immediately after `**Status:** complete`, three fields in fixed order: `**Implemented-by:** sneat-co/chess@cfabf5e`, then `**Note:** shipped to production, verified live`, then `**Evidence:** cfabf5e, https://example.com/live`.

### AC: note-evidence-blank-writes-nothing

**Requirements:** [cli/task/change-status#req:task-annotation-fields](#req-task-annotation-fields)

**Given** a board task `tasks/auth/README.md` in `**Status:** in_progress`
**When** the user runs `specscore task change-status auth --to=complete --note "   " --evidence " , ,"`
**Then** the command exits `0`, sets status `complete`, and writes neither a `**Note:**` nor an `**Evidence:**` field — both reduce to empty after trimming and are treated as absent.

### AC: note-evidence-not-written-on-rejected-transition

**Requirements:** [cli/task/change-status#req:task-annotation-fields](#req-task-annotation-fields)

**Given** a board task `tasks/auth/README.md` in `**Status:** planning`
**When** the user runs `specscore task change-status auth --to=complete --note "should not land"` (an illegal `planning → complete` pair)
**Then** the command exits `4` (InvalidTransition) and the task file is byte-unchanged — no `**Note:**` field is written on a rejected transition.

### AC: note-evidence-with-amend-provenance-rejected

**Requirements:** [cli/task/change-status#req:task-annotation-fields](#req-task-annotation-fields)

**Given** a board task `tasks/auth/README.md` already `**Status:** complete`
**When** the user runs `specscore task change-status auth --amend-provenance --commit a1b2c3d --note "not allowed here"`
**Then** the command exits `2` (InvalidArgs) naming `--amend-provenance` as incompatible with `--note`/`--evidence`, and the task file is byte-unchanged. The same holds for `--evidence` in place of `--note`.

### AC: plan-inline-coordination-mismatch-rejected

**Requirements:** [plan-inline-coordination-branch-enforcement](#req-plan-inline-coordination-branch-enforcement)

**Given** `spec/plans/auth.md` declaring `**Coordination:** specscore/specscore-cli@main` with a task block `**Id:** setup` at `**Status:** in_progress`, invoked from a git checkout on a different branch
**When** the user runs `specscore task change-status setup --plan auth --to=complete`
**Then** the command exits `1` (Conflict) before any mutation, and the plan file is byte-unchanged.

### AC: plan-inline-coordination-force-bypasses

**Requirements:** [plan-inline-coordination-branch-override](#req-plan-inline-coordination-branch-override)

**Given** the same mismatched checkout as `plan-inline-coordination-mismatch-rejected`
**When** the user adds `--force-coordination`
**Then** the command exits `0`, a `warning:`-prefixed line naming the plan and `--force-coordination` is printed to stderr, and the task's status is rewritten as usual.

### AC: board-mode-unaffected-by-coordination

**Requirements:** [plan-inline-coordination-branch-enforcement](#req-plan-inline-coordination-branch-enforcement)

**Given** a board task `tasks/auth/README.md` in `**Status:** in_progress` (no `--plan`), invoked from a directory that is not a git repository at all
**When** the user runs `specscore task change-status auth --to=complete`
**Then** the command exits `0` exactly as before this feature — board mode never resolves a plan file, so no `**Coordination:**` check ever applies to it.

## Open Questions

- ~~Plan-inline task addressing~~ — **Resolved:** an explicit stable `**Id:**` on the task block (see [task-dual-target-resolution](#req-task-dual-target-resolution)); the plan task-block `**Id:**` field is added upstream in the `plan` Feature.
- ~~Corrective re-stamp of an already-`complete` task~~ — **Resolved:** the `--amend-provenance` path ([provenance-corrective-restamp](#req-provenance-corrective-restamp)).
- ~~A `--note`/annotation mechanism for tasks~~ — **Resolved:** [task-annotation-fields](#req-task-annotation-fields) adds `--note`/`--evidence`, valid on any transition, written adjacent to `**Status:**`.
- Is the MVP transition matrix complete? Should `depends_on` gating (`queued → in_progress` only when dependencies are `complete`) be enforced here or left to orchestrators? (Still open.)
- ~~Should `**Note:**`/`**Evidence:**` be correctable without a further status transition?~~ — **Resolved:** `task amend` provides explicit replacement/removal, append-only audit provenance, duplicate rejection, a fail-fast artifact lock, and a final expected-byte fence ([annotation-corrective-amendment](#req-annotation-corrective-amendment)).
- Should `plan info` surface each task's `**Note:**`/`**Evidence:**` (already parsed into `pkg/plan.Task`) the way it surfaces `ImplementationEvidence` (`**Implemented-by:**` refs) today? Deferred — not required by the motivating case (a plain rollup count), and it would need a decision on whether to fold it into `ImplementationEvidence` or add a parallel field.

---
*This document follows the https://specscore.md/feature-specification*
