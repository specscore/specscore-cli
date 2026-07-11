---
format: https://specscore.md/plan-specification
status: Approved
---

# Plan: Cli Rehearse File Assertions Glob

**Status:** Approved
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

**Verifies:** cli/rehearse/file-assertions-glob#ac:glob-single-match, cli/rehearse/file-assertions-glob#ac:glob-multiple-match, cli/rehearse/file-assertions-glob#ac:glob-partial-match-fail, cli/rehearse/file-assertions-glob#ac:glob-no-matches-missing, cli/rehearse/file-assertions-glob#ac:glob-recursive
**Depends-On:** —
**Status:** pending

Modify `Eval()` in `fileblock.go`: check if path contains glob characters; if yes, resolve via `filepath.Glob()`, apply kind function to each matched file, return (passed if all match, error message if any fail). Add 8 unit tests (glob-single, glob-multiple, glob-partial-fail, glob-no-match-exists, glob-no-match-missing, glob-no-match-contains, glob-permissions, glob-recursive). 100% coverage.

**Files:** `internal/rehearse/blocks/fileblock/fileblock.go`, `internal/rehearse/blocks/fileblock/fileblock_test.go`, 1 e2e scenario in `spec/features/cli/rehearse/file-assertions-glob/_tests/`

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
