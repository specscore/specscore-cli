---
format: https://specscore.md/plan-specification
status: Implemented
---

# Plan: Cli Rehearse Run Filter

**Status:** Implemented
**Source Feature:** cli/rehearse/run-filter
**Date:** 2026-07-11
**Owner:** ai
**Supersedes:** —

## Summary

Implements `specscore rehearse run --filter <ac-ref>`: filters scenarios by `**Verifies:**` field before execution. Supports multiple `--filter` flags (OR semantics). 7 ACs, 1 task, 100% coverage.

## Approach

Single task: add `--filter` flag to `rehearseRunCommand()`, validate AC references, filter scenario.Verifies slices before passing to runner, adjust output labeling for matched vs. skipped scenarios. Write 7 unit tests covering flag syntax, matching, OR accumulation, no-filter default, invalid syntax, output labels, and zero-match behavior. 100% coverage.

## Tasks

### Task 1: CLI --filter flag and scenario filtering

**Verifies:** cli/rehearse/run-filter#ac:filter-flag-syntax, cli/rehearse/run-filter#ac:filter-matching-exact, cli/rehearse/run-filter#ac:filter-multiple-or, cli/rehearse/run-filter#ac:no-filter-default, cli/rehearse/run-filter#ac:filter-invalid-syntax, cli/rehearse/run-filter#ac:filter-output-labels, cli/rehearse/run-filter#ac:filter-no-matches
**Depends-On:** —
**Status:** planning

Modify `internal/cli/rehearse.go` `rehearseRunCommand()`: add `--filter` flag (string slice, repeatable). Before scenario execution, parse each filter as an AC reference (validate format: must contain `#ac:` and have non-empty parts). Filter the scenarios slice by checking scenario.Verifies (OR semantics: include if any Verifies matches any filter). Adjust output: prepend `[filter-match]` or `[filter-skip]` labels. Handle zero-match case: exit 0 with "No scenarios matched filter(s): ..." message. Write 7 unit tests covering all paths (flag parsing, exact-match filtering, OR accumulation, default no-filter passthrough, invalid syntax error, output labels, zero-match). 100% coverage.

**Files:** `internal/cli/rehearse.go`, `internal/cli/rehearse_run_test.go`

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
