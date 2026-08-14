---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Plan Change-Status

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan/change-status?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan/change-status?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan/change-status?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan/change-status?op=request-change) |
**Status:** Approved
**Date:** 2026-06-17
**Owner:** alexander.trakhimenok
**Source Ideas:** —
**Supersedes:** —

## Summary

`specscore plan change-status <slug> --to=<status> [--note] [--successor]` transitions a Plan artifact from its current `**Status:**` to the target named by `--to`. It implements the [lifecycle-transitions](../../lifecycle-transitions/README.md) shared contract for the Plan kind. Plans are flat single files (`spec/plans/<slug>.md`, with the optional directory form `spec/plans/<slug>/README.md`) and never relocate, so — like `feature change-status` — this verb has **no file-relocation side effect**: every transition is a pure status rewrite + index sync (plus the optional `## Resolution` note and `**Superseded By:**` reference).

Crucially, this verb owns **only the human-authored transitions**: the prep band (`Draft`, `In Review`, `Approved`) and the dispositions (`Rejected`, `Withdrawn`, `Superseded`, `Deprecated`). The **execution band** (`Executing`, `Blocked`, `Implemented`, `Failed`) is **lint-derived** — `specscore spec lint --fix` computes it from the plan's task-status rollup (canonical [plan#req:status-rollup](https://specscore.md/plan-specification), realized by lint rule P-007) — and MUST NOT be set via this verb.

## Synopsis

```
specscore plan change-status <slug> --to=<status> [--note <markdown>] [--successor <plan-slug>] [--project <path>]
```

## Problem

Plans move through a document lifecycle, but historically there was **no CLI verb** for transitioning a Plan's status: `specscore plan` exposed only `info`/`list`/`new`. So the `specstudio:plan` and `specstudio:implement` skills transitioned plan status by **hand-editing** the body `**Status:**` line — the exact anti-pattern that `idea`/`feature change-status` eliminate. Hand-edits skip the legal-transition matrix (a hand-edit can jump `Draft → Implemented` silently), forget the index sync, lose the reason, and violate the SpecScore tenet that every human status transition goes through a CLI verb. This verb closes the gap for the human-authored arcs of the Plan kind, while deliberately leaving the execution band to `lint --fix`.

## Behavior

This verb inherits every cross-cutting rule from [lifecycle-transitions](../../lifecycle-transitions/README.md), including one committed artifact transaction, fail-fast lock contention, and retained recovery-required derived-work failures.

### Legal-transition matrix

