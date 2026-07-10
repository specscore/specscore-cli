---
format: https://specscore.md/feature-specification
status: Implementing
---

# Feature: rehearse run — execute markdown scenarios (bash + Hurl http blocks)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/run?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/run?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/run?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/run?op=request-change) |
**Status:** Implementing
**Source Ideas:** —
**Supersedes:** —
**Grade:** A

## Summary

`specscore rehearse run <paths...>` executes markdown acceptance scenarios: files with `Verifies:` AC identity in body metadata and fenced executable/declarative step blocks. v0.3 block kinds: `bash` (exec), `hurl` (HTTP — embedded Hurl syntax delegated to the hurl binary), `sql` (query + row asserts against a DSN), `dtql` (dalgo DTQL query against a SQLite store), `graphql` (query + JSONPath asserts, compiled onto the Hurl engine). Per-scenario/per-AC pass-fail reporting (human table + JSON), non-zero exit on failure. Folds the Rehearse runner into specscore-cli per `rehearse` repo `spec/ideas/rehearse-evidence-layer.md` (v0.3), so it rides the existing distribution channels. Success gate: the 11 committed Studio scenarios (`spec/features/cli/studio/index/_tests/`) run green through this runner.

## Problem

Acceptance criteria have no deterministic proof layer: the 11 Studio scenarios pass but only via hand-run bash; there is no discovery, no per-AC reporting, no declarative HTTP/SQL/query checks, and the standalone rehearse prototype is dormant with zero distribution. Agent-authored verification needs a paved, deterministic road whose results can later feed SpecScore Studio's verified-behavior evidence tier.

## Behavior

### Discovery & scenario shape

#### REQ: scenario-discovery

`specscore rehearse run <path...>` accepts files, directories, and globs. A directory is scanned recursively for `*.md` scenario files, excluding `README.md`. With no path argument inside a SpecScore repo, it defaults to all `spec/features/**/_tests/`. Standalone mode: explicit paths work in any directory — no `specscore.yaml` required.

#### REQ: scenario-shape

A scenario file may carry `**Verifies:**` body metadata — a comma-separated list of AC ids, each optionally followed by a parenthetical annotation which is stripped when parsing (the committed corpus writes `<ac-id> (REQ: <slug>)`) and `**Status:**`. Its executable content is the ordered sequence of fenced step blocks (kinds below). Steps run in order in one scenario-scoped temp working dir; the first failing step fails the scenario; remaining steps are skipped-after-failure. A scenario with zero step blocks is reported `no-steps` (neither pass nor fail).

### Step blocks

#### REQ: bash-block

A ```` ```bash ```` block runs via `bash -euo pipefail`; non-zero exit fails the step. Stdout/stderr are captured into the report (truncated with a note beyond 8 KiB per step).

#### REQ: hurl-block

A ```` ```hurl ```` block contains verbatim Hurl syntax and is executed by delegating to the `hurl` binary (`hurl --test`). The runner does NOT implement an HTTP client. When `hurl` is not on PATH, the runner detects this in an upfront scan: any scenario containing a hurl-derived block is reported `skipped` (none of its steps execute, including earlier bash steps) with a warning naming the missing binary; skips do not affect the exit code.

#### REQ: sql-block

