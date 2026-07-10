---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: rehearse evidence — verified-behavior facts into studio index

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/evidence?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/evidence?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/evidence?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/evidence?op=request-change) |
**Status:** Approved
**Source Ideas:** —
**Supersedes:** —
**Grade:** A

## Summary

Rehearse results become the top rung of the Studio evidence ladder. `specscore rehearse run` gains `--report-out <path>` persisting the JSON run report (with run provenance) to a conventional per-repo location, and a fifth studio ingestion adapter (`rehearse`) reads persisted reports during `specscore studio index`, emitting one `verified-by` and one `has-verification-status` fact per scenario–AC pair with the new `verified-behavior` evidence class. `specscore studio facts --class verified-behavior` makes the proof queryable. Implements Rehearse v0.4 per the governing Idea (`rehearse` repo, `spec/ideas/rehearse-evidence-layer.md`): *"Studio `facts --class verified-behavior` returns real rows for cli/studio/index ACs."*

## Problem

Rehearse v0.3 proves acceptance criteria deterministically (`specscore rehearse run`, 21-scenario self-hosting corpus green in CI), but the proof evaporates when the process exits: the JSON report goes to stdout and nowhere else. Studio's fact store — built to answer "what is verified?" — only knows `declared` and `derived` evidence; its top confidence tier (`verified-behavior`, per the Studio evidence-ladder design) has no producer. Nothing connects "this AC passed at commit X" to the queryable ecosystem index, so freshness, contradiction detection (spec says Stable, latest run says fail), and per-AC evidence chips have no data to stand on.

## Behavior

### Report persistence

#### REQ: report-out

`specscore rehearse run` gains a `--report-out <path>` flag that persists the run report as a JSON file at `<path>`, creating parent directories as needed. The conventional per-repo location consumed by the studio adapter is `<repo-root>/.specscore/rehearse/latest.json`; the flag accepts any path (standalone mode keeps working anywhere). The report file is written whether scenarios pass or fail — a failing run is evidence too — and exit-code semantics are unchanged. The stdout report (`--format human|json`) is unchanged; the persisted file uses the envelope shape of `report-provenance`. Only the latest report is kept per location: the file is overwritten in place (run history is a later, Studio-hosted concern).

#### REQ: report-provenance

The persisted report is an envelope object: `{"runner_version", "git_sha", "git_dirty", "started_at", "scenarios": [...]}` where `scenarios` is exactly the existing JSON report array (`{file, status, verifies, duration_ms, bag, steps}` per `cli/rehearse/run#req:run-report`, with `file` a repo-root-relative path when the run is inside a git work tree), `runner_version` is the specscore CLI version, `started_at` is the run's start time (UTC, RFC 3339), and `git_sha`/`git_dirty` are the working tree's `HEAD` commit and dirty flag. Outside a git work tree, `git_sha` is empty and `git_dirty` is false — provenance is honest, never invented.

### Studio ingestion

#### REQ: adapter-rehearse

