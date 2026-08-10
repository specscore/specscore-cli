---
name: specscore-lessons
description: Record, query, import, and enforce SpecScore Lessons without leaking prompts or losing historical evidence.
---

# SpecScore Lessons

Use canonical `spec/lessons/<slug>/README.md` directories. Record a compact durable rule in the README and one immutable JSON observation per `occurrences/<uuid-v4>.json`. Never commit original prompts, raw logs, diffs, credentials, personal contact data, or absolute worktree paths.

## Create and record

Run `specscore lesson new <slug>` only after `specscore init` and a non-empty `lessons.classifications` in `specscore.yaml`; a failed preflight must leave the tree unchanged. Record a bounded occurrence with `specscore lesson occurrence add <slug> --summary "short factual statement" --context-json '<safe JSON>'`. Context is restricted to repository, git, worktree, and execution facts. Use `occurrence list` and `occurrence info` to read it. `lesson recur` is the compatible alias; `lesson check` is the opt-in recurrence gate.

## Import, relationships, and delivery

Always start legacy work with `lesson import-legacy --source <log> --dry-run --format json`. Repeated historical IDs are distinct evidence keys, never automatic duplicates. Apply only a reviewed mapping that explicitly chooses `new`, `occurrence`, or `manual` for every key. Use `lesson relation add` only with its preview `--confirm` token; do not infer duplicates from title similarity.

Lesson mutations queue a durable event per subscriber. If a sink fails, use `specscore event replay --subscriber <stable-name>`; a successful subscriber does not acknowledge another's pending delivery.

## Migration

Only `specscore spec lint --fix` renames `## Outstanding Questions` to `## Open Questions`. Never rely on a scaffold command to make unrelated migrations.
