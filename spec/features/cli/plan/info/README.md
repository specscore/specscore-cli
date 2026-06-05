---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Plan Info

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan/info?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan/info?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan/info?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan/info?op=request-change) |
**Status:** Approved
**Date:** 2026-06-04
**Owner:** alexander.trakhimenok
**Source Ideas:** specscore-cli-should-expose-a-plan-verb-with-list-and-query
**Supersedes:** —

## Summary

`specscore plan info <slug>` returns structured metadata for a single plan: its status, source feature, mode, date, owner, and a rollup of its task statuses (e.g., 6 of 8 tasks done).

## Synopsis

```
specscore plan info <slug> [--format <yaml|json|text>] [--project <path>]
```

## Problem

Tools and agents stepping through plans — a "what's the state of this plan?" check, a status dashboard, a pre-implementation gate — need a stable, machine-readable snapshot of a plan's metadata and progress. Parsing `README`/plan Markdown on every query re-implements the `pkg/plan` parser and is fragile. A single command that returns everything a consumer needs, in a known schema, removes that burden — and the task rollup turns the already-parsed task statuses into an at-a-glance progress signal.

## Behavior

### Output shape

Output is a YAML (default) / JSON / text document describing one plan.

#### REQ: required-fields

The output MUST include, at minimum:

- `slug` — the plan slug (filename without `.md`)
- `status` — the plan's `**Status:**` value, read verbatim (empty when the line is absent)
- `source_feature` — the `**Source Feature:**` value (empty when absent)
- `mode` — the `**Mode:**` value (empty when absent)
- `date` — the `**Date:**` value (empty when absent)
- `owner` — the `**Owner:**` value (empty when absent)
- `tasks` — a rollup object (see REQ:task-rollup)

Additional fields MAY be added in later releases; consumers MUST tolerate unknown fields.

### Task rollup

A plan's tasks each carry a `**Status:**` drawn from `pending`, `in-progress`, `done`, `blocked`. The rollup summarizes them.

#### REQ: task-rollup

The `tasks` field MUST include `total` (count of tasks in the plan) and a per-status breakdown covering `done`, `in-progress`, `pending`, and `blocked` (each `0` when none). The counts MUST be derived from the parsed task `**Status:**` values, not from the plan-level status.

### Format selection

#### REQ: format-and-not-found

`--format` MUST accept `yaml` (default), `json`, or `text`; text output is a condensed human-readable rendering, not for parsing. An unresolved `<slug>` MUST exit `3` (NotFound) with a message naming the requested slug, per the [cli/plan](../README.md) shared contract.

## Parameters

| Name | Required | Description |
|---|---|---|
| `slug` | Yes | Plan to inspect — its filename without `.md` (e.g., `cli-rules`). |

## Exit codes

| Code | Condition |
|---|---|
| `0` | Plan found and info printed |
| `2` | Missing `slug` argument, invalid `--format` value |
| `3` | Plan not found |
| `10` | Unexpected I/O failure while reading the plan |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [cli/plan](../README.md) | Inherits the shared `--format`/`--project` contract, slug resolution, and the not-found exit code. |
| [cli/feature/info](../../feature/info/README.md) | Sibling whose single-artifact metadata contract this command mirrors. |

## Acceptance Criteria

### AC: info-returns-metadata (verifies REQ:required-fields)

**Given** a plan `cli-rules` with `**Status:** Completed` and `**Source Feature:** cli/rules`
**When** the user runs `specscore plan info cli-rules`
**Then** the YAML output contains `slug: cli-rules`, `status: Completed`, and `source_feature: cli/rules`.

### AC: info-returns-task-rollup (verifies REQ:task-rollup)

**Given** a plan with 8 tasks of which 8 are `done`
**When** the user runs `specscore plan info <slug>`
**Then** the output's `tasks` field reports `total: 8` and `done: 8`, with `in-progress`, `pending`, and `blocked` each `0`.

### AC: not-found-exits-3 (verifies REQ:format-and-not-found)

**Given** a project with no plan named `does-not-exist`
**When** the user runs `specscore plan info does-not-exist`
**Then** the command exits `3` with a stderr message naming the missing slug, and no partial output is written to stdout.

## Open Questions

- Should `plan info` optionally include each task's title and status (`--fields tasks`) for consumers that want the full task list, or should that stay a separate read?

---
*This document follows the https://specscore.md/feature-specification*
