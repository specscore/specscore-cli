---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Plan Reconcile

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan/reconcile?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan/reconcile?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan/reconcile?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan/reconcile?op=request-change) |
**Status:** Approved
**Date:** 2026-07-25
**Owner:** alexander.trakhimenok
**Source Ideas:** —
**Supersedes:** —

## Summary

`specscore plan reconcile <slug> --tasks=complete --note=<text> [--evidence=<ref>[,<ref>...]]` corrects a Plan's recorded `**Status:**` — and its embedded tasks' `**Status:**` lines — to match work that was actually delivered outside the tracked [`change-status`](../change-status/README.md) flow. It is a distinct verb, not an extension of `change-status`: it never walks the legal-transition matrix arc by arc, it writes the corrected state directly, and it always records — via a `**Reconciled:**` header marker and a dated `## Resolution` paragraph — that the jump happened out of band. A `--note` justification is mandatory; there is no bypass path that skips it.

## Problem

Work sometimes gets implemented outside the tracked flow — a fix lands directly on `main` during an incident, a plan's tasks get completed by hand without ever running `task change-status`, or a plan is simply forgotten about after the code shipped. Later, someone discovers a Plan still says `**Status:** Draft` with every task at `planning`, even though the code is demonstrably in production. Before this verb there was **no supported way** to record that truth:

