---
format: https://specscore.md/plan-specification
status: Implemented
---

# Plan: Cli Rehearse File Assertions

**Status:** Implemented
**Source Feature:** cli/rehearse/file-assertions
**Date:** 2026-07-11
**Owner:** ai
**Supersedes:** —

## Summary

Implements file assertion blocks in Rehearse scenarios: `### Assert: file <path> <kind>` with code-block details. Supports exists, missing, contains, not-contains, permissions kinds. Requires H3-heading parser (new), file-block struct (new), and runner evaluation integration. 7 ACs, 3 tasks, 100% coverage.

## Approach

Task 1: H3-heading parser for file assertions (internal/rehearse/scenario). Task 2: File block struct + evaluation (internal/rehearse/blocks/fileblock). Task 3: Runner integration + e2e scenarios (internal/rehearse/runner). Each task depends on the prior; each commits with Verifies trailers.

## Tasks

### Task 1: H3-heading parser for file assertions

**Verifies:** cli/rehearse/file-assertions#ac:runner-parses-file-blocks
**Depends-On:** —
**Status:** planning

Extend `internal/rehearse/scenario/scenario.go` to parse `### Assert: file` H3 headings and extract file assertion details (path, kind, code-block content). Add a `FileAssertions []FileAssertion` field to the Scenario struct. Parsing happens as a second pass after fenced-block parsing. Write unit tests covering multi-line code blocks, different kinds (exists, missing, contains, not-contains, permissions), and edge cases (missing code block, invalid kind). 100% coverage.

**Files:** `internal/rehearse/scenario/scenario.go`, `internal/rehearse/scenario/scenario_test.go`

### Task 2: File block struct and evaluation

**Verifies:** cli/rehearse/file-assertions#ac:exists-kind, cli/rehearse/file-assertions#ac:missing-kind, cli/rehearse/file-assertions#ac:contains-kind, cli/rehearse/file-assertions#ac:contains-fails-mismatch, cli/rehearse/file-assertions#ac:permissions-kind, cli/rehearse/file-assertions#ac:not-contains-kind
**Depends-On:** 1
**Status:** planning

Create `internal/rehearse/blocks/fileblock/` package with `FileAssertion` struct (path, kind, expected-value). Implement evaluation methods: `exists()` (stat), `missing()` (inverse), `contains()` (substring match), `not-contains()` (inverse), `permissions()` (octal compare). Each method returns (passed bool, message string). Use os.Stat, os.ReadFile, os.FileMode for introspection. Write unit tests with fixture files (temp dir setup). 100% coverage.

**Files:** `internal/rehearse/blocks/fileblock/fileblock.go`, `internal/rehearse/blocks/fileblock/fileblock_test.go`

### Task 3: Runner integration and e2e scenarios

**Verifies:** cli/rehearse/file-assertions#ac:runner-parses-file-blocks
**Depends-On:** 2
**Status:** planning

Extend `internal/rehearse/runner/run.go` to invoke file assertion evaluation after bash steps in `runScenario()`. Parse scenario.FileAssertions, evaluate each, accumulate pass/fail status. Output assertion results (silent on pass, error message on fail). Author 7 e2e scenario stubs in `spec/features/cli/rehearse/file-assertions/_tests/` (one per AC) with fixture files and expected outcomes. Each scenario exercises one kind (exists, missing, contains, contains-fails, not-contains, permissions, runner-parses) and verifies end-to-end. All 7 ACs are covered by unit tests in Tasks 1-2; Task 3 validates the e2e flow via scenarios. Verify corpus runs green with 100% coverage.

**Files:** `internal/rehearse/runner/run.go`, `internal/rehearse/runner/run_test.go`, 7 scenario stubs in `spec/features/cli/rehearse/file-assertions/_tests/`

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
