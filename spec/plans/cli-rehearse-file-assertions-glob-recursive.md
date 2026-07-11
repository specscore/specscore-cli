---
format: https://specscore.md/plan-specification
status: Implemented
---

# Plan: Cli Rehearse File Assertions Glob Recursive

**Status:** Implemented
**Source Feature:** cli/rehearse/file-assertions-glob-recursive
**Date:** 2026-07-11
**Owner:** ai
**Supersedes:** —

## Summary

Add recursive `**` glob support to file assertions via `github.com/bmatcuk/doublestar/v4`, superseding v0.7.1's `**`-rejection. Single task: route glob resolution through a helper (doublestar for `**`, `filepath.Glob` otherwise), remove the rejection guard, reuse the existing set-based switch, and cover the recursive paths to 100%. One e2e scenario over a nested fixture tree.

## Approach

In `internal/rehearse/blocks/fileblock/fileblock.go`, extract a `resolveGlob(pattern string) ([]string, error)` helper that calls `doublestar.FilepathGlob(pattern)` when the pattern contains `**` and `filepath.Glob(pattern)` otherwise. Replace the v0.7.1 `if strings.Contains(fa.Path, "**") { return reject }` guard with a call to this helper; the set-based kind switch is unchanged. Add `doublestar/v4` to go.mod. Add 5 unit tests for the recursive paths plus one e2e scenario. Update the v0.7.1 glob feature/spec cross-references to note supersession. One commit with Verifies trailers.

## Tasks

### Task 1: Recursive glob via doublestar

**Verifies:** cli/rehearse/file-assertions-glob-recursive#ac:glob-recursive-matches-all-depths, cli/rehearse/file-assertions-glob-recursive#ac:glob-recursive-contains-all, cli/rehearse/file-assertions-glob-recursive#ac:glob-recursive-contains-partial-fail, cli/rehearse/file-assertions-glob-recursive#ac:glob-recursive-no-matches-missing
**Depends-On:** —
**Status:** complete

Add `github.com/bmatcuk/doublestar/v4` to go.mod. In `fileblock.go`, add `resolveGlob` (doublestar for `**`, `filepath.Glob` otherwise) and remove the `**`-rejection guard so `Eval` resolves recursive patterns and applies the existing set-based logic. Add 5 unit tests (matches-all-depths, contains-all, contains-partial-fail, no-matches-missing, invalid-pattern) and one e2e scenario over a nested fixture tree. 100% coverage of the fileblock package.

**Files:** `internal/rehearse/blocks/fileblock/fileblock.go`, `internal/rehearse/blocks/fileblock/fileblock_test.go`, `go.mod`, `go.sum`, 1 e2e scenario in `spec/features/cli/rehearse/file-assertions-glob-recursive/_tests/`

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
