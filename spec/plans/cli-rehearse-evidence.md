---
format: https://specscore.md/plan-specification
status: Approved
---
# Plan: Cli Rehearse Evidence

**Status:** Executing
**Source Feature:** cli/rehearse/evidence
**Date:** 2026-07-10
**Owner:** ai
**Supersedes:** —

## Summary

Implements Rehearse v0.4 (`cli/rehearse/evidence`): `rehearse run --report-out` persists a provenance-carrying JSON report envelope, a fifth studio adapter (`rehearse`) ingests persisted reports as `verified-behavior` facts at `studio index` time, `studio facts --class verified-behavior` serves them, and the repo self-hosts the v0.4 success gate in CI. Subsystems touched: `internal/rehearse/runner`, `internal/cli/rehearse.go`, `internal/studio/{fact,adapters,adapters/rehearse}`, `internal/cli/studio.go`, CI workflow.

## Approach

Producer first, consumer second, wiring third, proof last. Task 1 lands the persisted report (the artifact everything downstream reads) with its provenance fields and repo-relative scenario paths. Task 2 adds the `verified-behavior` class and the pure-function adapter that turns a report into facts. Task 3 wires the adapter into the registry and pipeline (conditional `observed_at` stamping, partial tolerance, `--class` help text) so the facts flow end-to-end through `studio index` and `studio facts`. Task 4 self-hosts the proof: authors the 10 `_tests/` scenarios (replacing the pending stubs), extends the `Rehearse corpus` CI job with the success-gate assertion, and updates docs/.gitignore. Each task keeps `scripts/coverage-gate.sh` at 100% and commits with `Verifies:` trailers.

## Tasks

### Task 1: report-out flag and provenance envelope

**Verifies:** cli/rehearse/evidence#ac:report-out-writes-envelope, cli/rehearse/evidence#ac:report-out-on-failure, cli/rehearse/evidence#ac:report-out-outside-git
**Depends-On:** —
**Status:** complete

Add the `RunReport` envelope (runner_version, git_sha, git_dirty, started_at, scenarios) and report persistence to `internal/rehearse/runner` — git provenance via a test-seam exec wrapper, scenario `file` paths repo-relative inside a git work tree, parent dirs created, unwritable path exits 2 after the stdout report. Wire `--report-out <path>` in `internal/cli/rehearse.go`; stdout formats unchanged.

### Task 2: verified-behavior class and the rehearse adapter

**Verifies:** cli/rehearse/evidence#ac:adapter-emits-verified-by, cli/rehearse/evidence#ac:fail-status-fact, cli/rehearse/evidence#ac:skipped-scenario-no-facts
**Depends-On:** 1
**Status:** planning

Add `VerifiedBehavior Class = "verified-behavior"` to `internal/studio/fact` and the new pure-function adapter package `internal/studio/adapters/rehearse`: read `<repo>/.specscore/rehearse/latest.json`, emit `verified-by` + `has-verification-status` fact pairs per scenario–AC (pass/fail only; skipped/no-steps/empty-verifies emit nothing), subjects `#<verifies-entry>` for repo-slug prefixing, evidence_pointer `.specscore/rehearse/latest.json`, observed_at set to the report's started_at. Fixture-tested like its four siblings.

### Task 3: pipeline wiring, tolerance, and query surface

**Verifies:** cli/rehearse/evidence#ac:observed-at-run-time, cli/rehearse/evidence#ac:malformed-report-warns, cli/rehearse/evidence#ac:missing-report-silent
**Depends-On:** 2
**Status:** planning

Register the adapter in `All()` (one-line append), change `adapters.Run` to stamp `observed_at` only onto facts whose adapter left it empty, warn-and-skip on malformed reports and stay silent on missing ones (partial tolerance), and update the `--class` flag help text in `internal/cli/studio.go` to name all three classes. End-to-end unit coverage through `studio index` + `studio facts`.

### Task 4: self-hosting scenarios, CI gate, and docs

**Verifies:** cli/rehearse/evidence#ac:self-hosting-gate
**Depends-On:** 3
**Status:** planning

Author the 10 `_tests/` scenarios for this feature's ACs (replacing the pending stubs with executable steps), add `.specscore/rehearse/` to `.gitignore`, extend the `Rehearse corpus` CI job to run the corpus with `--report-out`, `studio index` a minimal workspace over this repo, and assert `facts --class verified-behavior` returns rows for `cli/studio/index#ac:*` — the v0.4 success gate. Update the feature statuses (`cli/rehearse/evidence` → Implementing) and any docs the change touches.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
