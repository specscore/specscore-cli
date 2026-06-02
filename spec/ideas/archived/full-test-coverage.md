# Idea: Full Test Coverage — Path to 100%

**Status:** Archived
**Date:** 2026-05-25
**Owner:** alexander.trakhimenok
**Promotes To:** —
**Supersedes:** —
**Related Ideas:** —
**Archive Reason:** Achieved — 100% statement coverage enforced by CI across all packages.

## Problem Statement

How might we achieve and maintain 100% test coverage across the specscore-cli module without resorting to impractical testing patterns?

## Context

As of 2026-05-25, the specscore-cli module reached **98.0%** overall test coverage across two concentrated sprints. The first sprint (commit `bad217b`) established the `var`-stub pattern: package-level `var` stubs defaulting to real stdlib functions, replaced in tests via `t.Cleanup`. `internal/telemetry` hit 100%, `pkg/lifecycle` 96%, `internal/cli` 95%. The second sprint (commits `247cffb`, `73dde24`, `d0f04a4`) extended the pattern with encoder factory vars (`newYAMLEnc`/`newJSONEnc`), function stubs (`lintLintFn`, `osStatFn`, `installIDFn`, `statePathFn`), and targeted test fixtures for `pkg/idearelocate` rollback/link/preflight paths. Current per-package state: `internal/telemetry` 100%, `pkg/task` 100%, `pkg/gitremote` 100%, `pkg/lifecycle` 98.9%, `pkg/idea` 99.3%, `pkg/lint` 98.2%, `pkg/feature` 98.4%, `pkg/idearelocate` 94.9%, `internal/cli` 97.2%, `pkg/sourceref` 96.6%.

The remaining ~2% clusters into three categories: (1) `pkg/idearelocate` at 94.9% — `filepath.Rel` error branches in `ExecutePreCommitPhase` rollback (lines 63, 89) that are genuinely unreachable on same-volume absolute paths, plus commit-phase git error paths that require multi-repo git failure simulation; (2) scattered `if err != nil` guards in `internal/cli` for `os.Getwd()` failures (lines 38, 50), format-switch dead-code (lines 136, 232, 353), and the `filepath.Rel` error in `resolveSpecRoot` (line 322) — all genuinely unreachable under normal OS operation; (3) `pkg/sourceref` at 96.6% with WalkDir callback edge cases.

## Recommended Direction

The `var`-stub pattern proved sufficient to reach 98.0% without interface extraction. To close the remaining ~2% to 100%, two categories of work remain:

**(1) `pkg/idearelocate` commit-phase git error paths.** `ExecuteCommitPhase` error handling and the multi-repo git rollback fixture are the highest-value remaining targets (~15 uncovered statements). Create a test helper that sets up source + dest repos with `git init`, stages changes, and simulates commit failure by making `.git/objects` read-only. Reliable on macOS + Linux CI.

**(2) `//coverage:ignore` directives for genuinely unreachable paths.** Approximately 20 statements across the codebase are genuinely unreachable: `filepath.Rel` failures on same-volume absolute paths, `os.Getwd()` errors, format-switch dead-code after format validation, and `init()` panic guards. These should be marked with `//coverage:ignore` when Go 1.25+ supports it (track golang/go#51430).

## Alternatives Considered

- **Interface extraction for method-call error paths.** Initially recommended for the 96.5%→98% push, but the `var`-stub pattern (encoder factories, function pointer stubs) covered all the same paths without introducing interface types — keeping the production code simpler.
- **`//coverage:ignore` directives (Go 1.24+).** Mark remaining unreachable statements as intentionally uncovered. Appropriate for the ~20 genuinely unreachable paths but not yet available; track golang/go#51430.
- **Accept 98.0% as the practical ceiling.** The remaining ~2% is genuinely hard-to-test code: OS-level failure modes (`os.Getwd`, `filepath.Rel` on same volume), format-switch dead-code after format validation, and `init()` panic guards. These have never fired in production. 98% is a pragmatic ceiling given the current Go toolchain.

## MVP Scope

**Achieved:** 98.0% overall coverage as of commit `d0f04a4`. The 98% target was reached via the `var`-stub pattern throughout — no interface extraction was needed. The next milestone toward 100% is the `pkg/idearelocate` commit-phase fixture (see Recommended Direction item 1).

## Not Doing (and Why)

- Full `afero` filesystem abstraction — the `var`-stub approach proved sufficient for all I/O error paths; `afero` would add a dependency and require rewriting every function signature for marginal gain
- Covering `cmd/specscore/main.go` — it's a 3-line wrapper calling `cli.Run()`; Go's coverage tool doesn't attribute subprocess execution to the parent profile. A `main_test.go` with `go build` + `exec.Command` integration tests exists but doesn't contribute to the coverage number
- Achieving exactly 100.0% — approximately 5 statements are genuinely unreachable: `filepath.Rel` failure between two absolute paths on the same volume, `init()` panic guards that protect against impossible matrix states, and `scanner.Err()` after successful iteration (requires mid-read kernel I/O failure). These should use `//coverage:ignore` when Go supports it

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | The 98% floor holds across Go version bumps and new code additions | Add a CI coverage gate: `go tool cover -func=cover.out \| grep total \| awk '{if ($NF+0 < 98) exit 1}'` |
| Should-be-true | The `pkg/idearelocate` git-rollback fixture is reliable across CI environments (macOS + Linux) | The test uses `git init` + `chmod` in temp dirs; verify on both platforms |
| Might-be-true | The remaining ~20 genuinely unreachable statements can be marked with `//coverage:ignore` in a future Go release | Track golang/go#51430 for coverage exclusion directive support |

## SpecScore Integration

- **New Features this would create:** none (this is a quality/infrastructure improvement)
- **Existing Features affected:** none
- **Dependencies:** none

## Open Questions

- Should we adopt a CI coverage gate at 98% to prevent regressions? The gate command would be: `go test ./... -coverprofile=cover.out && go tool cover -func=cover.out | grep total | awk '{if ($NF+0 < 98) exit 1}'`
- Is the `var`-stub pattern safe under `-race`? (Yes — each test restores via `t.Cleanup` and tests run sequentially within a package by default. Parallel sub-tests touching the same `var` would need `sync.Mutex`.)
- Should the ~20 genuinely unreachable statements be deleted rather than marked? E.g., the format-switch dead-code (lines 136, 232, 353 in task.go) is truly dead — it can never be reached because format validation happens earlier. Deleting it would be cleaner but removes defense-in-depth. The `init()` panic guard is harder to remove safely.

---
*This document follows the https://specscore.md/idea-specification*
