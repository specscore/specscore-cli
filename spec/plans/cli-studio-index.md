---
format: https://specscore.md/plan-specification
status: Approved
---
# Plan: studio index — Phase-0 ecosystem indexer + facts query

**Status:** Executing
**Source Feature:** cli/studio/index
**Date:** 2026-07-10
**Owner:** alex
**Supersedes:** —

## Summary

Implements the `specscore studio` command group's Phase-0 surface: the `index` verb (studio.yaml workspace → four ingestion adapters → atomic-rebuild SQLite fact store → per-repo INGR export) and the `facts` query verb. All new code lives in `internal/studio/*` and `internal/cli/studio`; no existing subsystems change.

## Approach

Foundations before consumers: workspace loading and the command skeleton first, then the fact model + store + query verb (giving every later task an observable CLI surface), then the four adapters in independence order (specscore first — richest fixtures; manifests next, proving rebuild semantics end-to-end; codegraph and registries after), then cross-cutting failure tolerance, and finally the INGR exporter with the full two-repo end-to-end. Each task authors its AC's Rehearse scenario(s) as executable Bash alongside unit tests and lands within the repo's coverage gate. Commits carry `Verifies:` trailers for their AC IDs.

## Tasks

### Task 1: Workspace loading + `studio index` skeleton

**Verifies:** cli/studio/index#ac:workspace-missing-error
**Depends-On:** —
**Status:** complete

`internal/studio/workspace` (studio.yaml parsing, glob resolution, workspace-relative defaults) and the `studio` command group with an `index` verb that validates the workspace and prints a run summary (no adapters yet). Exit-2 paths for missing/unparsable/zero-resolving workspaces.

### Task 2: Fact model, SQLite store, `studio facts` verb

**Verifies:** cli/studio/index#ac:missing-store-error
**Depends-On:** 1
**Status:** in_progress

`internal/studio/fact` (Fact/Evidence types, stable-ID helpers), `internal/studio/store` (schema, atomic temp-file-swap rebuild, filter queries), and the `facts` verb with table/JSON/`--count` output plus the exit-2 missing-store path. Resolves the Feature's open questions as implementation details: local repo-slug = basename with `-N` collision counter; no `--repo` sugar in v1.

### Task 3: SpecScore adapter

**Verifies:** cli/studio/index#ac:spec-status-fact
**Depends-On:** 2
**Status:** planning

`internal/studio/adapters/specscore`: parse spec trees into idea/feature entities and declared facts (has-status, promotes-to/sourced-from, contains, has-ac), reusing the CLI's existing spec-parsing packages where exported. Fixture repo in testdata; the AC asserts the full fact shape end-to-end.

### Task 4: Manifests adapter + rebuild semantics

**Verifies:** cli/studio/index#ac:manifest-consumes-fact, cli/studio/index#ac:rebuild-drops-removed-repo
**Depends-On:** 2
**Status:** planning

`internal/studio/adapters/manifests`: go.mod + package.json at root and one nested level → publishes/consumes facts. With adapters live, prove rebuild-only semantics end-to-end (facts from a repo removed from the workspace disappear on re-index).

### Task 5: Codegraph adapter

**Verifies:** cli/studio/index#ac:codegraph-import-fact
**Depends-On:** 2
**Status:** planning

`internal/studio/adapters/codegraph`: read committed codegraph/ snapshot recordsets, emit package entities and derived imports edges; symbol-level pass-through only where the snapshot already provides it.

### Task 6: Registries adapter

**Verifies:** cli/studio/index#ac:registry-domain-fact
**Depends-On:** 2
**Status:** planning

`internal/studio/adapters/registries`: domains.json (Sneat-ops shape) → domain entities + fronts/serves-status facts; ecosystem*.yaml maps → product entities + aliased-as/implemented-by/member-of facts. Per-repo `registries:` configuration in studio.yaml.

### Task 7: Partial tolerance + `--strict`

**Verifies:** cli/studio/index#ac:partial-tolerance-warns, cli/studio/index#ac:strict-mode-fails
**Depends-On:** 3, 4, 5, 6
**Status:** planning

Warning collection at file→adapter→repo granularity across all adapters, adapter-panic recovery, per-repo summary lines, exit 0-with-warnings default and exit 3 under `--strict`.

### Task 8: INGR export + two-repo end-to-end

**Verifies:** cli/studio/index#ac:ingr-export-counts, cli/studio/index#ac:index-two-repos
**Depends-On:** 7
**Status:** planning

`internal/studio/ingr` recordset writer (reusing the codegraph snapshot encoding), `--ingr-dir`/`--no-ingr` flags, per-repo record-count parity with the store, the full two-repo fixture workspace end-to-end scenario, and command docs (README section for `studio index`/`facts`).

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
