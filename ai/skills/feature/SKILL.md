---
name: specscore-feature
description: |
  Navigate and manage SpecScore feature specifications — list features, render hierarchy trees, inspect metadata, trace dependencies, find reverse references, scaffold new features, transition feature lifecycle status. Use whenever you need to understand or change the spec graph. Use when asked to list features, check feature status, show what's implemented or in progress, or query the specification tree.
user-invocable: true
---

# SpecScore feature navigation

This skill wraps the `specscore feature` command family — the structured query surface over a project's feature tree. Pick the verb that matches your current need and read the corresponding reference for full instructions, parameters, and exit codes.

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
| List features in the project (flat view) | [references/list.md](references/list.md) |
| Render the feature hierarchy as a tree | [references/tree.md](references/tree.md) |
| Inspect one feature's metadata, sections, and children | [references/info.md](references/info.md) |
| Trace what a feature depends on | [references/deps.md](references/deps.md) |
| Find what depends on (references) a feature | [references/refs.md](references/refs.md) |
| Scaffold a new feature directory | [references/new.md](references/new.md) |
| Transition a feature's lifecycle status (Draft → Under Review → Approved → Implementing → Stable → Deprecated) | [references/change-status.md](references/change-status.md) |

## Exit codes (shared)

All `specscore feature` commands share the [SpecScore CLI exit-code contract](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/README.md#shared-exit-code-contract). Per-verb specifics in each reference.
