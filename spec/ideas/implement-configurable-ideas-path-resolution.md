---
format: https://specscore.md/idea-specification
status: Approved
---

# Idea: Implement configurable ideas-path resolution in specscore-cli

**Status:** Approved
**Date:** 2026-06-07
**Owner:** claude
**Promotes To:** —
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we make `specscore-cli` honor a per-module `path_overrides.ideas_path` — resolved consistently by every reader — instead of the hardcoded `spec/ideas` literal scattered across the codebase?

## Context

CLI side of the **Configurable Ideas Path** behavior, specified (Approved) in the product repo at `github.com/specscore/specscore` → `spec/features/configurable-ideas-path/README.md`. That Feature holds the full requirements + 6 ACs; the `repo-config` schema and `idea` location-rule revisions already landed there. This seed parks the `specscore-cli` implementation for this repo's own lifecycle.


Override each module's ideas dir via `path_overrides.ideas_path` in `specscore.yaml` (default `spec/ideas`, relative to module root), resolved through one contract every CLI reader uses instead of a hardcoded literal. Opt-in, non-breaking.


1. Parse/validate `module.path_overrides.ideas_path` in `pkg/config` — default `spec/ideas`; absolute/`../` = hard error naming the module; unknown keys round-trip. (AC: invalid-path-rejected)
2. Resolver: module → ideas dir (default vs override) + seeds at `<resolved>/seeds`; sole owner of the default literal. (AC: default/override/seeds-resolution)
3. Route `idea new`/`promote`, lint location checks, idea-index through the resolver; drop `spec/ideas` literals. (AC: all-readers-consistent)
4. Idea-location validation against `<resolved>/` and `<resolved>/archived/`; reject stale `spec/ideas/` when overridden. (AC: location-validation-honors-override)


`specscore migrate ideas` relocation command (separate follow-up Feature); studio URL resolution (open question); features/plans/decisions paths. Also parameterize the `idea` feature's remaining default `spec/ideas/` references (index/scaffold/scenarios) when this lands.

## Recommended Direction

Implement the behavior of the approved `configurable-ideas-path` Feature (specscore product repo) in `specscore-cli`: parse and validate `module.path_overrides.ideas_path` in `pkg/config`, add a single resolver that maps a module to its ideas directory (default `spec/ideas`, override relative to the module root, seeds at `<resolved>/seeds`), and route every reader/writer (idea `new`/`promote`, lint location checks, idea-index rendering, idea-location validation) through that resolver. The resolver is the sole owner of the `spec/ideas` default literal. Opt-in and non-breaking: with no override, behavior is identical to today.

## Alternatives Considered

- **Resolve the path independently at each call site.** Rejected: the partial-wiring split-brain risk the source Feature warns about — one reader honors the override, another keeps the literal, and the tree desyncs.
- **Reuse the repo-wide `specs_dir_name` mechanism.** Rejected: name-only and repo-wide; it cannot relocate just the ideas dir per module.

## MVP Scope

The four tasks already captured: (1) `pkg/config` parsing + validation of `path_overrides.ideas_path` (absolute/`../` rejected), (2) the resolver, (3) route all CLI readers through it, (4) resolved-path idea-location validation. Verifies the source Feature's 6 ACs.

## Not Doing (and Why)

- `specscore migrate ideas` relocation command — separate follow-up Feature
- Studio URL resolution — open question on the source Feature
- Wiring features/plans/decisions paths — only `ideas_path` is implemented

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | Every CLI reader/writer of the ideas location can be routed through one resolver. | Grep the codebase for `spec/ideas` literals; confirm each is a single resolution seam. |
| Should-be-true | `ideas_path` relative to the module root is the right semantics. | Cover a multi-module fixture (`backend/ideas`) in tests. |
| Might-be-true | Other artifact kinds will want the same override later. | Defer; only `ideas_path` now. |


## SpecScore Integration

- **New Features this would create:** none in this repo (implements the product-repo `configurable-ideas-path` Feature)
- **Existing Features affected:** the `cli` Feature (idea commands, lint location checks, config parsing)
- **Dependencies:** the approved `configurable-ideas-path` Feature (behavioral contract)

## Open Questions

None at this time.
