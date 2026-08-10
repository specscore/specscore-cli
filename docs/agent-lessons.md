# Lessons for AI coding agents

Use canonical directory Lessons. The compact rule lives at `spec/lessons/<slug>/README.md`; append observations as immutable JSON under `occurrences/`. Do not put prompts, raw logs, diffs, credentials, or absolute worktree paths in either artifact.

## Create and record

`specscore lesson new <slug>` is deliberately configuration-gated. First run `specscore init` and configure a non-empty `lessons.classifications` vocabulary in `specscore.yaml`. On a failed preflight it writes nothing.

Use `specscore lesson occurrence add <slug> --summary "short factual fact" --context-json '<safe object>'`. Safe context is limited to generic `repository`, `git`, `worktree`, and `execution` values; `worktree.path_hint` must be relative or `redacted`. Inspect with `occurrence list` and `occurrence info`. Existing `lesson recur <slug> --note ...` remains a compatibility alias. Use `lesson check --not-enforced --min-recurred N` only after its CI baseline is reviewed.

## Import and deduplicate without losing history

Run `lesson import-legacy --source LESSONS-LEARNED.md --dry-run --format json` first. It is write-free and assigns distinct source keys even when numeric IDs repeat. Review the generated inventory and make a mapping where every source key explicitly chooses `new`, `occurrence`, or `manual`. `--apply --mapping mapping.json` verifies the source digest, preserves original bytes and provenance, never overwrites, and is idempotent.

Use `lesson relation add <from> <to> --type related|duplicates|supersedes` without guessing semantic duplication. Copy the exact preview token to `--confirm`; there is intentionally no `--yes`. `duplicates` is evidence of a human decision, not an auto-merge.

## Migration

Only `specscore spec lint --fix` changes the obsolete heading `## Outstanding Questions` to `## Open Questions`. Scaffold and create commands never perform that repository-wide rewrite as a side effect.
