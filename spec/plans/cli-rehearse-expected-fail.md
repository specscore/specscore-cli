---
format: https://specscore.md/plan-specification
status: Implemented
---

# Plan: Cli Rehearse Expected Fail

**Status:** Implemented
**Source Feature:** cli/rehearse/expected-fail
**Date:** 2026-07-11
**Owner:** ai
**Supersedes:** —

## Summary

Add a `**Expect:** fail` scenario directive and runner outcome-inversion so negative acceptance scenarios can join the CI corpus. Two tasks: (1) parser + runner + report support with tests; (2) declare the one negative file-assertion scenario as expected-fail and wire the three file-assertion `_tests` directories into the corpus job. 100% coverage on touched packages.

## Approach

Task 1 adds `Scenario.Expect` (parsed from `**Expect:**`, default `pass`) and inverts the terminal pass/fail outcome inside runScenario's single `finish` helper, plus an `Expect` field on `ScenarioReport`. Task 2 marks `contains-fails.md` with `**Expect:** fail` and extends the corpus job's directory list to include the file-assertion families, proving the end-to-end path.

## Tasks

### Task 1: Expect metadata, inversion, and report field

**Verifies:** cli/rehearse/expected-fail#ac:expect-fail-parses, cli/rehearse/expected-fail#ac:expected-fail-reports-pass, cli/rehearse/expected-fail#ac:expected-fail-but-passed-reports-fail
**Depends-On:** —
**Status:** complete

Parse `**Expect:**` into `Scenario.Expect` (default `pass`, unknown → `pass`). In `run.go`, capture the parsed expect and invert the terminal outcome in `finish`: raw fail → pass (retain step detail), raw pass → fail with an explanatory detail; leave no-steps/skipped/parse-error untouched. Add `Expect string json:"expect,omitempty"` to `ScenarioReport`. Unit tests in `scenario` and `runner`; 100% coverage of both packages.

**Files:** `internal/rehearse/scenario/scenario.go`, `internal/rehearse/scenario/scenario_test.go`, `internal/rehearse/runner/run.go`, `internal/rehearse/runner/run_test.go`

### Task 2: Declare the negative scenario and wire the corpus

**Verifies:** cli/rehearse/expected-fail#ac:corpus-runs-file-assertions
**Depends-On:** 1
**Status:** complete

Add `**Expect:** fail` to `file-assertions/_tests/contains-fails.md`. Extend `.github/workflows/go-ci.yml`'s corpus run with `spec/features/cli/rehearse/file-assertions/_tests`, `.../file-assertions-glob/_tests`, and `.../file-assertions-glob-recursive/_tests`. Verify locally that `rehearse run` over those directories exits 0.

**Files:** `spec/features/cli/rehearse/file-assertions/_tests/contains-fails.md`, `.github/workflows/go-ci.yml`

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
