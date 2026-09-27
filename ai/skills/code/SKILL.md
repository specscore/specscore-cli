---
name: specscore-code
description: |
  Query source-code → SpecScore relationships. Reads `specscore:` annotations and bare `https://specscore.md/...` URLs from comments and reports the features, plans, or docs source files depend on. Use for impact analysis or code-to-spec traceability.
user-invocable: true
---

# SpecScore code-to-spec navigation

This skill wraps the `specscore code` command family — read-only queries over the source-code → spec edge. Where [`feature deps`](../feature/references/deps.md) walks the spec → spec graph, `code deps` walks the code → spec graph.

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
| List the SpecScore resources that source files depend on | [references/deps.md](references/deps.md) |

## Scope of this group

Commands under `specscore code` are **read-only**. They never mutate source files, the spec tree, or any project state.

## Exit codes (shared)

`specscore code` commands share the [SpecScore CLI exit-code contract](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/README.md#shared-exit-code-contract). Per-verb specifics in [references/deps.md](references/deps.md).
