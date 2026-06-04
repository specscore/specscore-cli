# Plan: Idea Promote (CLI) — internal/cli idea promote

**Status:** Approved
**Mode:** full
**Source Feature:** cli/idea/promote
**Date:** 2026-06-04
**Owner:** alexander.trakhimenok
**Supersedes:** —

## Summary

Implementation plan for the `cli/idea/promote` Feature — the `specscore idea promote <slug>` verb that turns a sidekick seed into a lint-clean Idea. Six tasks covering all nine ACs, zero deferred: command scaffold + guards, the same-repo move+transform, back-link discovery/classification + same-repo reconcile, the cross-repo archive branch, verdict carry-forward, and the stdout/format surface.

## Approach

Foundation → branches → presentation. Task 1 lands the command, argument/flag parsing, and the three pre-mutation guards (resolution, collision, clean-tree) so every later task runs behind a validated entry point. Task 2 implements the common same-repo happy path (move + transform + lint-fix), reusing the `idea new` skeleton writer. Task 3 adds back-link discovery and the same/cross-repo classifier (sharing `idea relocate`'s sibling-repo reference scan) plus the same-repo reconcile. Task 4 adds the cross-repo archive branch on top of that classifier. Tasks 5 (verdict carry-forward) and 6 (stdout/format) are independent of the move/archive branch and depend only on the transform from task 2, so they sit off the 1→2→3→4 critical path via `Depends-On`. No ACs are deferred.

## Tasks

### Task 1: Scaffold `idea promote` command + pre-mutation guards

**Status:** done
**Depends-On:** —
**Verifies:** cli/idea/promote#ac:seed-not-found, cli/idea/promote#ac:collision-without-force, cli/idea/promote#ac:dirty-tree-rejected

Add the `promote` cobra subcommand under `idea` with `<slug>`, `--force`, `--verdict`, and `--project`. Resolve the seed to `spec/ideas/seeds/<slug>.md` (exit `3` if absent), refuse when `spec/ideas/<slug>.md` exists without `--force` (exit `1`), validate the `--verdict` enum (exit `2`), and run the clean-tree pre-flight over the paths to be modified (exit `7`). No move or transform yet — all guards return before mutation.

### Task 2: Same-repo move + seed→Idea transform

**Status:** done
**Depends-On:** 1
**Verifies:** cli/idea/promote#ac:same-repo-promote-happy-path

Implement the same-repo path: `git mv` the seed to `spec/ideas/<slug>.md`, then transform in place — swap seed frontmatter for Idea body-metadata, retitle `# Idea: <title>`, fold seed prose into `## Context`, and insert HTML-comment prompts for unfilled sections (reusing the `idea new` skeleton writer). Run lint-fix so the result is lint-clean and history is preserved via rename.

### Task 3: Back-link discovery, classification, and same-repo reconcile

**Status:** pending
**Depends-On:** 2
**Verifies:** cli/idea/promote#ac:same-repo-backlinks-reconciled

Discover `## Sidekick Seeds Generated` entries referencing the seed (reusing `idea relocate`'s sibling-repo reference scan), classify each as same-repo (bare relative path) or cross-repo (`<repo-slug>:` prefix), and rewrite same-repo entries from the old `seeds/<slug>.md` target to the new `spec/ideas/<slug>.md`. This classifier is the branch selector consumed by Task 4.

### Task 4: Cross-repo archive branch

**Status:** pending
**Depends-On:** 3
**Verifies:** cli/idea/promote#ac:cross-repo-archive, cli/idea/promote#ac:never-deprecated

When the classifier finds any cross-repo reference, take the no-move-of-content branch: create the Idea at `spec/ideas/<slug>.md` by copy+transform, `git mv` the seed to `spec/ideas/archived/<slug>.md` with frontmatter `status: promoted` and `promoted_to: <slug>`, run lint-fix, and never set `deprecated`. The sibling repo's back-link is left untouched (delegated to lint/UI).

### Task 5: Verdict carry-forward

**Status:** pending
**Depends-On:** 2
**Verifies:** cli/idea/promote#ac:verdict-carry-forward-modes

Implement the three carry-forward modes — `pointer` (default, single line), `full` (copy the `## Consilium Verdict` section), `drop` — selected by `specscore.yaml promote.verdict_carry_forward` with a `--verdict` flag override (flag wins). Omit the pointer when the seed carries no verdict. Reuse the existing `specscore.yaml` loader pattern.

### Task 6: stdout summary + structured output

**Status:** pending
**Depends-On:** 2
**Verifies:** cli/idea/promote#ac:stdout-summary

Emit the deterministic success summary — created Idea path, the seed's fate (`moved` or `archived`), and each reconciled back-link — and a `--format json|yaml` structured form mirroring `idea relocate`'s `stdout-format` shape.

## Open Questions

- Cross-repo back-link reconciliation after the seed is archived is delegated to lint/UI cross-repo reference resolution (tracked in the source Feature's Open Questions); Task 4 deliberately does not touch sibling repos.
- The exact `--format json|yaml` field schema (Task 6) is to be aligned with `idea relocate`'s structured output during implementation.

---
*This document follows the https://specscore.md/plan-specification*
