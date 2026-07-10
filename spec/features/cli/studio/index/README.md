---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: studio index — Phase-0 ecosystem indexer + facts query

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/index?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/index?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/index?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/index?op=request-change) |
**Status:** Approved
**Source Ideas:** —
**Supersedes:** —
**Grade:** A

## Summary

`specscore studio index` builds a **rebuildable per-ecosystem fact store** (SQLite) by running four pure-function ingestion adapters — SpecScore spec trees, CodeGrapher `codegraph/` snapshots, dependency manifests, ops registries — over the repos listed in a `studio.yaml` workspace file, and **always exports the facts as per-repo INGR recordsets**. `specscore studio facts` is the minimal query verb that makes every indexed fact observable from the CLI. This is Phase 0 of the SpecScore Studio design (`specstudio-skills/spec/research/studio-design-2026-07/`): read what already exists; invent no new authoring format.

## Problem

Every fresh human or AI session working on a multi-repo ecosystem re-derives the same facts (which repo implements what, what depends on what, what status a spec claims) by grepping — measured at ~50k tokens of orientation per agent session on the Sneat dataset. The artifacts already exist per repo, distributed Go-modules-style; nothing federates them into one queryable, provenance-carrying store.

## Behavior

### Workspace

#### REQ: workspace-config

A workspace is defined by a `studio.yaml` file containing an ecosystem `name` and a list of `repos` entries (absolute or workspace-relative local directory paths; glob patterns allowed). `studio index` reads the workspace given by `--workspace <path>` (default: `./studio.yaml`). The fact store is written to `--db <path>` (default: `<workspace-dir>/.specscore-studio/facts.db`).

#### REQ: workspace-errors

A missing or unparsable workspace file, or a workspace whose `repos` resolve to zero existing directories, terminates the run with exit code 2 and a one-line actionable error. Individual unreadable/missing repo paths do NOT abort the run (see `partial-tolerance`).

### Indexing

#### REQ: rebuild-only

Every `studio index` run rebuilds the fact store from scratch: facts from previous runs never survive unless re-derived. The store is a disposable cache; the repos remain the system of record. The rebuild happens in a temporary file swapped in atomically on success, so a failed run leaves the previous store intact.

#### REQ: fact-shape

Every stored fact carries: `subject`, `predicate`, `object` (entity ref or scalar), `evidence_class` (`declared` or `derived` in this feature), `evidence_pointer` (repo-relative file path, plus a line or record locator where cheap), `adapter` (id + version), `observed_at`, and the ecosystem name. Entity references use stable IDs: repo (`host/org/name`, or an absolute-path-derived slug for local-only repos), spec (`<repo>#<path-slug>`), package/module (registry coordinates), domain (fqdn).

#### REQ: partial-tolerance

A repo or adapter failure (unreadable dir, malformed artifact file) is recorded as a per-repo warning and skipped at the smallest possible granularity (file, then adapter, then repo); all other ingestion completes. The run summary lists repos indexed, facts written per adapter, and every warning. Exit code is 0 with warnings by default; `--strict` makes any warning exit 3.

### Adapters

#### REQ: adapter-specscore

The SpecScore adapter parses `spec/` trees (repos with `specscore.yaml`): it emits entities for ideas and features (path-slug identity) and `declared` facts for: status (`has-status`), idea↔feature links (`promotes-to` / `sourced-from`), feature parent/child containment (`contains`), and acceptance-criterion existence (`has-ac`, one fact per AC id).

#### REQ: adapter-codegraph

The codegraph adapter reads committed `codegraph/` snapshots (CodeGrapher export format): it emits package entities and `derived` facts for `imports` edges at package granularity. Symbol-level entities and edges are stored only when the snapshot provides them without additional parsing passes.

#### REQ: adapter-manifests

The manifests adapter parses `go.mod` and `package.json` files (repo root plus one nested level, e.g. `frontend/`, `backend/`, `landings*/`): it emits `derived` facts for module identity (`publishes`) and direct dependencies with their version specifiers (`consumes`).

