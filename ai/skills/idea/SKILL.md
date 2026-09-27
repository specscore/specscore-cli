---
name: specscore-idea
description: |
  List, inspect, scaffold, and transition SpecScore Idea artifacts at `spec/ideas/<slug>.md`. Use whenever you need to list open ideas, check an idea's status, filter ideas by lifecycle stage, create a new pre-spec one-pager, approve a Draft Idea, or archive an Idea that has been superseded or abandoned.
user-invocable: true
---

# SpecScore Idea scaffolding and lifecycle

This skill wraps the `specscore idea` command family — creation and lifecycle surface for SpecScore Idea artifacts.

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
| List all ideas (optionally filtered by status) | [references/list.md](references/list.md) |
| Scaffold a new Idea artifact at `spec/ideas/<slug>.md` | [references/new.md](references/new.md) |
| Transition an Idea's lifecycle status (approve, archive) | [references/change-status.md](references/change-status.md) |

## What an Idea is

An Idea is a **pre-spec, lintable one-pager** that captures a problem, a recommended direction, an MVP scope, and the assumptions that must hold for the direction to be worth pursuing. Ideas are the optional front-door to SpecScore: they refine a vague concept into something concrete enough to promote into one or more [Features](https://github.com/specscore/specscore-cli/blob/main/spec/features/cli/feature/README.md).

For the **methodology** of refining ideas (divergent / convergent thinking, stress-testing assumptions), use the [`specstudio:ideate`](https://github.com/specscore/specstudio-skills) skill. This skill is the *artifact-creation* surface only.

## Exit codes (shared)

`specscore idea` commands share the [SpecScore CLI exit-code contract](https://github.com/specscore/specscore-cli/blob/main/spec/features/cli/README.md#shared-exit-code-contract). Per-verb specifics in [references/new.md](references/new.md) and [references/change-status.md](references/change-status.md).