A ```` ```sql dsn=<driver:path-or-dsn> ```` block executes its statement(s) against the DSN (v0.3 drivers: `sqlite:<path>` via the pure-Go driver already in go.mod). Assertions are declared as trailing directive comments inside the block: `-- assert-rows: <N>` (result row count) and/or `-- assert-row-json: {...}` (first row as JSON object equality). A failed assertion or query error fails the step.

#### REQ: dtql-block

A ```` ```dtql db=<sqlite-path> ```` block contains a DTQL query document (dalgo's `dtql` package deserializes it) executed against the SQLite store at `db=` via the dalgo SQLite adapter. Same `-- assert-rows:` / `-- assert-row-json:` directives as `sql`. This makes a Studio fact store (`facts.db`) directly assertable by scenarios.

#### REQ: graphql-block

A ```` ```graphql url=<endpoint> ```` block contains a GraphQL query (optionally with a `-- variables: {...}` directive line carrying the variables JSON object, consistent with the other directive syntax). It is compiled to a Hurl POST request (JSON body `{query, variables}`) with `HTTP 200` asserted plus any trailing `-- assert-jsonpath: <path> == <json-value>` directives, and executed via the hurl delegation of `hurl-block` (including its missing-binary skip semantics).

### Context bag (cross-block state)

#### REQ: context-bag

Each scenario run owns a **context bag** — an ordered map of string variables scoped to the scenario. Variable *consumption* is per block class: for `bash`/`sql`/`dtql` blocks, `{{name}}` placeholders in the block body and info-string params are textually interpolated before execution (unknown variable → step fail naming it). For **hurl-derived blocks** (`hurl`, `graphql`) there is NO textual pre-interpolation — Hurl owns the `{{name}}` syntax natively, so the bag is passed as `--variable name=value` flags and Hurl resolves references itself (its own error reporting covers unknowns); this keeps every valid multi-request hurl block (capturing and reusing its own variables) verbatim-valid. Captures into the bag: `bash` via appending `name=value` lines to the file at `$REHEARSE_CAPTURES`; `hurl` via Hurl's native `[Captures]`, exported from the run's JSON report and merged after the step; `sql`/`dtql` via `-- capture: <name> = <column>` (first row's column value); `graphql` via `-- capture-jsonpath: <name> = <path>`. The JSON report includes the bag's final state per scenario.

### Reporting

#### REQ: run-report

The human report is one line per scenario — status (`pass`/`fail`/`skipped`/`no-steps`), file path, its `Verifies:` AC ids, duration — plus a totals line. `--format json` emits an array of `{file, status, verifies[], duration_ms, bag:{}, steps:[{kind, status, detail}]}`. Exit code: 0 when no scenario failed; 1 when any failed; 2 on usage/config errors — including when discovery matches zero scenario files (an empty run is a config error, not a pass).

### Corpus gate

#### REQ: studio-corpus-green

The 11 committed Studio scenarios under `spec/features/cli/studio/index/_tests/` run green via `specscore rehearse run` (they are bash-block scenarios; any edits to them are limited to what discovery/shape compliance requires), and repo CI runs them.

## Architecture & Components

- `internal/rehearse/scenario` — file parsing: body metadata (`Verifies:`/`Status:`), fenced-block extraction with info-string params. No execution.
- `internal/rehearse/blocks/{bash,hurl,sqlblock,dtqlblock,graphql}` — one executor per kind implementing `Block{ Kind() string; Run(ctx StepCtx) StepResult }`; graphql composes onto hurl; dtql/sql share the assert-directive parser.
- `internal/rehearse/runner` — discovery, ordered execution, the context bag (consumption per block class, capture merge after each step), report model, JSON/human rendering.
- `internal/cli/rehearse.go` — `rehearse` command group: `run` (flags: `--format`, paths...). Reuses house exit-code conventions.
- Reuse: existing pure-Go sqlite driver (go.mod), `github.com/dal-go/dalgo` + its `dtql` package + SQLite adapter for dtql-block; NO new HTTP client (hurl delegation).

## Error Handling & Failure Modes

Missing path → exit 2. Unparsable scenario file → reported `fail` with parse detail (not a run abort). Missing hurl binary → per-scenario `skipped` + warning. DSN/driver unsupported → step fail with actionable message. Panic in a block executor → recovered, step fail.

## Testing Strategy

Unit tests per package with fixture scenarios in `testdata/` (incl. failing, no-steps, unparsable, missing-hurl via PATH manipulation, sqlite fixture DB, DTQL against a fixture store, graphql against `httptest`). E2e: Rehearse scenarios for this feature's own ACs (self-hosting — the runner runs its own acceptance scenarios). Coverage gate 100% holds.

## Not Doing / Out of Scope

Evidence emission into `studio index` (v0.4); `rehearse new <ac-id>` scaffolding (v0.5); python/starlark blocks; own HTTP client; browser automation; parallel scenario execution; non-sqlite SQL drivers.

## Open Questions

- Standalone thin binary from the same packages: deferred decision per the Idea (v0.4, demand-driven).

## Acceptance Criteria

### AC: corpus-green

Scenario: the Studio corpus passes
Given the specscore-cli repo checkout
When I run `specscore rehearse run spec/features/cli/studio/index/_tests --format json`
Then the command exits 0 and the JSON lists 11 scenarios all with status `pass` and non-empty `verifies` arrays

### AC: failing-scenario-fails-run

Scenario: a failing bash step fails the run
Given a scenario file whose bash block exits non-zero
When I run `specscore rehearse run <that-file>`
Then the command exits 1 and the report marks that scenario `fail`

### AC: standalone-run

Scenario: standalone mode without a spec repo
Given a scenario file with a passing bash block in a directory containing no `specscore.yaml`
When I run `specscore rehearse run <that-file>`
Then the command exits 0 and reports the scenario `pass`

### AC: json-report-shape

Scenario: JSON report carries the contract fields
Given any single passing scenario
When I run `specscore rehearse run <file> --format json`
Then the output is valid JSON whose first element has `file`, `status`, `verifies`, `duration_ms`, and a non-empty `steps` array with `kind` and `status`

### AC: hurl-pass

Scenario: hurl block executes against a live server
Given `hurl` on PATH and a scenario whose bash step starts a local HTTP server and whose hurl block asserts `HTTP 200` from it
When I run `specscore rehearse run <that-file>`
Then the command exits 0 and the scenario is `pass`

### AC: hurl-missing-skips

Scenario: missing hurl binary skips, not fails
Given a PATH without `hurl` and a scenario containing a hurl block
When I run `specscore rehearse run <that-file>`
Then the command exits 0, the scenario is reported `skipped`, and the warning names the `hurl` binary

### AC: sql-assert-rows

Scenario: sql block asserts a row count
Given a sqlite fixture database with 3 rows in table `t` and a scenario whose sql block queries `t` with `-- assert-rows: 3`
When I run `specscore rehearse run <that-file>`
Then the command exits 0 and the scenario is `pass`

### AC: dtql-counts-facts

Scenario: dtql block queries a fact store
Given a SQLite fact store produced by `specscore studio index` on a fixture workspace and a scenario whose dtql block selects facts with `-- assert-rows:` equal to the store's known fact count
When I run `specscore rehearse run <that-file>`
Then the command exits 0 and the scenario is `pass`

### AC: context-bag-chains

Scenario: a captured value flows into a later block
Given a scenario whose bash block writes `uid=42` to `$REHEARSE_CAPTURES`, followed by an sql block against a sqlite fixture querying `... WHERE id = {{uid}}` with `-- assert-rows: 1` and `-- capture: name = username`, followed by a bash block asserting `[ "{{name}}" = "alice" ]`
When I run `specscore rehearse run <that-file>`
Then the command exits 0, the scenario is `pass`, and the JSON report's final bag contains `uid` and `name`

### AC: graphql-jsonpath

Scenario: graphql block asserts a response field
Given a local `httptest`-style GraphQL stub server returning `{"data":{"ok":true}}` and `hurl` on PATH, and a scenario whose graphql block posts a query with `-- assert-jsonpath: $.data.ok == true`
When I run `specscore rehearse run <that-file>`
Then the command exits 0 and the scenario is `pass`

## Open Questions (resolved into plan)

None beyond the standalone-binary deferral above.


---
*This document follows the https://specscore.md/feature-specification*