#### REQ: adapter-registries

The registries adapter parses well-known ops registry files when present at a repo root or configured per-repo in `studio.yaml` (`registries:` list): `domains.json` (Sneat-ops shape) → domain entities with `fronts`/`serves-status` facts; `ecosystem*.yaml` maps → product entities with `aliased-as`, `implemented-by`, and `member-of` facts. All facts from this adapter are `declared`.

### Query verb

#### REQ: facts-query

`specscore studio facts` filters the store by any combination of `--subject`, `--predicate`, `--object`, `--class`, `--adapter` (exact match; `--subject`/`--object` also accept a trailing `*` for prefix match) and prints a table by default or JSON with `--format json`. JSON output includes the full fact shape (`fact-shape`). `--count` prints only the row count. Querying a missing or empty store exits 2 with an actionable message naming the expected store path.

### INGR export

#### REQ: ingr-export

Every `studio index` run also writes the facts as INGR recordsets, one directory per repo, under `<workspace-dir>/.specscore-studio/ingr/<repo-slug>/` (overridable with `--ingr-dir`; `--no-ingr` disables). The record count per repo equals the number of facts attributed to that repo in the store for the same run.

## Architecture & Components

Go packages inside specscore-cli (no new binary):

- `internal/studio/workspace` — `studio.yaml` parsing + repo resolution (globs).
- `internal/studio/fact` — Fact/Entity/Evidence types and stable-ID helpers. Depends on nothing else in studio.
- `internal/studio/store` — SQLite (pure-Go driver, house precedent) schema, atomic rebuild transaction, query filters. Consumes `fact`.
- `internal/studio/adapters/{specscore,codegraph,manifests,registries}` — each implements `Adapter{ ID() string; Version() string; Ingest(repoPath string) ([]fact.Fact, []Warning) }`. Pure functions of the repo path; no store access; independently unit-testable against fixture repos.
- `internal/studio/ingr` — INGR recordset writer (reuses the encoding already used by codegraph snapshots).
- `internal/cli/studio` — the `studio` command group wiring: `index`, `facts`.

Data flow: workspace → resolved repo list → per-repo adapter fan-out (parallel, bounded) → fact slices + warnings → single atomic rebuild into SQLite → INGR export → summary. `facts` reads the store only.

## Error Handling & Failure Modes

- Workspace missing/invalid/empty-resolution → exit 2 (`workspace-errors`).
- Per-file parse failure → warning + skip file; adapter panic → recovered, warning + skip that adapter for that repo; missing repo dir → warning + skip repo (`partial-tolerance`).
- Store write failure (disk, lock) → exit 1; previous DB left intact (atomic temp-file swap, `rebuild-only`).
- INGR export failure → warning, not fatal (the store is the query surface; export is interchange).

## Testing Strategy

Unit tests per adapter against small fixture repos committed under each package's `testdata/` (one fixture per artifact kind, including a malformed file for `partial-tolerance`). End-to-end: Rehearse scenarios per AC (CLI surface — testable per the heuristic) driving `studio index` + `studio facts` over a two-repo fixture workspace. Coverage follows the repo's existing coverage gate.

## Not Doing / Out of Scope

Incremental indexing (rebuild-only is a feature, not a gap); live probes / `verified-behavior` evidence; confidence computation beyond the class field; contradiction detection; alias *resolution* in queries (aliases are stored as facts only); `studio ask`, `studio serve`, MCP; remote repo cloning; cross-repo symbol resolution beyond snapshot contents; embeddings; web UI. Each is a designed follow-on feature per the Phase-0→2 roadmap.

## Assumption Carryover (from the design research)

Survives: rebuildable-cache principle; Go-modules-style per-repo publishing; all four artifact kinds exist in the wild (validated on Sneat: 19 spec-tree repos, 11 snapshot repos, manifests everywhere, 2 registry files). Answered here: fact-store engine = SQLite (the design left SQLite vs inGitDB open — inGitDB remains the durable-store option for attestations in later features). Deferred to `studio ask`: executing the 25-question Phase-0 benchmark — this feature only guarantees the facts exist and are queryable.

