---
captured_by: claude
status: queued
---
# Implement configurable ideas-path resolution in specscore-cli

CLI side of the **Configurable Ideas Path** behavior, specified (Approved) in the product repo at `github.com/specscore/specscore` → `spec/features/configurable-ideas-path/README.md`. That Feature holds the full requirements + 6 ACs; the `repo-config` schema and `idea` location-rule revisions already landed there. This seed parks the `specscore-cli` implementation for this repo's own lifecycle.

## What

Override each module's ideas dir via `path_overrides.ideas_path` in `specscore.yaml` (default `spec/ideas`, relative to module root), resolved through one contract every CLI reader uses instead of a hardcoded literal. Opt-in, non-breaking.

## Tasks (1 → 2 → {3,4})

1. Parse/validate `module.path_overrides.ideas_path` in `pkg/config` — default `spec/ideas`; absolute/`../` = hard error naming the module; unknown keys round-trip. (AC: invalid-path-rejected)
2. Resolver: module → ideas dir (default vs override) + seeds at `<resolved>/seeds`; sole owner of the default literal. (AC: default/override/seeds-resolution)
3. Route `idea new`/`promote`, lint location checks, idea-index through the resolver; drop `spec/ideas` literals. (AC: all-readers-consistent)
4. Idea-location validation against `<resolved>/` and `<resolved>/archived/`; reject stale `spec/ideas/` when overridden. (AC: location-validation-honors-override)

## Deferred

`specscore migrate ideas` relocation command (separate follow-up Feature); studio URL resolution (open question); features/plans/decisions paths. Also parameterize the `idea` feature's remaining default `spec/ideas/` references (index/scaffold/scenarios) when this lands.
