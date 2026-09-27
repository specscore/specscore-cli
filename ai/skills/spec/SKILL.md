---
name: specscore-spec
description: |
  Validate a SpecScore specification tree against structural conventions. Use whenever you've edited specs, before creating commits, or when CI needs to gate on spec hygiene.
user-invocable: true
---

# SpecScore specification validation

This skill wraps the `specscore spec` command family — tree-wide validation of a SpecScore project's specification tree.

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
| Lint the spec tree against structural conventions | [references/lint.md](references/lint.md) |

## Scope of this group

`specscore spec` operates on the project's spec tree as a whole — not on individual features. Per-feature queries live in the [`feature/`](../feature/SKILL.md) skill.

## Exit codes (shared)

`specscore spec` commands share the [SpecScore CLI exit-code contract](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/README.md#shared-exit-code-contract). Per-verb specifics in each reference. Note: `spec lint` uniquely uses exit `1` to mean "violations found" — see the [reference](references/lint.md#exit-codes).
