---
format: https://specscore.md/feature-specification
status: Implementing
---

# Feature: Task (CLI)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/task?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/task?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/task?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/task?op=request-change) |
>
> **AI skill:** [GitHub](https://github.com/specscore/ai-plugin-specscore/blob/main/skills/task/SKILL.md) · [local](../../../../../ai-plugin-specscore/skills/task/SKILL.md) — if this command's CLI signature or behavior changes, update the linked skill to keep agents in sync.

**Status:** Implementing
**Source Ideas:** —

## Summary

`specscore task` commands read and create entries on the project task board. The MVP surface covers listing, inspecting, and creating tasks. **Coordination-bearing** status transitions (claim, release, contention-resolved updates) remain out of scope for this group; the single exception is the single-actor [`change-status`](change-status/README.md) verb, which performs a pure status rewrite (plus optional implementation-commit provenance) with no claim/release semantics.

## Problem

Task boards live as Markdown (`tasks/README.md` plus `tasks/<slug>/README.md` per entry) so they are diff-able and reviewable. Without command-line read and create surfaces, every consumer re-parses the Markdown, and every agent writes the files by hand. A minimal CLI — list, info, new — unlocks automation while keeping the human-readable format as the source of truth.

## Contents

| Directory | Description |
|---|---|
| [info/](info/README.md) | Show detailed task metadata for one slug |
| [list/](list/README.md) | List tasks from the board, optionally filtered by status |
| [new/](new/README.md) | Create a new task in `planning` status |
| [change-status](change-status/README.md) | Transition a Task's status and optionally record implementation-commit provenance. |
| [amend](change-status/README.md#req-annotation-corrective-amendment) | Correct singleton Task annotations without changing lifecycle state. |

### info

Reads `tasks/<slug>/README.md` and the task's row on the board. Returns YAML or JSON with title, status, description, dependencies, and summary.

### list

Reads the `tasks/README.md` board. Returns all rows or those matching `--status`. YAML is the default format; `--format md` re-emits the board table unchanged for round-tripping.

### new

Writes a new `tasks/<slug>/README.md` and appends the row to the board. New tasks are always created with status `planning`.
## Behavior

### Scope of this group

Today the group covers read/seed operations plus the explicitly scoped local `change-status` and annotation-amendment surfaces. Distributed transition semantics (who can claim, sync policy, and multi-agent terminal agreement) warrant their own orchestrator-owned feature specs.

#### REQ: no-lifecycle-in-mvp

No subcommand in this group may mutate an existing task's status field **except** the single-actor [`change-status`](change-status/README.md) verb (see [cli/task/change-status#req:single-actor-task-lifecycle-permitted](change-status/README.md#req-single-actor-task-lifecycle-permitted)). `new` only creates tasks in `planning`; it MUST NOT accept a `--status` argument for other values. Coordination-bearing lifecycle commands (e.g., `task claim`, `task release`) remain out of scope and orchestrator-owned; they would land under new subcommands with their own feature specs.

### Task slug argument

Every non-listing command takes a task slug via `--task <slug>` (a flag, not positional, matching the current implementation).

#### REQ: task-flag-required

`task info` and `task new` MUST require `--task`. A missing slug MUST exit `2` (InvalidArgs).

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [task](../../task/README.md) | Source of truth for task file structure, board format, and allowed status values. |
| [CLI](../README.md) | Inherits shared exit-code contract, `--project`, `--format`. |
| [`task/` skill](https://github.com/specscore/ai-plugin-specscore/blob/main/skills/README.md#planned-cli-wrapper-catalogue) (ai-plugin-specscore) | Agent-side wrapper for `task list`, `task info`, and `task new`. Treats this feature spec as the authoritative contract. |

## Open Questions

- When should lifecycle commands (`task claim`, `task release`, `task status`) land? They depend on answering how concurrency and multi-agent claim semantics work in a git-backed board — which is a feature spec in its own right.
- Should `--task` move from a flag to a positional argument (`specscore task info <slug>`) for consistency with `feature info <id>`? Today the two groups diverge on this point.

---
*This document follows the https://specscore.md/feature-specification*