## Rehearse Integration

Every AC has a CLI-observable surface; Rehearse stubs are scaffolded under `_tests/` (one per AC) with `**Status:** pending`.

## Acceptance Criteria

### AC: index-two-repos

Scenario: index a two-repo workspace
Given a `studio.yaml` naming ecosystem `demo` and listing two fixture repos, one with a `spec/` tree and one with a `codegraph/` snapshot
When I run `specscore studio index --workspace studio.yaml`
Then the command exits 0, prints a summary with both repos and per-adapter fact counts, and `<workspace-dir>/.specscore-studio/facts.db` exists

### AC: rebuild-drops-removed-repo

Scenario: rebuild-only semantics
Given a fact store previously indexed from a workspace of two repos
When I remove one repo from `studio.yaml` and run `specscore studio index` again
Then `specscore studio facts --subject <removed-repo>*` reports zero facts

### AC: workspace-missing-error

Scenario: index without a workspace file
Given a directory containing no `studio.yaml`
When I run `specscore studio index`
Then the command exits 2 and prints a one-line error naming the expected workspace path

### AC: spec-status-fact

Scenario: SpecScore adapter emits status with the full fact shape
Given a fixture repo whose `spec/features/x/README.md` has `**Status:** Approved`, indexed in ecosystem `demo`
When I run `specscore studio index` and then `specscore studio facts --predicate has-status --format json`
Then the JSON contains a fact with subject ending `#x`, object `Approved`, evidence_class `declared`, an evidence_pointer to that README path, adapter id `specscore` with a non-empty version, a non-empty `observed_at`, and ecosystem `demo`

### AC: codegraph-import-fact

Scenario: codegraph adapter emits import edge
Given a fixture repo with a committed `codegraph/` snapshot containing a package-import edge from `a` to `b`
When I run `specscore studio index` and then `specscore studio facts --predicate imports`
Then the output contains one row with subject package `a`, object package `b`, evidence_class `derived`

### AC: manifest-consumes-fact

Scenario: manifests adapter emits dependency
Given a fixture repo whose `go.mod` requires `example.com/m v1.2.3`
When I run `specscore studio index` and then `specscore studio facts --predicate consumes --format json`
Then the JSON contains a fact whose object is `example.com/m@v1.2.3` with evidence_pointer `go.mod`

### AC: registry-domain-fact

Scenario: registries adapter emits domain
Given a fixture repo with a `domains.json` mapping `example.app` to a live status
When I run `specscore studio index` and then `specscore studio facts --predicate fronts`
Then the output contains a fact whose subject is domain `example.app`

### AC: partial-tolerance-warns

Scenario: one broken repo does not abort the run
Given a workspace listing one healthy fixture repo and one path that does not exist
When I run `specscore studio index`
Then the command exits 0, the summary lists a warning for the missing path, and facts from the healthy repo are queryable

### AC: strict-mode-fails

Scenario: --strict escalates warnings
Given the same workspace with one missing repo path
When I run `specscore studio index --strict`
Then the command exits 3 and the warning is printed

### AC: ingr-export-counts

Scenario: INGR export matches store
Given an indexed two-repo workspace
When I compare each repo's INGR record count under `<workspace-dir>/.specscore-studio/ingr/` with that repo's fact count in the index summary
Then the counts are equal for every repo

### AC: missing-store-error

Scenario: facts without an index
Given a workspace directory where `studio index` has never run
When I run `specscore studio facts --predicate imports`
Then the command exits 2 with a message naming the expected store path and suggesting `studio index`

## Open Questions

- Repo-slug scheme for local-only paths (no remote): basename with a collision counter, or short hash suffix? (Implementation detail; decide in plan.)
- Should `facts` support a `--repo <name>` convenience filter over subject prefixes in v1? (Nice-to-have; not required by the ACs.)

---
*This document follows the https://specscore.md/feature-specification*
