---
name: specscore-plan
description: |
  Read and scaffold a SpecScore project's plans — list plans, inspect a single plan's metadata and task-status rollup, and scaffold a new lint-clean Plan from a Source Feature, a Source Idea, or no source at all (source-less). Use whenever you need to read plans, check a plan's status, or create a new plan artifact. Status transitions are out of scope for this MVP.
user-invocable: true
---

# SpecScore plan management

This skill wraps the `specscore plan` command family — the read-and-scaffold surface over a project's plans (`spec/plans/<slug>.md`). Pick the verb that matches your current need.

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
| List plans (optionally filtered by status, with selected metadata fields) | [references/list.md](references/list.md) |
| Inspect one plan's metadata and task-status rollup | [references/info.md](references/info.md) |
| Scaffold a new lint-clean plan from a Source Feature, a Source Idea, or no source (source-less) | [references/new.md](references/new.md) |

## Scope of this group

This wrapper covers **read and scaffold only.** `list` and `info` are read-only; `new` creates a lint-clean plan skeleton. Plan lifecycle transitions (status changes such as `Approved`, `Executing`, `Implemented`) are not part of this skill's surface — the `plan` execution-status band is derived by `specscore spec lint --fix` from the plan's task rollup, and prep/disposition statuses are authored directly in the artifact.

## Exit codes (shared)

All `specscore plan` commands share the [SpecScore CLI exit-code contract](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/README.md#shared-exit-code-contract). Per-verb specifics in each reference.
