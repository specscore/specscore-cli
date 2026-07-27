# Plan: Plan Info (CLI)

**Status:** Implemented
**Source Feature:** cli/plan/info
**Date:** 2026-06-04
**Owner:** alexander.trakhimenok
**Supersedes:** —

## Summary

Implementation plan for the `cli/plan/info` Feature — `specscore plan info <slug>`, returning a single plan's metadata plus a task-status rollup. Builds on the `plan` command group and plan-level parsing delivered by the `cli-plan` plan.

## Approach

Two tasks, linear. Task 1 adds the task-status rollup helper to `pkg/plan` (counts tasks by their parsed `**Status:**`). Task 2 implements the `info` command output and the not-found path, consuming both the rollup and the plan-level metadata from the `cli-plan` plan. All three `info` ACs are covered; none deferred.

## Tasks

### Task 1: Task-status rollup in pkg/plan

**Verifies:** cli/plan/info#ac:info-returns-task-rollup
**Status:** complete

Add a rollup helper to `pkg/plan` that counts a plan's tasks by their parsed `**Status:**` values — `complete`, `in_progress`, `planning`, `blocked` — plus a `total`, each `0` when none. Derived from the existing per-task status parse, independent of the plan-level status. Unit-test a plan with all tasks `complete`.

### Task 2: `plan info` command — metadata, rollup, not-found

**Verifies:** cli/plan/info#ac:info-returns-metadata, cli/plan/info#ac:info-returns-task-rollup, cli/plan/info#ac:not-found-exits-3
**Status:** complete

Implement `plan info <slug>`: emit a YAML (default) / JSON / text document with `slug`, `status`, `source_feature`, `mode`, `date`, `owner`, and the `tasks` rollup from Task 1. An unresolved slug exits `3` naming the missing slug, with no partial output written to stdout.

### Task 3: Expose validated cross-plan prerequisites

**Verifies:** cli/plan/info#ac:info-returns-prerequisites
**Status:** complete

Parse the optional `**Prerequisite Plans:**` header into an ordered list and expose it as `prerequisite_plans` in YAML/JSON and a readable text line. An absent header is an empty collection (`[]` in JSON, never `null`) and reads `Prerequisite plans: none` in text. Cover populated and absent values in structured and text command tests.

## Open Questions

- Should `plan info` optionally include each task's title and status (`--fields tasks`), or should that stay a separate read?

---
*This document follows the https://specscore.md/plan-specification*