- [`plan change-status`](../change-status/README.md)'s legal-transition matrix only reaches as far as the prep band's `Approved` state (via `Draft → In Review → Approved` or the direct `Draft → Approved` fast-track); the execution band (`Executing`/`Blocked`/`Implemented`/`Failed`) is explicitly refused (exit `2`) because it is derived by `specscore spec lint --fix` from the task-status rollup (canonical [plan#req:execution-status-derived](https://specscore.md/plan-specification)).
- The rollup itself comes from the `**Status:**` lines under each `### Task N:` heading inside the plan body. [`task change-status`](../../task/change-status/README.md) does not touch those — it operates on the separate task board (`tasks/<task>/README.md`) or, in plan-inline mode, exactly one task block at a time by its `**Id:**`.
- So the only way to make an 8-task plan's record match reality was to hand-edit eight `**Status:**` lines plus the plan's own — the exact anti-pattern every other `change-status` verb in this CLI exists to eliminate.

`plan reconcile` closes this gap without reopening the door it closes: it is evidence-gated (mandatory `--note`, optional structured `--evidence`) and self-marking (the artifact itself says "this was reconciled," so a reader is never misled into thinking a plan earned its status through the normal arc).

## Behavior

### REQ: distinct-verb

Reconciliation MUST be a separate verb (`plan reconcile`), never a flag or an accepted `--to` value on `plan change-status`. The two verbs answer different questions — "advance this plan one legal step" versus "correct the record to match reality" — and conflating them would make it impossible to tell, from the CLI surface alone, whether a given status was reached through the tracked flow or asserted after the fact.

### REQ: mandatory-justification

`--note <text>` MUST be required. A missing or whitespace-only `--note` MUST exit `2` (InvalidArgs) before any mutation. This is the mechanism that keeps reconcile from becoming a silent bypass of the state machine: every reconciliation carries a caller-supplied explanation of why the record is being corrected, permanently attached to the artifact.

### REQ: optional-evidence

`--evidence <ref>[,<ref>...]` MAY be supplied as a comma-separated list of commit SHAs, PR URLs, or file paths substantiating the claim that the work is done. When supplied, it is recorded verbatim as a trailing `Evidence: ...` line in the same `## Resolution` paragraph as `--note`. Omitting it is not an error — `--note` alone satisfies [mandatory-justification](#req-mandatory-justification) — but a reconciliation with evidence is strictly more auditable than one without.

### REQ: tasks-flag

`--tasks=complete` MUST be required and, today, is the ONLY accepted value: every `### Task N:` block's `**Status:**` is set to `complete`. A missing `--tasks` or any other value MUST exit `2` (InvalidArgs) naming the offending value; matching is case-insensitive. Per-task selection (reconciling a subset of tasks to a mix of statuses) is deferred — see Open Questions.

### REQ: terminal-task-requires-acknowledgement

`--tasks=complete` MUST NOT silently overwrite a task recorded as `failed` or `aborted`. A task in either terminal status is a deliberate, meaningful claim about what happened — reconcile exists to make the record TRUE, and silently flipping a recorded failure to `complete` just because `--tasks=complete` asked for "every" task would make the record false in exactly the tool whose whole purpose is honest correction.

A task in `blocked` is NOT covered by this REQ: `blocked` has outgoing arcs in the task state machine and is not a terminal claim, so completing it needs no acknowledgement (it is treated the same as `planning`/`queued`/`in_progress`).

Concretely:

- If any task is `failed` or `aborted` and its task number does NOT appear in `--force-tasks`, the whole reconciliation MUST be refused with exit `4` (InvalidState), before any mutation. The error MUST name every offending task by number and current status (e.g. `Task 3 (failed)`) and MUST suggest the exact `--force-tasks=<n>[,<n>...]` value that would acknowledge them.
- `--force-tasks=<n>[,<n>...]` (a comma-separated list of task numbers) is the caller's explicit, per-task acknowledgement that a specific failed/aborted task should be overridden to `complete` anyway. A task number named in `--force-tasks` that does NOT correspond to an actual failed/aborted task is a harmless no-op (not an error) — it simply has nothing to acknowledge.
- Every task actually overridden this way MUST be itemized — by number and prior status — in the `## Resolution` paragraph (see [resolution-paragraph](#req-resolution-paragraph)), in addition to being counted in the aggregate "N task(s) marked complete" figure. An override is never disclosed as an aggregate count ALONE.

### REQ: derived-not-asserted-target

The verb MUST NOT accept a `--to` flag. The plan's target status is always DERIVED from the reconciled task rollup via the same [plan#req:status-rollup](https://specscore.md/plan-specification) precedence `spec lint`'s P-007 rule uses (canonical `DeriveExecutionBand`), never asserted independently. With `--tasks=complete` this derivation always yields `Implemented` (every task complete). This guarantees the plan-level status and the task-level rollup can never disagree after a successful reconciliation — the exact self-contradiction a naive "just rewrite the Status line" fix would risk.

### REQ: no-silent-history

Reconcile MUST NOT reach its target by writing the plan through any intermediate status in the legal-transition matrix. It performs exactly one direct rewrite of the plan's `**Status:**` line (and each reconciled task's `**Status:**` line) to the final values. `pkg/lifecycle`'s state-machine validation (`Transition`/`Validate`) is never invoked by this verb — reconciliation is definitionally outside that machine, not a fast-path through it.

### REQ: reconciled-marker

On the first successful reconciliation of a plan, a `**Reconciled:** <date>` header line MUST be inserted immediately after the plan's `**Status:**` line. This is the always-visible, grep-able signal that the current status did not arrive through the tracked flow. A plan that already carries the marker (a second reconciliation, e.g. after more tasks were added later) MUST NOT gain a second marker line — the marker records only that at least one reconciliation occurred; the full history lives in `## Resolution`.

### REQ: resolution-paragraph

Every successful reconciliation MUST append a `## Resolution` paragraph (per the shared [lifecycle-transitions](../../lifecycle-transitions/README.md) note mechanism) containing: a fixed preamble naming the `<from> → <to>` jump and stating explicitly that it did not walk the legal-transition matrix, the caller's `--note` text verbatim, the itemized `--force-tasks` overrides when any (per [terminal-task-requires-acknowledgement](#req-terminal-task-requires-acknowledgement)), and — when supplied — the `--evidence` list. Multiple reconciliations over a plan's life each get their own paragraph; none are overwritten.

### REQ: disposition-not-resurrected

A plan whose current `**Status:**` is a terminal disposition (`Rejected`, `Withdrawn`, `Superseded`, `Deprecated`) MUST be refused with exit `4` (InvalidState). Reconcile corrects a record that fell behind reality; it does not resurrect a plan a human deliberately closed out. Re-pursuing work on a disposed plan means authoring a new plan, or using `plan change-status` if the disposition itself was recorded in error.

### REQ: structural-preconditions

Reconcile MUST refuse (exit `4`, InvalidState) when the artifact cannot support the operation:

- the plan has no `## Tasks` entries at all (there is nothing to derive a status from);
- any `### Task N:` block is missing an explicit `**Status:**` line (reconcile only rewrites an existing line; it does not author task schema);
- the plan resolves only to the directory form (`spec/plans/<slug>/README.md`) — reconcile supports the flat single-file form only, matching the scope of `pkg/plan.Parse` and the execution-band derivation it feeds.

### REQ: not-idempotent

Per the shared [lifecycle-transitions#req:not-idempotent](../../lifecycle-transitions/README.md#req-not-idempotent) invariant, re-running reconcile when nothing would actually change — every targeted task is already at the requested status AND the plan's `**Status:**` already equals the derived target — MUST exit `4` (InvalidState) rather than silently succeeding a second time. When new work has landed since the prior reconciliation (e.g. a task was added and left at `planning`), a further reconcile call IS legal and proceeds normally per [reconciled-marker](#req-reconciled-marker) and [resolution-paragraph](#req-resolution-paragraph).

### REQ: slug-resolution

The `<slug>` positional MUST resolve to an existing flat-form Plan file (`spec/plans/<slug>.md`) within the project root (autodetected per [CLI#req:project-autodetect](../../README.md#req-project-autodetect), or `--project`). A slug that resolves to neither the flat nor the directory form exits `3` (NotFound), naming the expected flat path.

### REQ: index-sync

The post-mutation `specscore spec lint --fix` (per [lifecycle-transitions#req:index-sync-on-success](../../lifecycle-transitions/README.md#req-index-sync-on-success)) MUST run after the rewrite, syncing the plans index and the frontmatter `status:` mirror. The verb's exit `0` depends on the rewrite AND the lint pass both succeeding; a lint failure rolls back every mutation and exits `10`.

### REQ: execution-band-error-points-here

[`plan change-status`](../change-status/README.md)'s execution-band-not-settable error (triggered by passing an execution-band value as `--to`) MUST name this verb as the corrective path when the underlying work was actually delivered outside the tracked flow, so a caller who hits the illegal-jump wall is pointed at the supported alternative rather than left to hand-edit the file.

## Parameters

| Name | Required | Description |
|---|---|---|
| `slug` | Yes | Plan slug — resolves to `spec/plans/<slug>.md` (flat form only). |

## Flags

| Flag | Required | Description |
|---|---|---|
| `--tasks` | Yes | Which embedded tasks to reconcile. Only supported value: `complete` (case-insensitive). |
| `--note` | Yes | Justification for the reconciliation; written verbatim into a `## Resolution` paragraph. Missing/blank exits `2`. |
| `--evidence` | No | Comma-separated commit SHAs / PR URLs / file paths backing the reconciliation; appended to the same paragraph as `--note`. |
| `--force-tasks` | Conditional | Comma-separated task numbers explicitly acknowledging the override of a `failed`/`aborted` task to `complete`. Required only when such a task exists and is not named here (see [terminal-task-requires-acknowledgement](#req-terminal-task-requires-acknowledgement)); otherwise unused. |
| `--project` | No | Project root. Autodetected per [CLI#req:project-autodetect](../../README.md#req-project-autodetect). |

## Exit codes

| Code | Condition |
|---|---|
| `0` | Reconciliation succeeded; plan and task Status lines rewritten; Resolution paragraph and (on first run) Reconciled marker written; plans index synced. |
| `2` | Missing/malformed `<slug>`; missing or unrecognized `--tasks`; missing/blank `--note`; malformed `--force-tasks` value (not a comma-separated list of positive integers). |
| `3` | No Plan file at `spec/plans/<slug>.md` (nor the directory form). |
| `4` | Plan resolves only to the directory form; plan is in a terminal disposition status; plan has no embedded tasks; a task is missing an explicit `**Status:**` line; a task is `failed`/`aborted` and not acknowledged via `--force-tasks`; or the plan is already reconciled (re-run no-op). |
| `10` | I/O failure, or `spec lint --fix` failed after a successful rewrite (rollback applied). |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [`cli/plan/change-status`](../change-status/README.md) | Sibling verb for the human-authored arcs; its execution-band-rejection error names this verb as the corrective path (REQ:execution-band-error-points-here). The two verbs are deliberately never merged. |
| [`cli/task/change-status`](../../task/change-status/README.md) | Owns single-task, single-actor transitions (board or plan-inline `--id`); reconcile owns the coarser "every embedded task at once, out of band" correction and does not replace it for day-to-day task completion. |
| [lifecycle-transitions](../../lifecycle-transitions/README.md) | Reconcile reuses the shared `## Resolution` note mechanism and the index-sync-on-success contract, but does NOT use the shared state-machine validation (`Transition`/`Validate`) — see REQ:no-silent-history. |
| [spec lint](../../spec/lint/README.md) | Invoked internally for index sync; rule P-007 (execution-band derivation) sees the post-reconcile plan already at its correct, self-consistent state, so it is a no-op on a freshly reconciled plan. |
| [plan (upstream Feature)](https://specscore.md/plan-specification) | `status-rollup` and `execution-status-derived` are the canonical rules this verb's target-status derivation realizes (REQ:derived-not-asserted-target). |

## Dependencies

- cli/plan/change-status
- cli/lifecycle-transitions

## Acceptance Criteria

### AC: eight-tasks-draft-to-implemented

**Requirements:** [tasks-flag](#req-tasks-flag), [derived-not-asserted-target](#req-derived-not-asserted-target), [reconciled-marker](#req-reconciled-marker), [resolution-paragraph](#req-resolution-paragraph)

**Given** `spec/plans/auth.md` with `**Status:** Draft` and 8 `### Task N:` blocks all at `**Status:** planning`
**When** the user runs `specscore plan reconcile auth --tasks=complete --note "implemented directly on main during the outage; tracked flow was skipped"`
**Then** the command exits `0`, every task's `**Status:**` becomes `complete`, the plan's `**Status:**` becomes `Implemented`, a `**Reconciled:**` header line appears immediately after `**Status:**`, and a `## Resolution` paragraph carries the fixed preamble plus the supplied note. `specscore spec lint` passes afterward with zero errors.

### AC: evidence-recorded

**Requirements:** [optional-evidence](#req-optional-evidence)

**Given** the fixture from eight-tasks-draft-to-implemented
**When** the user adds `--evidence a1b2c3d,https://github.com/org/repo/pull/42`
**Then** the `## Resolution` paragraph includes `Evidence: a1b2c3d, https://github.com/org/repo/pull/42`.

### AC: missing-note-rejected

**Requirements:** [mandatory-justification](#req-mandatory-justification)

**Given** any plan eligible for reconciliation
**When** the user runs `specscore plan reconcile auth --tasks=complete` with no `--note`
**Then** the command exits `2` (InvalidArgs) before any mutation, and the plan is unchanged.

### AC: unrecognized-tasks-value-rejected

**Requirements:** [tasks-flag](#req-tasks-flag)

**Given** any plan eligible for reconciliation
**When** the user runs `specscore plan reconcile auth --tasks=some --note "x"`
**Then** the command exits `2`, naming `some` as unrecognized, and the plan is unchanged.

### AC: disposition-refused

**Requirements:** [disposition-not-resurrected](#req-disposition-not-resurrected)

**Given** `spec/plans/auth.md` in `**Status:** Rejected` (or `Withdrawn`, `Superseded`, `Deprecated`)
**When** the user runs `specscore plan reconcile auth --tasks=complete --note "x"`
**Then** the command exits `4`, and the plan is unchanged.

### AC: no-embedded-tasks-refused

**Requirements:** [structural-preconditions](#req-structural-preconditions)

**Given** a plan with no `## Tasks` entries
**When** the user runs `specscore plan reconcile auth --tasks=complete --note "x"`
**Then** the command exits `4`, naming the absence of embedded tasks.

### AC: task-missing-status-line-refused

**Requirements:** [structural-preconditions](#req-structural-preconditions)

**Given** a plan where one `### Task N:` block has no `**Status:**` line
**When** the user runs `specscore plan reconcile auth --tasks=complete --note "x"`
**Then** the command exits `4`, naming the offending task number.

### AC: failed-task-refused

**Requirements:** [terminal-task-requires-acknowledgement](#req-terminal-task-requires-acknowledgement)

**Given** a plan with one task at `**Status:** failed` and no `--force-tasks`
**When** the user runs `specscore plan reconcile auth --tasks=complete --note "x"`
**Then** the command exits `4` before any mutation, naming the offending task by number and status (e.g. `Task 2 (failed)`) and suggesting the `--force-tasks` value that would acknowledge it. The plan is unchanged.

### AC: aborted-task-refused

**Requirements:** [terminal-task-requires-acknowledgement](#req-terminal-task-requires-acknowledgement)

**Given** a plan with one task at `**Status:** aborted` and no `--force-tasks`
**When** the user runs `specscore plan reconcile auth --tasks=complete --note "x"`
**Then** the command exits `4`, naming the offending task, and the plan is unchanged.

### AC: force-tasks-override-succeeds

**Requirements:** [terminal-task-requires-acknowledgement](#req-terminal-task-requires-acknowledgement), [resolution-paragraph](#req-resolution-paragraph)

**Given** a plan with `### Task 2:` at `**Status:** failed`
**When** the user runs `specscore plan reconcile auth --tasks=complete --note "task 2 actually landed on retry" --force-tasks=2`
**Then** the command exits `0`, Task 2's `**Status:**` becomes `complete`, and the `## Resolution` paragraph names the override explicitly (e.g. "Task 2 (was failed)") — not just as part of the aggregate "N task(s) marked complete" count.

### AC: blocked-task-not-guarded

**Requirements:** [terminal-task-requires-acknowledgement](#req-terminal-task-requires-acknowledgement)

**Given** a plan with one task at `**Status:** blocked` and no `--force-tasks`
**When** the user runs `specscore plan reconcile auth --tasks=complete --note "x"`
**Then** the command exits `0`; a `blocked` task needs no acknowledgement and is completed the same as `planning`/`queued`/`in_progress`.

### AC: re-run-refused

**Requirements:** [not-idempotent](#req-not-idempotent)

**Given** a plan already successfully reconciled (every task complete, plan Status already `Implemented`)
**When** the user runs `specscore plan reconcile auth --tasks=complete --note "again"` a second time with no further changes
**Then** the command exits `4`, and the plan is unchanged.

### AC: unknown-slug-not-found

**Requirements:** [slug-resolution](#req-slug-resolution)

**Given** no plan named `ghost`
**When** the user runs `specscore plan reconcile ghost --tasks=complete --note "x"`
**Then** the command exits `3`, naming the expected `spec/plans/ghost.md` path.

### AC: lint-failure-rolls-back

**Requirements:** [index-sync](#req-index-sync)

**Given** a plan eligible for reconciliation
**When** `spec lint --fix` fails after a successful rewrite
**Then** a full rollback restores the plan to its pre-invocation bytes — including removing the `**Reconciled:**` marker and `## Resolution` paragraph if this was the first reconciliation — and the command exits `10`.

### AC: change-status-error-mentions-reconcile

**Requirements:** [execution-band-error-points-here](#req-execution-band-error-points-here)

**Given** a plan in `**Status:** Approved`
**When** the user runs `specscore plan change-status auth --to=implemented`
**Then** the command exits `2`, and the stderr message names `specscore plan reconcile` as the path for recording work delivered outside the tracked flow.

## Open Questions

- **Per-task selection.** Today `--tasks` accepts only `complete` (every embedded task). A plan where only SOME tasks were actually delivered — the rest genuinely still pending — cannot be partially reconciled; the caller must wait until everything is done, or reconcile is extended with per-task syntax (e.g. `--tasks=1,2,5=complete`). Deferred until a real use case demands it. Note this is distinct from `--force-tasks`: that flag is an ACKNOWLEDGEMENT mechanism (which failed/aborted tasks may be overridden), not a target-selection mechanism — it never causes a task to be left untouched.
- **Non-`Implemented` targets.** Because the only supported `--tasks` value forces every task to `complete`, the derived target is always `Implemented`. Reconciling a plan to `Blocked`/`Failed`/`Executing` (a status stuck out of sync mid-flight, not because the work finished) is out of scope for this MVP and would need its own `--tasks` shape.
- **Per-task `--evidence` / `Implemented-by` stamping.** `task change-status --to=complete` stamps a structured `**Implemented-by:**` provenance field per task from `--commit`/`--repo`/`--branch`. Reconcile's `--evidence` is plan-level free text in `## Resolution` only; it does not stamp per-task provenance. Revisit if reconciled plans need the same machine-checkable provenance as normally-completed ones.

---
*This document follows the https://specscore.md/feature-specification*
