# Plan: Plan (CLI) — command group

**Status:** Implemented
**Source Feature:** cli/plan
**Date:** 2026-06-04
**Owner:** alexander.trakhimenok
**Supersedes:** —

## Summary

Implementation plan for the parent `cli/plan` Feature — the shared read-only command group: the `plan` cobra command, its shared flags, plan-slug resolution, exit-code contract, and the plan-level `**Status:**` parsing every subcommand reads. The `list` and `info` subcommands are planned separately (`cli-plan-list`, `cli-plan-info`), each building on this group.

## Approach

Two tasks, linear. Task 1 extends `pkg/plan` with the plan-level metadata fields (the parsing substrate the whole group and both subcommand plans depend on). Task 2 registers the `plan` cobra group with shared flags, slug resolution, and the not-found / invalid-format exit codes. All four parent ACs are covered; none deferred.

## Tasks

### Task 1: Plan-level metadata parsing in pkg/plan

**Verifies:** cli/plan#ac:missing-status-is-empty
**Status:** complete

Extend the `pkg/plan.Plan` struct and parser to capture the plan-level `**Status:**`, `**Date:**`, and `**Owner:**` body-metadata fields (`**Mode:**` and `**Source Feature:**` already exist), each defaulting to empty when its line is absent. Cover the missing/blank plan-level status with a unit test so it reports empty rather than failing. This is the shared substrate the `list` and `info` plans build on.

### Task 2: `plan` cobra group — shared flags, slug resolution, exit codes

**Verifies:** cli/plan#ac:group-exposes-subcommands, cli/plan#ac:invalid-format-rejected, cli/plan#ac:unknown-slug-exits-3
**Status:** complete

Register the `plan` parent command and wire it into the root command tree, mirroring `cli/feature`. Provide the shared `--format yaml|json|text` and `--project` flags, print group help with exit `0` when invoked with no subcommand, reject an invalid `--format` value with exit `2` naming the value, and resolve a `<slug>` argument to `spec/plans/<slug>.md` with exit `3` (naming the slug) when it does not exist.

## Open Questions

- Should the recognized `--fields` set (used by `list`) live in a shared registry alongside `cli/feature`'s field names, or stay defined per-command?

---
*This document follows the https://specscore.md/plan-specification*
