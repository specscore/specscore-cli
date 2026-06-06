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

Decomposes the [cli/spec/migrate](../features/cli/spec/migrate/README.md) Feature — the Task 7 finale of the artifact-frontmatter-convention work. It builds the one-shot `specscore spec migrate` backfill command, then runs it across this repo and flips the three graced frontmatter rules from `warning` to `error`.

## Approach

Two tasks in dependency order. Task 1 builds and unit-tests the deterministic, offline backfill engine + `spec migrate` verb in isolation (it does not yet touch the real spec tree). Task 2 is the cutover: it runs the command against this repo to backfill every artifact, then flips `format-field` / `status-mirror` / `footer-format-mirror` to `error` severity and verifies the migrated tree lints clean at the new default — landed as one commit so the repo is never left half-enforced. The deferred create-verb retrofits (decision/task) and cross-repo migration are out of scope here (tracked separately).

## Tasks

### Task 1: Implement the `specscore spec migrate` backfill command

**Verifies:** cli/spec/migrate#ac:backfills-format-and-status, cli/spec/migrate#ac:status-less-types-excluded, cli/spec/migrate#ac:footer-aligned-to-format, cli/spec/migrate#ac:migration-idempotent

**Depends-On:** none

Add the `spec migrate` verb and a deterministic, offline backfill engine that walks the convention's document/index types and, per artifact, inserts a leading frontmatter block with the type-derived `format:` URL, adds `status:` mirrored from the body `**Status:**` for status-bearing types (and never for status-less types), and aligns the footer URL to `format:`. Re-running on a conformant artifact is a byte-level no-op. Reuse the existing type registry (`docTypeTargets`), `bodyAfterFrontmatter`, and the status-mirror helpers; cover with table-driven tests to the 100% gate.

### Task 2: Run the migration and flip the graced rules to error

**Verifies:** cli/spec/migrate#ac:rules-enforce-after-cutover

**Depends-On:** 1

Run `specscore spec migrate` against this repo to backfill every existing artifact (feature READMEs, idea files, `*-index` READMEs, directory-form plan READMEs), then flip `format-field`, `status-mirror`, and `footer-format-mirror` from `warning` to `error` in the rules registry and checkers. Verify the migrated tree passes `specscore spec lint` at the post-cutover `error` severity, and that removing an artifact's frontmatter is then flagged as an error. Land as a single commit so the repo is never left migrated-but-unenforced or enforced-but-unmigrated.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
