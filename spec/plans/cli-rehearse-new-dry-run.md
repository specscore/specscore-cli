---
format: https://specscore.md/plan-specification
status: Implemented
---

# Plan: Cli Rehearse New Dry Run

**Status:** Implemented
**Source Feature:** cli/rehearse/new-dry-run
**Date:** 2026-07-10
**Owner:** ai
**Supersedes:** —

## Summary

Implements `specscore rehearse new <ac-ref> --dry-run`: prints the generated scaffold markdown to stdout without creating files or committing to git. Reuses v0.5's resolver/extractor/generator; adds a single flag and output branch in the CLI command handler. Three ACs, one task, 100% coverage.

## Approach

Single task: modify `rehearseNewCommand()` in `internal/cli/rehearse.go` to add the `--dry-run` flag and thread an `io.Writer` through `runRehearseNew()` (following the pattern `runRehearseRun` already uses with `cmd.OutOrStdout()`). After scaffold generation, branch: if dry-run, print to stdout and return; else, existing file-write/commit logic. Write 5 unit tests covering the flag interactions and error paths. 100% coverage gate. One commit with Verifies trailers for all three ACs.

## Tasks

### Task 1: CLI dry-run flag and output branch

**Verifies:** cli/rehearse/new-dry-run#ac:dry-run-flag, cli/rehearse/new-dry-run#ac:dry-run-ignores-flags, cli/rehearse/new-dry-run#ac:error-handling-same
**Depends-On:** —
**Status:** planning

Modify `internal/cli/rehearse.go`:
- Add `dryRun bool` flag to `rehearseNewCommand()` 
- Thread `out io.Writer` parameter through `runRehearseNew()` (default to `cmd.OutOrStdout()`)
- After `content := scaffold.Generate(...)`, check `if dryRun { fmt.Fprint(out, content); return nil }`
- Else, existing path (file write, optional commit)

Write `internal/cli/rehearse_new_test.go` tests (5 total):
- `TestRehearseNew_DryRunPrintsScaffold` — scaffolds prints to captured writer
- `TestRehearseNew_DryRunIgnoresForce` — `--dry-run --force` prints (no file check)
- `TestRehearseNew_DryRunIgnoresCommit` — `--dry-run --commit` prints (no git call)
- `TestRehearseNew_DryRunExitCodeSame` — error path (missing AC) exits 2 with stderr
- `TestRehearseNew_DryRunNormalWriteWorks` — without `--dry-run`, write still works

Verification: TDD (failing test first), 100% coverage gate before commit. One commit with Verifies trailers for all three ACs.

**Files:** `internal/cli/rehearse.go`, `internal/cli/rehearse_new_test.go`

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
