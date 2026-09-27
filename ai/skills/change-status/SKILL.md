---
name: specscore-change-status
description: |
  Change the lifecycle status of any SpecScore artifact — Feature, Idea, or Task — via the `specscore` CLI. Use whenever an AI agent or user needs to transition, approve, deprecate, archive, or mark stable a Feature (Draft → Under Review → Approved → Implementing → Stable → Deprecated) or an Idea (Draft → Under Review → Approved → Specifying → Specified → Implementing → Implemented (or → Archived)), or to change a Task's status — complete a task, mark a task complete/blocked/failed/aborted, or move it through planning → queued → in_progress → blocked → complete (optionally stamping the implementation commit on completion). This is the canonical route for any change-status action on a Feature, Idea, or Task — prefer it over direct file edits of the `**Status:**` line.
user-invocable: true
---

# SpecScore lifecycle status transitions

This skill is the canonical route whenever you need to **change the `**Status:**` field of a SpecScore artifact**. It wraps the `specscore feature change-status`, `specscore idea change-status`, and `specscore task change-status` CLI verbs behind one entry point so the legal-transition matrix, file-relocation side effects (Idea archival), implementation-commit provenance (Task completion), and `spec lint --fix` follow-up are all handled by the CLI — never by ad-hoc file edits.

> **Why a dedicated skill?** Direct edits to a `**Status:**` line bypass the CLI's legal-transition validation and the `specscore spec lint --fix` follow-up that keeps the matching index row in sync. The CLI is the only path that gives you both at once.

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

## Pick a kind

Identify which artifact kind you are transitioning, then read the matching reference for the full invocation, legal `--to` values, exit codes, and examples.

| Kind | Reference |
|---|---|
| Feature (`spec/features/<slug>/README.md`) | [references/feature.md](references/feature.md) |
| Idea (`spec/ideas/<slug>.md` or `spec/ideas/archived/<slug>.md`) | [references/idea.md](references/idea.md) |
| Task (`tasks/<task>/` board mode, or a task block in `spec/plans/<slug>.md` via `--plan`) | [references/task.md](references/task.md) |

The Plan kind does **not** yet have a `change-status` verb on the published CLI. If the user asks to transition a Plan, stop and surface that the verb does not yet exist — do **not** fall back to a direct file edit.

## No silent fallback

If the CLI exits non-zero, the skill MUST surface the exit code and the verbatim stderr to the caller. The skill MUST NOT:

- Re-implement the transition by editing the `**Status:**` line directly.
- Re-run `specscore spec lint --fix` as a substitute for the failed verb.
- Paraphrase or summarize the CLI's stderr in a way that hides the real failure mode.

On exit `0`, relay the CLI's single-line success output (`<id>: <from-status> → <to-status>`, using Unicode `→`) byte-for-byte. See the reference files for the canonical interpretation of every non-zero exit code.

## Exit codes (shared)

`specscore feature change-status`, `specscore idea change-status`, and `specscore task change-status` share the [SpecScore CLI exit-code contract](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/README.md#shared-exit-code-contract). Per-kind specifics — including verb-level codes such as `4 InvalidTransition`, the Idea-specific archival-rollback behavior on exit `10`, and the Task-specific provenance-argument errors on exit `2` — live in [references/feature.md](references/feature.md), [references/idea.md](references/idea.md), and [references/task.md](references/task.md).