Only the human-authored `(from, to)` pairs below are accepted; any other pair exits `4` (InvalidTransition). The matrix mirrors the canonical [plan#req:status-transitions](https://specscore.md/plan-specification), restricted to the human-owned arcs. The execution band is reachable only via `lint --fix`, so there is deliberately **no** `Approved → Executing` arc here; the execution-band states appear below only as **From**-states for the dispositions, so a plan that lint has already advanced into the execution band can still be retired by a human.

| From | To | Notes |
|---|---|---|
| `Draft` | `In Review` | submit for review |
| `Draft` | `Approved` | direct approve — fast-track, skips review |
| `In Review` | `Draft` | revisions requested |
| `In Review` | `Approved` | approve |
| `In Review` | `Rejected` | rejected outright (not sent back for revisions) |
| `Approved` | `Withdrawn` | disposition — **reason required** |
| `Executing` | `Withdrawn` | disposition — **reason required** |
| `Blocked` | `Withdrawn` | disposition — **reason required** |
| `Implemented` | `Withdrawn` | disposition — **reason required** |
| `Failed` | `Withdrawn` | disposition — **reason required** |
| `Approved` | `Superseded` | disposition — **reason + successor required** |
| `Executing` | `Superseded` | disposition — **reason + successor required** |
| `Blocked` | `Superseded` | disposition — **reason + successor required** |
| `Implemented` | `Superseded` | disposition — **reason + successor required** |
| `Failed` | `Superseded` | disposition — **reason + successor required** |
| `Approved` | `Deprecated` | disposition — no named successor |
| `Executing` | `Deprecated` | disposition — no named successor |
| `Blocked` | `Deprecated` | disposition — no named successor |
| `Implemented` | `Deprecated` | disposition — no named successor |
| `Failed` | `Deprecated` | disposition — no named successor |

#### REQ: legal-transition-matrix

The verb MUST accept only the human-authored `(from, to)` pairs in the matrix above; any other pair MUST exit `4` (InvalidTransition) per [lifecycle-transitions#req:state-machine-strictness](../../lifecycle-transitions/README.md#req-state-machine-strictness), with a stderr message naming both the current status and the legal target statuses from the current state. Reverse and skip transitions (e.g. `Approved → Draft`, `Draft → Implemented`) are NOT in the matrix and exit `4`. Per the Meta's [not-idempotent](../../lifecycle-transitions/README.md#req-not-idempotent) invariant, re-running on the current status exits `4`. `Draft → Approved` is a direct, human-authored fast-track arc: a reviewer may approve a plan without first routing it through `In Review`, mirroring the Idea and Feature matrices where the same direct arc is already legal. The two-step `Draft → In Review → Approved` path remains legal and unchanged.

#### REQ: execution-band-not-settable

The execution-band statuses (`Executing`, `Blocked`, `Implemented`, `Failed`) are recognized Plan statuses but are **lint-derived**, not human-settable. Passing one as `--to` MUST exit `2` (InvalidArgs) BEFORE state-machine validation, with a stderr message stating the value is a lint-derived execution-band status set by `specscore spec lint --fix` from the task rollup, not via `change-status`, and naming [`specscore plan reconcile`](../reconcile/README.md) as the corrective path when the work was actually delivered outside the tracked flow. This is the verb-level guard that keeps the authority handoff at `Approved` (canonical [plan#req:execution-status-derived](https://specscore.md/plan-specification)).

#### REQ: target-status-flag

The verb MUST accept the target status via a required `--to=<status>` flag whose value is a human-settable Plan status (`Draft`, `In Review`, `Approved`, `Rejected`, `Withdrawn`, `Superseded`, `Deprecated`). A missing `--to` exits `2`. Matching is case-insensitive; the canonical title-case value is written to the file and the success line. Multi-word values use shell quoting (`--to="In Review"`). An unrecognized value (not a Plan status at all) exits `2` with a message naming the offending value; an execution-band value exits `2` per [execution-band-not-settable](#req-execution-band-not-settable).

#### REQ: plan-slug-resolution

The `<slug>` positional MUST resolve to an existing Plan file within the project root (autodetected per [CLI#req:project-autodetect](../../README.md#req-project-autodetect), or `--project`): the flat form `spec/plans/<slug>.md` is tried first, then the optional directory form `spec/plans/<slug>/README.md`. A slug that resolves to neither exits `3` (NotFound), naming the expected flat path. The verb never relocates the resolved file.

#### REQ: disposition-reason-required

Both **disposition** transitions to `Withdrawn` and to `Superseded` are **reason-required** per [lifecycle-transitions#req:reason-required-transitions](../../lifecycle-transitions/README.md#req-reason-required-transitions): `--note <markdown>` is mandatory (for `Withdrawn`, *why* the plan was abandoned; for `Superseded`, *what* superseded it). A missing or empty/whitespace-only `--note` on `--to=withdrawn` or `--to=superseded` MUST exit `2` (InvalidArgs) before any mutation, naming the transition and stating a reason is required. The `Rejected` and `Deprecated` dispositions and all prep transitions keep `--note` optional; when supplied, the note is written per [lifecycle-transitions#req:optional-transition-note](../../lifecycle-transitions/README.md#req-optional-transition-note) (a `## Resolution` section appended to the plan body).

#### REQ: superseded-requires-successor

A `Superseded` plan MUST reference its successor plan (canonical [plan#req:valid-statuses](https://specscore.md/plan-specification): "A `Superseded` plan MUST carry a reference to its successor plan"). Therefore `--to=superseded` MUST require a `--successor <plan-slug>` flag in addition to `--note`. The successor MUST resolve to an existing plan (flat or directory form) in the same project; an absent flag, an empty value, or an unresolvable successor MUST exit `2` (InvalidArgs) before any mutation. On success the verb writes a `**Superseded By:** <plan-slug>` header line into the plan body, mirroring the Decision "Superseded By" convention. For every non-`Superseded` transition, supplying `--successor` MUST exit `2`.

#### REQ: plans-index-sync

The post-mutation lint/index sync runs after the committed Plan transaction. Exit `0` requires both stages; a derived-work failure exits `10` with the committed Plan retained for recovery.

#### REQ: coordination-branch-enforcement

When the resolved plan declares `**Coordination:** <owner>/<repo>@<branch>` (upstream [plan#coordination-branch](https://github.com/specscore/specscore/blob/main/spec/features/plan/README.md#coordination-branch)), the verb MUST resolve the CURRENT invocation's ambient git identity — the `origin` remote (owner/repo, parsed per `pkg/gitremote`) and the checked-out branch, both rooted at the resolved project root — and compare it against the declared reference BEFORE any mutation. Owner/repo comparison is case-insensitive; branch comparison is exact. An unresolvable git remote, a non-GitHub remote, a detached HEAD, or a directory that is not a git repository at all MUST be treated as a mismatch (fail closed), never silently accepted. A present-but-malformed `**Coordination:**` value (already a `P-010` lint violation) MUST also be treated as a mismatch rather than silently skipping the check. A mismatch MUST exit `1` (Conflict) with a message naming the plan, the declared reference, what was actually found, and the `--force-coordination` override, BEFORE any file mutation. A plan with no `**Coordination:**` field is unrestricted and this check is a no-op.

#### REQ: coordination-branch-override

The verb MUST accept a `--force-coordination` boolean flag. When set, a coordination-branch mismatch (including a malformed value) MUST NOT be refused: the verb prints a `warning:`-prefixed line to stderr naming the plan and what was bypassed, then proceeds with the mutation exactly as if the check had passed. `--force-coordination` has no effect when the plan declares no `**Coordination:**` field, or when the current invocation already matches.

## Parameters

| Name | Required | Description |
|---|---|---|
| `slug` | Yes | Plan slug — resolves to `spec/plans/<slug>.md` (or the directory form). |

## Flags

| Flag | Required | Description |
|---|---|---|
| `--to` | Yes | Target status. Legal values: `draft`, `in review`, `approved`, `rejected`, `withdrawn`, `superseded`, `deprecated` (case-insensitive). The execution-band values exit `2`. |
| `--note` | Conditional | Markdown appended as a `## Resolution` section. **Required** for `--to=withdrawn` and `--to=superseded`; optional otherwise. |
| `--successor` | Conditional | Slug of the plan that replaces this one. **Required** for `--to=superseded` (must resolve to an existing plan), rejected for every other transition. |
| `--project` | No | Project root. Autodetected per [CLI#req:project-autodetect](../../README.md#req-project-autodetect). |
| `--force-coordination` | No | Bypasses the plan's `**Coordination:**` repo/branch check for this invocation. Prints a `warning:` line to stderr; does not modify the plan's `**Coordination:**` field. No effect when the plan declares no `**Coordination:**` field, or the check already passes. |

## Exit codes

| Code | Condition |
|---|---|
| `0` | Transition succeeded; file rewritten; plans index synced. |
| `1` | The plan declares `**Coordination:** <owner>/<repo>@<branch>` and the current invocation's git repo/branch does not match (or the value is malformed) — refused BEFORE any mutation. Not reached when `--force-coordination` is set. |
| `2` | Missing/malformed `<slug>`; missing `--to`; unrecognized `--to`; an execution-band `--to`; missing required `--note` on `--to=superseded`/`--to=withdrawn`; missing/unresolvable `--successor` on `--to=superseded`; `--successor` on a non-superseded transition. |
| `3` | No Plan file at `spec/plans/<slug>.md` (nor the directory form). |
| `4` | `(current_status, --to)` is not a legal transition per the matrix. |
| `10` | I/O failure, or derived lint/index work failed after the atomic Plan transaction committed; committed bytes are retained and the error requires recovery. |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [lifecycle-transitions](../../lifecycle-transitions/README.md) | Defines every cross-cutting REQ this verb satisfies; this verb declares no relocation side effect and consumes the `reason-required-transitions` mechanism for `Withdrawn`/`Superseded`. Also implements the Meta's [`--dry-run`](../../lifecycle-transitions/README.md#req-dry-run-mode) and [`transitions`](../../lifecycle-transitions/README.md#req-transitions-query-verb) verbs (the coordination-branch check runs against the real project root even under `--dry-run`, since it only reads ambient git state). |
| [`cli/feature/change-status`](../../feature/change-status/README.md) | Closest sibling — also a flat, relocation-free `change-status`. Mirrors its structure and its index-sync coupling. |
| [`cli/idea/change-status`](../../idea/change-status/README.md) | Sibling verb for the Idea kind (which DOES relocate on archive). |
| [`cli/plan/reconcile`](../reconcile/README.md) | Deliberately separate verb for the out-of-band correction path: when work was delivered outside the tracked flow, this verb's execution-band-rejection error points there instead of letting a caller hand-edit the file. Both verbs independently enforce the same coordination-branch check. |
| [plan (CLI group)](../README.md) | Parent group (`info`/`list`/`new`); this adds the lifecycle verb. |
| [spec lint](../../spec/lint/README.md) | Invoked internally for index sync; rule P-007 derives the execution band that this verb refuses to set; rule P-010 validates the syntax of the `**Coordination:**` field this verb enforces at runtime. |
| [plan (upstream Feature)](https://specscore.md/plan-specification) | The canonical lifecycle: `status-transitions`, `valid-statuses`, `execution-status-derived`, and `status-rollup` are the source of truth this verb realizes for the human band. [plan#coordination-branch](https://specscore.md/plan-specification#coordination-branch) is the source of truth for the `**Coordination:**` field's syntax and intended meaning; this verb is its enforcement. |

## Dependencies

- cli/lifecycle-transitions

## Acceptance Criteria

### AC: draft-to-in-review-happy-path

**Requirements:** [cli/plan/change-status#req:legal-transition-matrix](#req-legal-transition-matrix), [cli/plan/change-status#req:target-status-flag](#req-target-status-flag), [lifecycle-transitions#req:status-line-rewrite](../../lifecycle-transitions/README.md#req-status-line-rewrite), [lifecycle-transitions#req:success-output-format](../../lifecycle-transitions/README.md#req-success-output-format)

**Given** `spec/plans/auth.md` containing `**Status:** Draft`
**When** the user runs `specscore plan change-status auth --to="in review"`
**Then** the command exits `0`, writes exactly `auth: Draft → In Review\n` to stdout, rewrites the Status line to `In Review`, and leaves all other bytes unchanged.

### AC: in-review-back-to-draft

**Requirements:** [cli/plan/change-status#req:legal-transition-matrix](#req-legal-transition-matrix)

**Given** `spec/plans/auth.md` in `**Status:** In Review`
**When** the user runs `specscore plan change-status auth --to=draft`
**Then** the command exits `0` with stdout `auth: In Review → Draft\n`. The revisions-requested arc is legal.

### AC: in-review-to-approved-happy-path

**Requirements:** [cli/plan/change-status#req:legal-transition-matrix](#req-legal-transition-matrix)

**Given** `spec/plans/auth.md` in `**Status:** In Review`
**When** the user runs `specscore plan change-status auth --to=approved`
**Then** the command exits `0` with stdout `auth: In Review → Approved\n`.

### AC: draft-to-approved-direct-happy-path

**Requirements:** [cli/plan/change-status#req:legal-transition-matrix](#req-legal-transition-matrix)

**Given** `spec/plans/auth.md` containing `**Status:** Draft`
**When** the user runs `specscore plan change-status auth --to=approved`
**Then** the command exits `0`, writes exactly `auth: Draft → Approved\n` to stdout, and rewrites the Status line to `Approved`. The two-step `Draft → In Review → Approved` path (the two ACs above) remains legal and unaffected by this direct arc.

### AC: execution-band-rejected

**Requirements:** [cli/plan/change-status#req:execution-band-not-settable](#req-execution-band-not-settable)

**Given** `spec/plans/auth.md` in `**Status:** Approved`
**When** the user runs `specscore plan change-status auth --to=executing` (or `blocked`, `implemented`, `failed`)
**Then** the command exits `2` BEFORE any state-machine check, with a stderr message stating the value is a lint-derived execution-band status set by `specscore spec lint --fix`, not via `change-status`, and naming `specscore plan reconcile` as the path for recording work already delivered outside the tracked flow. The plan is unchanged.

### AC: withdrawn-requires-reason

**Requirements:** [cli/plan/change-status#req:disposition-reason-required](#req-disposition-reason-required)

**Given** `spec/plans/auth.md` in `**Status:** Approved`
**When** the user runs `specscore plan change-status auth --to=withdrawn` with no `--note`
**Then** the command exits `2` (InvalidArgs), naming the `Withdrawn` transition and stating a reason is required. The plan is unchanged.

### AC: withdrawn-with-reason-writes-resolution

**Requirements:** [cli/plan/change-status#req:legal-transition-matrix](#req-legal-transition-matrix), [cli/plan/change-status#req:disposition-reason-required](#req-disposition-reason-required), [lifecycle-transitions#req:optional-transition-note](../../lifecycle-transitions/README.md#req-optional-transition-note)

**Given** `spec/plans/auth.md` in `**Status:** Approved`
**When** the user runs `specscore plan change-status auth --to=withdrawn --note "abandoned after the v2 pivot"`
**Then** the command exits `0`, rewrites the Status line to `Withdrawn`, appends a `## Resolution` section whose text includes `abandoned after the v2 pivot`, and writes stdout `auth: Approved → Withdrawn\n`.

### AC: superseded-requires-reason-and-successor

**Requirements:** [cli/plan/change-status#req:disposition-reason-required](#req-disposition-reason-required), [cli/plan/change-status#req:superseded-requires-successor](#req-superseded-requires-successor)

**Given** `spec/plans/auth.md` in `**Status:** Approved`
**When** the user runs `specscore plan change-status auth --to=superseded` with no `--note` (or with `--note` but no `--successor`, or with a `--successor` that does not resolve)
**Then** the command exits `2` (InvalidArgs) before any mutation, naming the missing requirement. The plan is unchanged.

### AC: superseded-writes-successor-reference

**Requirements:** [cli/plan/change-status#req:superseded-requires-successor](#req-superseded-requires-successor), [lifecycle-transitions#req:optional-transition-note](../../lifecycle-transitions/README.md#req-optional-transition-note)

**Given** `spec/plans/auth.md` in `**Status:** Approved` and an existing `spec/plans/auth-v2.md`
**When** the user runs `specscore plan change-status auth --to=superseded --note "replaced by auth-v2" --successor auth-v2`
**Then** the command exits `0`, rewrites the Status line to `Superseded`, writes a `**Superseded By:** auth-v2` header line, appends a `## Resolution` section including `replaced by auth-v2`, and writes stdout `auth: Approved → Superseded\n`.

### AC: successor-rejected-on-non-superseded

**Requirements:** [cli/plan/change-status#req:superseded-requires-successor](#req-superseded-requires-successor)

**Given** `spec/plans/auth.md` in `**Status:** In Review`
**When** the user runs `specscore plan change-status auth --to=approved --successor auth-v2`
**Then** the command exits `2`, stating `--successor` is only valid with `--to=superseded`. The plan is unchanged.

### AC: disposition-reachable-from-execution-band

**Requirements:** [cli/plan/change-status#req:legal-transition-matrix](#req-legal-transition-matrix)

**Given** `spec/plans/auth.md` in a lint-derived execution-band status (e.g. `**Status:** Executing`)
**When** the user runs `specscore plan change-status auth --to=withdrawn --note "abandoned mid-flight"`
**Then** the command exits `0` with stdout `auth: Executing → Withdrawn\n`. A human may retire a plan that lint advanced into the execution band, even though the human cannot set the execution band directly.

### AC: case-insensitive-to-flag

**Requirements:** [cli/plan/change-status#req:target-status-flag](#req-target-status-flag)

**Given** `spec/plans/auth.md` in `**Status:** In Review`
**When** the user runs `specscore plan change-status auth --to=APPROVED`, `--to=Approved`, or `--to=approved`
**Then** all three behave identically; the file is always written with canonical title-case (`**Status:** Approved`).

### AC: illegal-transition-rejected

**Requirements:** [cli/plan/change-status#req:legal-transition-matrix](#req-legal-transition-matrix), [lifecycle-transitions#req:state-machine-strictness](../../lifecycle-transitions/README.md#req-state-machine-strictness)

**Given** `spec/plans/auth.md` in `**Status:** Approved`
**When** the user runs `specscore plan change-status auth --to=draft`
**Then** the command exits `4`, with a stderr message naming `Approved` as the current status and the legal targets (`Deprecated`, `Superseded`, `Withdrawn`). No file change.

### AC: already-at-target-rejected

**Requirements:** [cli/plan/change-status#req:legal-transition-matrix](#req-legal-transition-matrix), [lifecycle-transitions#req:not-idempotent](../../lifecycle-transitions/README.md#req-not-idempotent)

**Given** `spec/plans/auth.md` in `**Status:** Approved`
**When** the user runs `specscore plan change-status auth --to=approved`
**Then** the command exits `4`. Re-running on the current state is illegal per the strict state machine.

### AC: unrecognized-to-value-rejected

**Requirements:** [cli/plan/change-status#req:target-status-flag](#req-target-status-flag)

**Given** `spec/plans/auth.md`
**When** the user runs `specscore plan change-status auth --to=banana`
**Then** the command exits `2` BEFORE any state-machine check, with stderr that `banana` is not a recognized Plan status.

### AC: missing-slug-rejected

**Requirements:** [lifecycle-transitions#req:slug-positional](../../lifecycle-transitions/README.md#req-slug-positional)

**Given** a project with plans
**When** the user runs `specscore plan change-status --to=approved` with no positional argument
**Then** the command exits `2`.

### AC: missing-to-flag-rejected

**Requirements:** [cli/plan/change-status#req:target-status-flag](#req-target-status-flag)

**Given** `spec/plans/auth.md`
**When** the user runs `specscore plan change-status auth` (no `--to`)
**Then** the command exits `2` with stderr stating that `--to` is required.

### AC: plan-not-found

**Requirements:** [cli/plan/change-status#req:plan-slug-resolution](#req-plan-slug-resolution)

**Given** no plan named `nonexistent`
**When** the user runs `specscore plan change-status nonexistent --to=approved`
**Then** the command exits `3` with stderr naming the expected `spec/plans/nonexistent.md` path.

### AC: lint-failure-retains-committed-transaction

**Requirements:** [cli/plan/change-status#req:plans-index-sync](#req-plans-index-sync)

**Given** `spec/plans/auth.md` in `**Status:** Draft`
**When** `spec lint --fix` fails after a successful Status rewrite
**Then** the command exits `10` with a committed/recovery-required error naming the lint violation(s), while the new `**Status:**` and any same-transaction `## Resolution` or `**Superseded By:**` bytes remain visible. No late rollback is attempted.

### AC: coordination-mismatch-rejected

**Requirements:** [cli/plan/change-status#req:coordination-branch-enforcement](#req-coordination-branch-enforcement)

**Given** `spec/plans/auth.md` in `**Status:** Draft` and declaring `**Coordination:** specscore/specscore-cli@main`, invoked from a git checkout whose origin remote is `specscore/specscore-cli` but whose checked-out branch is `some-other-branch`
**When** the user runs `specscore plan change-status auth --to="in review"`
**Then** the command exits `1` (Conflict) BEFORE any mutation, with a stderr message naming `auth`, the declared `specscore/specscore-cli@main` reference, the actual branch, and `--force-coordination`. The plan file is byte-for-byte unchanged.

### AC: coordination-match-proceeds

**Requirements:** [cli/plan/change-status#req:coordination-branch-enforcement](#req-coordination-branch-enforcement)

**Given** the same plan as `coordination-mismatch-rejected`, invoked from a git checkout on branch `main` with origin remote `specscore/specscore-cli`
**When** the user runs `specscore plan change-status auth --to="in review"`
**Then** the command exits `0` exactly as it would with no `**Coordination:**` field, and no warning is printed.

### AC: coordination-force-bypasses

**Requirements:** [cli/plan/change-status#req:coordination-branch-override](#req-coordination-branch-override)

**Given** the same mismatched checkout as `coordination-mismatch-rejected`
**When** the user runs `specscore plan change-status auth --to="in review" --force-coordination`
**Then** the command exits `0`, a `warning:`-prefixed line naming `auth` and `--force-coordination` is printed to stderr, and the status is rewritten exactly as in the matched case.

## Open Questions

- **Reverse transitions out of dispositions.** The canonical lifecycle has no resurrection from a disposition status — re-pursuing the work means authoring a new plan. This verb encodes that (no arc out of `Rejected`/`Withdrawn`/`Superseded`/`Deprecated`). Whether to relax this is deferred.
- **`In Review → Deprecated`/`Withdrawn`.** The canonical matrix permits dispositions only from `Approved` onward, so a `Draft`/`In Review` plan cannot be `Withdrawn` directly; the human path is `In Review → Rejected`. Confirm this matches real authoring flows.

---
*This document follows the https://specscore.md/feature-specification*
