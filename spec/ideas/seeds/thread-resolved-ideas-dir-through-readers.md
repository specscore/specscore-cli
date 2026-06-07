---
captured_by: claude
status: queued
---
# Thread the resolved ideas-dir through idea readers (configurable-ideas-path rewiring)

Remaining half of `configurable-ideas-path` (foundation — the `projectdef` resolver — already landed). The readers in `pkg/idea` and `pkg/lint` derive the ideas dir as `spec/ + "ideas"`, which breaks when `path_overrides.ideas_path` moves ideas out of `spec/` (e.g. project-root `/ideas`). This is **atomic** per the Feature's `single-resolver` AC — partial wiring makes lint flag correctly-placed ideas as misplaced, so all readers must switch together.

## Seam list (replace `spec/ + "ideas"` derivation with the resolved dir)

- `internal/cli/idea.go` — `idea new` target (`filepath.Join(specRoot, "spec", "ideas")`); load config, use `ModuleConfig.EffectiveIdeasPath`.
- `internal/cli/idea_promote.go` — promote target/scan paths.
- `pkg/idea/discover.go` — `Discover` + `FindIdeaDirectories` (`specRoot` here means the `spec/` dir — note the two different `specRoot` meanings).
- `pkg/lint/idea.go` — `idea-location` rule, `findMisplacedIdeaFiles`, archived-location.
- `pkg/lint/idea_index.go` — active/archived index path computation.
- `pkg/idea/transitions.go` — archived-dir path.
- `pkg/sidekick/scaffold.go` — seeds dir (use `EffectiveSeedsPath`).

## Approach

Introduce a single resolved-ideas-dir value (absolute or project-relative) threaded into these functions instead of deriving from `spec/`. Update each function's existing test suite. Verifies the Feature's `all-readers-consistent` + `location-validation-honors-override` ACs.
