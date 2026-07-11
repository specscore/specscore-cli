---
format: https://specscore.md/plan-specification
status: Implemented
---

# Plan: Cli Rehearse File Assertions Glob

**Status:** Implemented
**Source Feature:** cli/rehearse/file-assertions-glob
**Date:** 2026-07-11
**Owner:** ai
**Supersedes:** —

## Summary

Extend fileblock package to support glob patterns in file assertion paths. Reuse existing `Eval()` and kind-specific functions; add glob detection, resolution, and set-based logic. Single task, 100% coverage, 8 unit tests + 1 e2e scenario. Straightforward integration.

## Approach

Modify `internal/rehearse/blocks/fileblock/fileblock.go`: detect glob characters (`*?[`) in paths via `strings.ContainsAny()`, resolve with `filepath.Glob()`, apply kind functions across all matched files (AND semantics). Add 8 unit tests + e2e scenario fixture. One commit with Verifies trailers.

## Tasks

### Task 1: Glob pattern support in fileblock

**Verifies:** cli/rehearse/file-assertions-glob#ac:glob-single-match, cli/rehearse/file-assertions-glob#ac:glob-multiple-match, cli/rehearse/file-assertions-glob#ac:glob-partial-match-fail, cli/rehearse/file-assertions-glob#ac:glob-no-matches-missing
**Depends-On:** —
**Status:** complete

Modify `Eval()` in `fileblock.go`: check if path contains glob characters; if yes, reject recursive `**` with a clear error, otherwise resolve via `filepath.Glob()`, apply the kind function to each matched file, return (passed if all match, error message if any fail). Unit tests cover glob-single, glob-multiple, glob-partial-fail, glob-no-match (exists/missing/contains), glob-permissions, not-contains, invalid-pattern, unknown-kind, and `**`-rejected. 100% coverage of the fileblock package.

**Files:** `internal/rehearse/blocks/fileblock/fileblock.go`, `internal/rehearse/blocks/fileblock/fileblock_test.go`, 1 e2e scenario in `spec/features/cli/rehearse/file-assertions-glob/_tests/`

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
