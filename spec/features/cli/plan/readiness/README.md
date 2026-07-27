---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Plan readiness (CLI)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan/readiness?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan/readiness?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan/readiness?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan/readiness?op=request-change) |
**Status:** Approved
**Date:** 2026-07-27
**Owner:** alexander.trakhimenok
**Source Ideas:** specscore-cli-should-expose-a-plan-verb-with-list-and-query
**Supersedes:** —

## Summary

`specscore plan readiness <slug>` gives dispatchers one stable, read-only answer to whether a Plan's same-repository `**Prerequisite Plans:**` permit execution. Approval remains a human review decision; readiness is separate and requires every prerequisite to derive `Implemented` from its task rollup.

## Behavior

#### REQ: readiness-structured-output

The command MUST return `slug`, `ready`, and `unmet_prerequisites` in YAML (default) and JSON. `ready: false` is normal query data and exits `0`, so agents can inspect all unmet items. Each unmet item MUST include its prerequisite `slug` and recorded Plan `status`; it also includes the task-rollup `derived_status` when determinate. A Plan without prerequisites returns `ready: true` and an empty collection.

#### REQ: implemented-is-derived

A prerequisite satisfies readiness only if its embedded task-status rollup derives `Implemented`. A hand-authored `**Status:** Implemented` alone MUST NOT satisfy the check. Directory-form prerequisite Plans resolve with the same flat-first lookup used by Plan lifecycle commands.

#### REQ: execution-entrypoints-gated

Before writing a plan-inline task to `in_progress`, `task change-status --plan` MUST reject an unready Plan with exit `4`, naming every unmet prerequisite slug and status. `plan reconcile`, which directly records an `Implemented` Plan, MUST use the same guard and refuse before any mutation. `plan change-status` MUST NOT gate the human `In Review → Approved` arc: approval is not execution readiness.

Malformed declarations are authored-data errors owned by lint rule `P-009`; readiness MUST conservatively report them unready rather than treating a parser-recovered partial list as ready.
Reachable prerequisite cycles are likewise unready even where every direct task rollup derives `Implemented`; their unmet record has `status: invalid`, omits indeterminate `derived_status`, and includes a `reason` naming the cycle.

## Acceptance Criteria

### AC: unmet-prerequisites-are-queryable

**Given** `delivery` declares `foundation, integration`, where `foundation` is recorded `Approved` with queued work and `integration` is recorded `Executing` with in-progress work
**When** a dispatcher runs `specscore plan readiness delivery --format json`
**Then** it exits `0` with `ready: false` and both unmet slugs/statuses, in declaration order.

### AC: implemented-prerequisites-permit-execution

**Given** every prerequisite task rollup derives `Implemented`
**When** a dispatcher checks readiness or starts a plan-inline task
**Then** readiness is true and the task transition is permitted.

### AC: unmet-prerequisite-cannot-bypass-through-reconcile

**Given** a Plan with an unmet prerequisite
**When** a user runs `specscore plan reconcile <slug> --tasks=complete --note ...`
**Then** it exits `4`, names the unmet slug/status, and leaves the Plan byte-for-byte unchanged.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
