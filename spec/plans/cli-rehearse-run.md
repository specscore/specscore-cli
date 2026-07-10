---
format: https://specscore.md/plan-specification
status: Approved
---
# Plan: rehearse run — v0.3 acceptance-evidence runner

**Status:** Approved
**Source Feature:** cli/rehearse/run
**Date:** 2026-07-10
**Owner:** alex
**Supersedes:** —

## Summary

Implements `specscore rehearse run`: scenario parsing/discovery, five step-block executors (bash, hurl-delegated HTTP, sql, dtql, graphql-on-hurl), the per-block-class context bag, human/JSON reporting, and the Studio-corpus CI gate. All new code in `internal/rehearse/*` + `internal/cli/rehearse.go`.

## Approach

Runner core with the bash executor first (gives every later task a runnable surface), then reporting, then the sql/dtql pair (shared directive parser + dalgo/sqlite), then the context bag across the non-hurl classes, then the hurl-derived pair (delegation, upfront-scan skip, --variable passing, captures merge), and finally the corpus gate + CI + docs. Each task authors its ACs' Rehearse scenarios (self-hosting: the runner's own acceptance scenarios run through it) plus the reviewer-advisory unit coverage noted in its body. Commits carry Verifies: trailers.

## Tasks

### Task 1: Runner core — parsing, discovery, bash block, human report

**Verifies:** cli/rehearse/run#ac:failing-scenario-fails-run, cli/rehearse/run#ac:standalone-run
**Depends-On:** —
**Status:** planning

`internal/rehearse/scenario` (body metadata incl. Verifies grammar with parenthetical stripping, fenced-block extraction with info-string params), `internal/rehearse/runner` (discovery incl. `_tests/` default and exit-2-on-empty, ordered execution, skipped-after-failure, no-steps status), the bash executor (`bash -euo pipefail`, 8KiB capture truncation), human report + exit codes, `internal/cli/rehearse.go` group + `run` verb. Unit coverage includes the `no-steps` status (reviewer advisory).

### Task 2: JSON report

**Verifies:** cli/rehearse/run#ac:json-report-shape
**Depends-On:** 1
**Status:** planning

`--format json` emitting `{file, status, verifies[], duration_ms, bag:{}, steps:[{kind,status,detail}]}` (bag present, empty until Task 4).

### Task 3: sql + dtql executors

**Verifies:** cli/rehearse/run#ac:sql-assert-rows, cli/rehearse/run#ac:dtql-counts-facts
**Depends-On:** 1
**Status:** planning

Shared trailing-directive parser (`-- assert-rows:`, `-- assert-row-json:`, `-- capture:`); sql executor (sqlite DSN via the in-repo pure-Go driver); dtql executor (dalgo `dtql` package + SQLite adapter against `db=` stores, incl. a Studio `facts.db` fixture). Unit coverage includes `-- assert-row-json` (reviewer advisory).

### Task 4: Context bag

**Verifies:** cli/rehearse/run#ac:context-bag-chains
**Depends-On:** 2, 3
**Status:** planning

Bag in the runner: textual `{{name}}` interpolation for bash/sql/dtql bodies+params (unknown → step fail naming it), `$REHEARSE_CAPTURES` file for bash, `-- capture:` merge for sql/dtql, bag state in the JSON report. Unit coverage includes capture ordering and unknown-variable failure.

### Task 5: hurl + graphql executors

**Verifies:** cli/rehearse/run#ac:hurl-pass, cli/rehearse/run#ac:hurl-missing-skips, cli/rehearse/run#ac:graphql-jsonpath
**Depends-On:** 4
**Status:** planning

Hurl delegation (`hurl --test` + JSON report parsing for `[Captures]` merge), upfront missing-binary scan skipping whole scenarios pre-execution, bag passed via `--variable` flags (no textual pre-interpolation); graphql executor compiling query + `-- variables:` + `-- assert-jsonpath:` directives onto a generated Hurl file, `-- capture-jsonpath:` support. Unit coverage includes captures merge + capture-jsonpath (reviewer advisory).

### Task 6: Corpus gate, CI, docs

**Verifies:** cli/rehearse/run#ac:corpus-green
**Depends-On:** 5
**Status:** planning

The 11 Studio scenarios run green via `specscore rehearse run` (minimal shape-compliance edits only if required); CI job added to the repo workflow running the corpus; README section for `rehearse run` documenting block kinds, directives, the bag, and exit codes.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
