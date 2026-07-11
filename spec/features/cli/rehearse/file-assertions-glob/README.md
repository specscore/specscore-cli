---
format: https://specscore.md/feature-specification
status: Implemented
---

# Feature: rehearse file assertions with glob patterns

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/file-assertions-glob?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/file-assertions-glob?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/file-assertions-glob?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/file-assertions-glob?op=request-change) |
**Status:** Implemented
**Source Ideas:** —
**Supersedes:** —

## Summary

Extend file assertion paths to support glob patterns (e.g., `*.json`, `build/**/*.o`, `spec/**/*_test.md`). Matches multiple files; assertion passes if all matched files satisfy the condition (e.g., all `.json` files contain "version"). Enables scenario assertions over sets of files rather than individual paths. Complements v0.6.2's file assertions by adding set-based matching.

## Problem

v0.6.2's file assertions verify individual files by exact path. Some scenarios need to assert properties over file sets: all `.json` config files are valid, all `.log` files contain expected patterns, all generated `.o` files exist. Glob patterns enable these assertions without hardcoding each file path. Common in build verification and multi-file output scenarios.

## Behavior

### Glob pattern syntax

#### REQ: glob-pattern-support

File assertion paths support single-level glob patterns:
- `*.ext` — all files with that extension in the current directory
- `dir/*.txt` — all `.txt` files in a specific directory
- `?` and `[…]` wildcards per Go's `filepath.Glob` semantics

Glob syntax follows Go's `filepath.Glob` semantics for v0.7. Recursive `**`
globbing (e.g. `build/**/*.o`) is **not supported** in v0.7 — `filepath.Glob`
treats `**` as a single-level `*`, so a `**` pattern is rejected with a clear
error rather than returning a misleading partial match. True recursion is
deferred to v0.8 (requires the `github.com/bmatcuk/doublestar` dependency).

#### REQ: glob-matching-semantics

When a path contains glob characters (`*`, `?`, `[`, `{`):
- Resolve the glob pattern (find all matching files)
- For `exists` and `missing`: pass if count matches (e.g., exists passes if ≥1 match, missing passes if 0 matches)
- For `contains` and `not-contains`: pass if ALL matched files satisfy the condition
- For `permissions`: pass if ALL matched files have the specified mode

Example: `### Assert: file *.json contains` with content `"version"` passes only if every `.json` file in the directory contains the substring "version".

#### REQ: glob-no-matches-handling

If a glob pattern matches zero files:
- `exists`: fails (no files found)
- `missing`: passes (as expected)
- `contains`, `not-contains`, `permissions`: passes (vacuously true: all zero files satisfy the condition)

## Architecture & Components

- `internal/rehearse/blocks/fileblock/fileblock.go` — extend `Eval()` and kind-specific functions to detect glob patterns, resolve them via `filepath.Glob`, and apply set-based logic
- `internal/rehearse/blocks/fileblock/fileblock_test.go` — add tests for glob resolution and set-based assertions

No new packages. Reuse existing fileblock evaluation with glob preprocessing.

## Testing Strategy

Unit tests for glob resolution and set-based assertions:
- `TestEvalGlob_SingleMatch` — glob resolves to one file, assertion applies to that file
- `TestEvalGlob_MultipleMatches` — glob resolves to multiple files, passes if all satisfy condition
- `TestEvalGlob_NoMatches_Exists` — glob matches zero files, exists fails
- `TestEvalGlob_NoMatches_Missing` — glob matches zero files, missing passes
- `TestEvalGlob_NoMatches_Contains` — glob matches zero files, contains passes (vacuously true)
- `TestEvalGlob_Recursive_Rejected` — a `**` pattern is rejected with a clear error (recursion deferred to v0.8)
- `TestEvalGlob_SetContains_AllMatch` — contains passes only if ALL files match
- `TestEvalGlob_SetContains_PartialMatch` — contains fails if any file doesn't match

E2e: scenario in `spec/features/cli/rehearse/file-assertions-glob/_tests/` creates fixture files, runs glob assertion.

## Not Doing / Out of Scope

- No **negation patterns** (e.g., `!*.tmp`)
- No **character ranges** in glob (e.g., `[0-9]`) — keep to simple wildcards
- No **brace expansion** (e.g., `{a,b}` treated as literal, not expanded) — use Go glob syntax only

## Acceptance Criteria

### AC: glob-single-match

Scenario: glob pattern matching one file
Given files `config.json`, `data.txt`
When I run `### Assert: file *.json exists`
Then the assertion passes (finds one file)

### AC: glob-multiple-match

Scenario: glob pattern matching multiple files
Given files `a.log`, `b.log`, `c.log` each containing "INFO"
When I run `### Assert: file *.log contains` with "INFO"
Then the assertion passes (all three files contain "INFO")

### AC: glob-partial-match-fail

Scenario: glob fails if not all matched files satisfy condition
Given files `a.log` contains "INFO", `b.log` contains "ERROR"
When I run `### Assert: file *.log contains` with "INFO"
Then the assertion fails (b.log doesn't contain "INFO")

### AC: glob-no-matches-missing

Scenario: glob matching zero files, missing assertion
Given no `*.bak` files in directory
When I run `### Assert: file *.bak missing`
Then the assertion passes

### AC: glob-recursive-rejected

Scenario: recursive `**` pattern is rejected (deferred to v0.8)
Given a file at `build/x86/obj.o`
When I run `### Assert: file build/**/*.o exists`
Then the assertion fails with an error that `**` is not supported until v0.8
(rather than silently matching only one directory level)

## Open Questions

- Should glob patterns be supported in other assertion kinds (not just file assertions)? Deferred to v0.8.

## Autonomous Decisions

- Glob syntax follows Go's `filepath.Glob` (single-level) for v0.7 — standard and familiar to Go developers. Recursive `**` (via `doublestar`) is deferred to v0.8; a `**` pattern is rejected loudly in v0.7 to avoid a misleading partial match.
- Set semantics (ALL files must satisfy) is intuitive for "ensure no outliers" use cases.
- Vacuous truth for zero-match cases: `contains` on zero files passes rather than failing. Rationale: glob is a dynamic filter; if nothing matches, the assertion is vacuously satisfied.

---
*This document follows the https://specscore.md/feature-specification*
