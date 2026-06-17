# Plan: Lint Rule Catalog

**Status:** Implemented
**Mode:** full
**Source Feature:** cli/rules
**Date:** 2026-06-03
**Owner:** alexander.trakhimenok
**Supersedes:** —

## Summary

Implementation plan for the `cli/rules` Feature — a structured lint-rule registry as the single source of truth, a `specscore rules` discovery command, a generated `docs/lint-rules.md` catalog, and a `specscore rules --check` drift-guard wired into CI. Eight tasks covering all nine ACs in the source Feature, zero deferred.

## Approach

Foundation-first linear decomposition. The structured registry (Task 1) is the substrate every other task derives from, so it lands first; the parity check (Task 2) hardens it immediately. The discovery command (Tasks 3–4) and the catalog renderer (Task 5) both depend only on the registry and are built next as independent readers. The write path (Task 6) consumes the renderer to materialize `docs/lint-rules.md`; the drift-guard (Task 7) composes the renderer and the committed file; CI wiring (Task 8) invokes the guard. `**Depends-On:**` lines encode the dependency edges so `specstudio:implement` can batch correctly even though the task numbers are linear. No ACs are deferred — all nine ACs in `cli/rules` are covered.

## Tasks

### Task 1: Structured rule registry as single source of truth

**Status:** done
**Depends-On:** —
**Verifies:** cli/rules#ac:registry-has-metadata

Replace the bare `allRuleNames map[string]bool` in `pkg/lint` with a structured registry where each rule is an entry carrying a non-empty `id`, a non-empty one-line `description`, a `family`, and a `severity`. Populate descriptions for all currently-registered rules across families. Make an empty description impossible to register (construction-time guard or a validating test).

### Task 2: Registry ↔ checker parity validation

**Status:** done
**Depends-On:** 1
**Verifies:** cli/rules#ac:registry-parity-enforced

Add a reusable parity check that asserts every rule ID a checker can emit has a registry entry and every registry entry corresponds to an emittable rule. Report offending IDs (orphan entries and unregistered emissions) and fail. Cover it with a test.

### Task 3: `specscore rules` command — deterministic listing

**Status:** done
**Depends-On:** 1
**Verifies:** cli/rules#ac:rules-lists-all

Add the `rules` cobra command that prints every registered rule with its `id`, `family`, and `description` in a deterministic order and exits `0`. Wire it into the root command.

### Task 4: Family filter and output format

**Status:** done
**Depends-On:** 3
**Verifies:** cli/rules#ac:rules-filter-and-json

Add `--family <name>` to restrict output to one family and `--format text|json` (text default). JSON output emits one object per rule carrying the same fields as the registry entry.

### Task 5: Deterministic catalog renderer at the canonical path

**Status:** done
**Depends-On:** 1
**Verifies:** cli/rules#ac:catalog-deterministic-render, cli/rules#ac:catalog-at-canonical-path

Implement a renderer that produces the markdown catalog from the registry deterministically — stable ordering, grouped by family, each rule showing `id` and `description`, no timestamps or environment-dependent content. Target path is `docs/lint-rules.md`. Re-rendering an unchanged registry yields byte-identical output.

### Task 6: `--write` generates `docs/lint-rules.md`

**Status:** done
**Depends-On:** 5
**Verifies:** cli/rules#ac:catalog-write-regenerates

Wire `specscore rules --write` to render the catalog and write it to `docs/lint-rules.md`, and generate the initial committed catalog file. Re-running after a registry change rewrites the file to match.

### Task 7: `--check` drift-guard

**Status:** done
**Depends-On:** 5, 6
**Verifies:** cli/rules#ac:check-detects-drift

Add `specscore rules --check`: render the catalog in-memory and compare it to the committed `docs/lint-rules.md`. Exit non-zero and report the divergence when they differ (including when the file is missing); exit `0` writing nothing when they match.

### Task 8: Wire `specscore rules --check` into CI

**Status:** done
**Depends-On:** 7
**Verifies:** cli/rules#ac:ci-fails-on-stale-catalog

Add a CI step that runs `specscore rules --check`, so adding, removing, or renaming a rule, or changing a description, without regenerating the catalog fails the build.

## Open Questions

- None at this time.

---
*This document follows the https://specscore.md/plan-specification*