A fifth studio ingestion adapter with id `rehearse` (package `internal/studio/adapters/rehearse`, registered in the adapter registry's `All()` list) reads `<repo>/.specscore/rehearse/latest.json` during `specscore studio index`. A missing report file emits no facts and no warning (most repos have no Rehearse runs — absence is normal). An unreadable or unparsable report file emits a warning and skips the file, per `cli/studio/index#req:partial-tolerance`; all other adapters' ingestion is unaffected. `studio index` never executes scenarios itself — indexing time stays decoupled from test time.

#### REQ: verification-facts

For every scenario in the report whose status is `pass` or `fail`, and for every AC id in its `verifies` list, the adapter emits two facts: `(<ac-ref>, verified-by, <scenario-file>)` and `(<ac-ref>, has-verification-status, pass|fail)`. `<ac-ref>` is the repo-scoped AC reference `<repo-slug>#<verifies-entry>` (e.g. `specscore-cli#cli/studio/index#ac:index-two-repos`), joining the `has-ac` facts' subject scheme. `<scenario-file>` is the scenario's repo-relative path from the report. Both facts carry `evidence_class: verified-behavior` and `evidence_pointer: .specscore/rehearse/latest.json`. Scenarios with status `skipped` or `no-steps` emit no facts — nothing executed, so there is no behavioral evidence. A scenario with an empty `verifies` list emits no facts (there is no AC to attach evidence to).

#### REQ: observed-at-run-time

Facts emitted by the `rehearse` adapter carry `observed_at` equal to the report's `started_at` — the time the behavior was actually observed — not the index run's timestamp. Staleness must be honest: re-indexing an old report must not refresh its facts' apparent age. (The adapter pipeline stamps its shared `observed_at` only onto facts whose adapter left the field empty.)

### Evidence class and query surface

#### REQ: verified-behavior-class

The studio fact model gains a third evidence class, `verified-behavior` — behavior observed by executing the system, the top rung of the evidence ladder above `declared` and `derived`. This feature amends `cli/studio/index#req:fact-shape`'s "declared or derived in this feature" enumeration: the class set is now `declared | derived | verified-behavior`. `specscore studio facts --class verified-behavior` filters to these facts through the existing exact-match `--class` flag; the flag's help text names all three classes.

### Self-hosting gate

#### REQ: self-hosting-gate

The v0.4 success gate, self-hosted: after `specscore rehearse run spec/features/cli/studio/index/_tests --report-out .specscore/rehearse/latest.json` in this repo and `specscore studio index` over a workspace that includes this repo, `specscore studio facts --class verified-behavior` returns real rows for `cli/studio/index` ACs. Repo CI runs this end-to-end (extending the existing `Rehearse corpus` job).

## Architecture & Components

- `internal/rehearse/runner` — report envelope type (`RunReport`: runner_version, git_sha, git_dirty, started_at, scenarios) + provenance collection (git via a test-seam `exec` wrapper, house pattern) + `WriteReport(path)` persistence. Scenario `file` paths are emitted repo-relative when inside a git work tree.
- `internal/cli/rehearse.go` — the `--report-out` flag wiring; stdout rendering unchanged.
- `internal/studio/adapters/rehearse` — the new adapter: report discovery at the conventional path, envelope parsing, fact emission per `verification-facts` (pure function of the repo path; no store access; fixture-tested like its four siblings).
- `internal/studio/adapters/adapters.go` — one-line registry append; `Run` stamps `observed_at` only when the adapter left it empty (all existing adapters do, so their behavior is unchanged).
- `internal/studio/fact` — `VerifiedBehavior Class = "verified-behavior"` constant.
- `internal/cli/studio.go` — `--class` help text names the third class.
- `.gitignore` — `.specscore/rehearse/` (reports are local run artifacts, not committed).
- CI (`.github/workflows/go-ci.yml`) — the `Rehearse corpus` job adds `--report-out`, a minimal `studio.yaml`, `studio index`, and a non-empty `facts --class verified-behavior` assertion (`self-hosting-gate`).

Data flow: `rehearse run` → report envelope → `--report-out` file → (later, decoupled) `studio index` → `rehearse` adapter → `verified-behavior` facts in the store → `studio facts --class verified-behavior`.

## Error Handling & Failure Modes

- `--report-out` path unwritable → run exits 2 (config error) after printing the stdout report; scenario results are still shown.
- Report file missing at index time → no facts, no warning (`adapter-rehearse`).
- Report file malformed → adapter warning + skip; exit 0 (or 3 under `--strict`), other adapters unaffected.
- `git` absent or not a work tree at run time → empty `git_sha`, `git_dirty` false; report still written.
- Report from a different repo layout (scenario paths that don't resolve) → facts still emitted verbatim from the report; the report is the artifact of record.

## Testing Strategy

Unit tests per package with fixtures: runner envelope/provenance (git seam stubbed), adapter against fixture repos with pass/fail/skipped/malformed/missing reports, fact-class constant, CLI flag wiring. E2e: Rehearse scenarios per AC under `_tests/` (all ACs have a CLI or file surface — see `## Rehearse Integration`), including the self-hosting gate scenario. Coverage gate stays 100% (`scripts/coverage-gate.sh`).

## Not Doing / Out of Scope

- **`file` assert block** — originally roadmapped for v0.4; parked to v0.5 (sql/dtql/captures were pulled forward into v0.3; keeping this release evidence-focused).
- **Standalone `rehearse` binary** — decided per the Idea's Open Question, from demand: skip. `specscore rehearse` with no spec tree already IS standalone mode; no separate binary or module revival.
- **Report history/retention beyond `latest.json`** — run history is a Studio-hosted concern (later phase).
- **Freshness dots, contradiction items, `studio ask` citations** — Studio-side consumers of these facts (post-v0.4 roadmap).
- **Probe-style indexing** (`studio index` running scenarios) — rejected by design: couples indexing time to test time.
- **CI-artifact report ingestion / remote reports** — local conventional path only in v0.4.

## Rehearse Integration

All ACs have CLI/file surfaces; stub scenarios are scaffolded under `_tests/` (one per AC, `**Status:** pending`).

## Acceptance Criteria

### AC: report-out-writes-envelope

Scenario: --report-out persists a provenance envelope
Given a scenario file with a passing bash block inside a git work tree with a clean HEAD
When I run `specscore rehearse run <file> --report-out out/report.json`
Then the command exits 0 and `out/report.json` is valid JSON with a non-empty `runner_version`, a non-empty `git_sha`, `git_dirty` false, a non-empty RFC 3339 `started_at`, and a `scenarios` array whose first element has `file`, `status` `pass`, `verifies`, `duration_ms`, and `steps`

### AC: report-out-on-failure

Scenario: a failing run still persists the report
Given a scenario file whose bash block exits non-zero
When I run `specscore rehearse run <file> --report-out out/report.json`
Then the command exits 1 and `out/report.json` exists with that scenario's status `fail`

### AC: report-out-outside-git

Scenario: provenance is honest outside a git work tree
Given a passing scenario file in a directory that is not inside any git work tree
When I run `specscore rehearse run <file> --report-out out/report.json`
Then the command exits 0 and the report's `git_sha` is empty and `git_dirty` is false

### AC: adapter-emits-verified-by

Scenario: studio index ingests a persisted report
Given a fixture repo whose `.specscore/rehearse/latest.json` reports scenario `spec/features/x/_tests/s.md` with status `pass` verifying `x#ac:y`, indexed in ecosystem `demo`
When I run `specscore studio index` and then `specscore studio facts --class verified-behavior --format json`
Then the JSON contains a fact with subject `<repo-slug>#x#ac:y`, predicate `verified-by`, object `spec/features/x/_tests/s.md`, evidence_class `verified-behavior`, evidence_pointer `.specscore/rehearse/latest.json`, and adapter id `rehearse` with a non-empty version

### AC: fail-status-fact

Scenario: failures are evidence too
Given a fixture repo whose report lists a scenario with status `fail` verifying `x#ac:y`
When I run `specscore studio index` and then `specscore studio facts --predicate has-verification-status --format json`
Then the JSON contains a fact with subject `<repo-slug>#x#ac:y` and object `fail`

### AC: observed-at-run-time

Scenario: fact age is the run time, not the index time
Given a fixture repo whose report's `started_at` is `2026-01-02T03:04:05Z`
When I run `specscore studio index` and then `specscore studio facts --class verified-behavior --format json`
Then every rehearse-adapter fact's `observed_at` is `2026-01-02T03:04:05Z` while facts from other adapters carry the index run's timestamp

### AC: skipped-scenario-no-facts

Scenario: skipped scenarios emit no evidence
Given a fixture repo whose report lists one scenario with status `skipped` and one with status `no-steps`, both with non-empty `verifies`
When I run `specscore studio index` and then `specscore studio facts --class verified-behavior --count`
Then the count is 0

### AC: malformed-report-warns

Scenario: a malformed report degrades gracefully
Given a fixture repo whose `.specscore/rehearse/latest.json` is not valid JSON and whose `go.mod` requires a module
When I run `specscore studio index`
Then the command exits 0, the summary lists a warning naming the report file for adapter `rehearse`, and the manifests adapter's facts are queryable

### AC: missing-report-silent

Scenario: absence of a report is normal
Given a fixture repo with no `.specscore/rehearse/` directory
When I run `specscore studio index`
Then the command exits 0 with no warning from adapter `rehearse` and zero `verified-behavior` facts

### AC: self-hosting-gate

Scenario: the v0.4 success gate on this repo
Given this repo after `specscore rehearse run spec/features/cli/studio/index/_tests --report-out .specscore/rehearse/latest.json` and a `studio.yaml` listing this repo
When I run `specscore studio index` and then `specscore studio facts --class verified-behavior --format json`
Then the command exits 0 and the JSON contains at least one fact whose subject contains `cli/studio/index#ac:`

## Open Questions

None — the standalone-binary question the Idea deferred to v0.4 is decided under `## Not Doing`.

## Autonomous Decisions

- **Feature slug `cli/rehearse/evidence`** (alternative: `cli/studio/index/rehearse-adapter`) — the feature spans both the emission (rehearse) and ingestion (studio) ends; the handout lists this slug first and the Idea names the v0.4 milestone "evidence emission".
- **New class declared in this feature** (alternative: revise `cli/studio/index`'s fact-shape REQ in place) — the handout marks this the cleaner option; the amendment is recorded prose-level in `verified-behavior-class`.
- **Persisted file uses an envelope; stdout `--format json` stays a bare array** — preserves `cli/rehearse/run#ac:json-report-shape` verbatim while giving the persisted artifact provenance.
- **Skipped/no-steps scenarios emit no facts** (alternative: a `skipped` verification status) — nothing executed, so claiming behavioral evidence would be dishonest; skip visibility stays in the run report.
- **`file` assert block parked to v0.5; standalone binary skipped** — per the handout's recommendations, recorded under `## Not Doing`.

---
*This document follows the https://specscore.md/feature-specification*
