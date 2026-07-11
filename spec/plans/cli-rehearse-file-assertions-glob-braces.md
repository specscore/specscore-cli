---
format: https://specscore.md/plan-specification
status: Implemented
---

# Plan: Cli Rehearse File Assertions Glob Braces

**Status:** Implemented
**Source Feature:** cli/rehearse/file-assertions-glob-braces
**Date:** 2026-07-11
**Owner:** ai
**Supersedes:** —

## Summary

Add brace-expansion (`{a,b}`) to file-assertion globs, reusing the v0.8 doublestar dependency and `resolveGlob` helper. Single task: detect `{` as a glob char, route brace patterns through doublestar, reuse the set-based switch, cover to 100%, add one e2e scenario.

## Approach

In `fileblock.go`: add `{` to the `strings.ContainsAny` glob detection in `Eval`, and extend `resolveGlob` to use doublestar when the pattern contains `**` **or** `{` (else `filepath.Glob`, unchanged). Add 5 unit tests and one e2e scenario. Update the v0.7.1/v0.8 "Not Doing" notes to point at this feature. One commit with Verifies trailers.

## Tasks

### Task 1: Brace expansion via doublestar

**Verifies:** cli/rehearse/file-assertions-glob-braces#ac:brace-matches-alternatives, cli/rehearse/file-assertions-glob-braces#ac:brace-composes-with-recursive, cli/rehearse/file-assertions-glob-braces#ac:brace-contains-all, cli/rehearse/file-assertions-glob-braces#ac:brace-malformed-errors
**Depends-On:** —
**Status:** complete

Add `{` to the glob-character detection in `Eval`; extend `resolveGlob` to route `**`-or-`{` patterns through `doublestar.FilepathGlob`. Add 5 unit tests (alternatives, recursive+brace, contains-all, single-alternative, malformed) and one e2e scenario. 100% coverage of the fileblock package.

**Files:** `internal/rehearse/blocks/fileblock/fileblock.go`, `internal/rehearse/blocks/fileblock/fileblock_test.go`, 1 e2e scenario in `spec/features/cli/rehearse/file-assertions-glob-braces/_tests/`

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
