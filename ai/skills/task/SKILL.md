---
name: specscore-task
description: |
  Read and seed a SpecScore project's task board — list tasks, inspect a single task, create a new task in `planning`. Use whenever you need to read the board, check task status, list open or in-progress tasks, or add a fresh entry. Status transitions are out of scope for this MVP.
user-invocable: true
---

# SpecScore task management

This skill wraps the `specscore task` command family — the read-and-seed surface over a project's task board (`tasks/README.md` plus `tasks/<slug>/README.md` per entry). Pick the verb that matches your current need.

## Pre-flight check

Before running any `specscore` command, verify the CLI is installed:

```bash
command -v specscore >/dev/null 2>&1
```

If this check fails (exit `127` / `command not found`), stop and tell the user exactly:

> The `specscore` CLI is not installed. Either:
> - invoke `/specscore:install` to see install options, or
> - install directly from <https://specscore.md/install>.
>
> Then retry your command.

Do not proceed with the original command until `specscore --version` succeeds.

## Pick a verb

| You need to… | Read |
|---|---|
| List tasks (optionally filtered by status) | [references/list.md](references/list.md) |
| Inspect one task's metadata, dependencies, and summary | [references/info.md](references/info.md) |
| Create a new task (always in `planning` status) | [references/new.md](references/new.md) |

## Scope of this group

This wrapper covers **read and seed only.** Lifecycle transitions (claim, release, status change) are not part of the SpecScore CLI's MVP surface and therefore not part of this skill — see the parent group's [scope rule](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/task/README.md#req-no-lifecycle-in-mvp). For multi-agent task coordination with a full lifecycle, see the [`synchestra-cli`](https://github.com/synchestra-io/ai-plugin-synchestra) plugin's `task/` skill.

## Exit codes (shared)

All `specscore task` commands share the [SpecScore CLI exit-code contract](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/README.md#shared-exit-code-contract). Per-verb specifics in each reference.
