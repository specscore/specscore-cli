---
format: https://specscore.md/plan-specification
status: Draft
---

# Plan: Artifact Frontmatter Migration

**Status:** Draft
**Source Feature:** cli/spec/migrate
**Date:** 2026-06-05
**Owner:** alexandertrakhimenok
**Supersedes:** —

## Summary

Decomposes the [cli/spec/migrate](../features/cli/spec/migrate/README.md) Feature — the Task 7 finale of the artifact-frontmatter-convention work. It builds the one-shot `specscore migrate` backfill command, keeps `specscore spec migrate` as an equivalent nested form, then runs it across this repo and flips the three graced frontmatter rules from `warning` to `error`.

## Approach

Two tasks in dependency order. Task 1 builds and unit-tests the deterministic, offline backfill engine + root-visible `migrate` verb in isolation (it does not yet touch the real spec tree), while preserving `spec migrate` as an equivalent nested form. Task 2 is the cutover: it runs the command against this repo to backfill every artifact, then flips `format-field` / `status-mirror` / `footer-format-mirror` to `error` severity and verifies the migrated tree lints clean at the new default — landed as one commit so the repo is never left half-enforced. The deferred create-verb retrofits (decision/task) and cross-repo migration are out of scope here (tracked separately).

## Tasks

### Task 1: Implement the `specscore migrate` backfill command

**Verifies:** cli/spec/migrate#ac:backfills-format-and-status, cli/spec/migrate#ac:status-less-types-excluded, cli/spec/migrate#ac:footer-aligned-to-format, cli/spec/migrate#ac:migration-idempotent, cli/spec/migrate#ac:root-alias-discoverable

**Depends-On:** none

Add the root-visible `migrate` verb, keep `spec migrate` equivalent, and implement a deterministic, offline backfill engine that walks the convention's document/index types and, per artifact, inserts a leading frontmatter block with the type-derived `format:` URL, adds `status:` mirrored from the body `**Status:**` for status-bearing types (and never for status-less types), and aligns the footer URL to `format:`. Re-running on a conformant artifact is a byte-level no-op. Reuse the existing type registry (`docTypeTargets`), `bodyAfterFrontmatter`, and the status-mirror helpers; cover with table-driven tests to the 100% gate.

### Task 2: Run the migration and flip the graced rules to error

**Verifies:** cli/spec/migrate#ac:rules-enforce-after-cutover

**Depends-On:** 1

Run `specscore migrate` against this repo to backfill every existing artifact (feature READMEs, idea files, `*-index` READMEs, directory-form plan READMEs), then flip `format-field`, `status-mirror`, and `footer-format-mirror` from `warning` to `error` in the rules registry and checkers. Verify the migrated tree passes `specscore spec lint` at the post-cutover `error` severity, and that removing an artifact's frontmatter is then flagged as an error. Land as a single commit so the repo is never left migrated-but-unenforced or enforced-but-unmigrated.

### Task 3: Backfill the flat task board's body Status line

**Verifies:** cli/spec/migrate#ac:existing-board-task-gains-status-line, cli/spec/migrate#ac:task-board-status-backfill-never-invents-a-value

**Depends-On:** 1

Closes a 2026-08-27 gap: `task change-status` requires an existing body `**Status:**` line and has no path that initializes an absent one, so any task board created before `task new` scaffolded that line (every board that existed before this task) is permanently unable to transition through the sanctioned CLI. Add `migrateTaskBoardStatus` (`pkg/lint/task_board_migrate.go`), invoked from `MigrateWithProjectRoot`: for the flat project-root `tasks/` board (outside `spec/`, so not covered by the existing `docTypeTargets` walk), insert a `**Status:**` line into any `tasks/<slug>/README.md` that lacks one, sourced from that slug's `tasks/README.md` row — a one-time bootstrap, never overwriting an existing line and never inventing a value for a slug with no board row. Companion changes landed in the same effort (tracked by their own Features, not this plan): `cli/task/new#req:status-scaffolded` scaffolds the line on every new task, and a new `task-index-row-sync` lint rule reconciles the board's Status cell from the file after a transition — the file remains authoritative per the `specscore` meta-spec's Index feature `REQ: file-authoritative-over-index`.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
