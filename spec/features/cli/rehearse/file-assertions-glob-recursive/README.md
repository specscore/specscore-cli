---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: rehearse file assertions with recursive glob (`**`)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/file-assertions-glob-recursive?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/file-assertions-glob-recursive?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/file-assertions-glob-recursive?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/file-assertions-glob-recursive?op=request-change) |
**Status:** Stable
**Source Ideas:** —
**Supersedes:** cli/rehearse/file-assertions-glob (the `**`-rejection behavior)

## Summary

Extend file-assertion glob patterns with recursive `**` matching, resolved via `github.com/bmatcuk/doublestar/v4`. `### Assert: file build/**/*.o exists` now matches every `.o` file at any depth under `build/`. This supersedes v0.7.1, which detected `**` and rejected it with an error (because Go's `filepath.Glob` treats `**` as a single-level `*`). Set-based semantics are unchanged from v0.7.1 — the only difference is that `**` now resolves to a genuine recursive match set instead of an error.

## Problem

v0.7.1 shipped single-level globs (`*.json`, `dir/*.txt`) and deliberately rejected `**` rather than return a misleading partial match, because `filepath.Glob` has no recursive semantics. Build-verification and multi-file output scenarios routinely need "all `.o` files under `build/`, however deep" or "every `*_test.md` anywhere in the tree" — expressible only with `**`. This feature delivers that recursion.

## Behavior

### Recursive glob syntax

#### REQ: recursive-glob-support

When a file-assertion path contains `**`, it is resolved with `doublestar.FilepathGlob` (not `filepath.Glob`):
- `**` matches zero or more path segments (`build/**/*.o` matches `build/a.o`, `build/x86/a.o`, and `build/x86/deep/a.o`).
- Single-level wildcards (`*`, `?`, `[…]`) inside a `**` pattern retain their usual meaning.
- Relative paths are resolved against the scenario working directory; absolute paths are used as-is (identical to v0.7.1).

Patterns without `**` continue to use `filepath.Glob` (v0.7.1 behavior, unchanged) — doublestar is engaged only when `**` is present.

#### REQ: recursive-set-semantics

Set-based evaluation is identical to v0.7.1's single-level globs, applied to the recursive match set:
- `exists`: pass if ≥1 file matches.
- `missing`: pass if 0 files match (vacuously true otherwise fails).
- `contains`, `not-contains`, `permissions`: pass if ALL matched files satisfy the condition; vacuously true (pass) on 0 matches.

#### REQ: recursive-supersedes-rejection

The v0.7.1 `**`-rejection path (and its `glob-recursive-rejected` AC) is removed. A `**` pattern that previously returned `recursive glob (**) is not supported until v0.8` now resolves and evaluates. A malformed pattern (e.g. an unterminated `[`) still fails with an `invalid glob pattern` error.

## Architecture & Components

- `internal/rehearse/blocks/fileblock/fileblock.go` — in `Eval`, route glob resolution through a helper that uses `doublestar.FilepathGlob` when the path contains `**` and `filepath.Glob` otherwise; the existing set-based switch is reused unchanged. Remove the `**`-rejection guard added in v0.7.1.
- `go.mod` / `go.sum` — add `github.com/bmatcuk/doublestar/v4`.

No new packages. The set-based kind logic is shared between single-level and recursive globs.

## Testing Strategy

Unit tests in `fileblock_test.go`:
- `TestEvalGlob_Recursive_MatchesAllDepths` — `**/*.txt` matches files at depths 0, 1, 2 (all counted).
- `TestEvalGlob_Recursive_ContainsAllMatch` — `**/*.log contains "INFO"` passes only if every match contains it.
- `TestEvalGlob_Recursive_ContainsPartialFail` — fails if any deep match lacks the substring.
- `TestEvalGlob_Recursive_NoMatches_Missing` — `**/*.bak missing` passes when none exist.
- `TestEvalGlob_Recursive_InvalidPattern` — malformed recursive pattern errors.

E2e: a scenario in `_tests/` creates a nested fixture tree and asserts `**` matches across depths. 100% coverage of the fileblock package.

## Not Doing / Out of Scope

- No brace expansion (`{a,b}`) — doublestar supports it, but it stays out of scope for parity with v0.7.1's "Go glob syntax" contract; may be a later feature.
- No negation patterns (`!*.tmp`).
- No change to single-level glob behavior — `*`/`?`/`[…]` without `**` still go through `filepath.Glob`.

## Acceptance Criteria

### AC: glob-recursive-matches-all-depths

Scenario: `**` matches files at every depth
Given files at `build/x86/obj.o`, `build/arm/obj.o`, and `build/arm/deep/obj.o`
When I run `### Assert: file build/**/*.o exists`
Then the assertion passes (all three matched, at depths 1 and 2)

### AC: glob-recursive-contains-all

Scenario: recursive `contains` holds over the whole match set
Given `logs/a.log` and `logs/sub/b.log`, each containing "INFO"
When I run `### Assert: file logs/**/*.log contains` with "INFO"
Then the assertion passes (every matched file, any depth, contains "INFO")

### AC: glob-recursive-contains-partial-fail

Scenario: recursive `contains` fails if any deep file lacks the substring
Given `logs/a.log` contains "INFO" and `logs/sub/b.log` contains "ERROR"
When I run `### Assert: file logs/**/*.log contains` with "INFO"
Then the assertion fails (the nested `b.log` does not contain "INFO")

### AC: glob-recursive-no-matches-missing

Scenario: recursive `missing` passes when nothing matches
Given no `.bak` files anywhere in the tree
When I run `### Assert: file **/*.bak missing`
Then the assertion passes

## Open Questions

- Should brace expansion (`{o,a}`) be enabled now that doublestar supports it? Deferred — keep parity with v0.7.1's syntax contract.

## Autonomous Decisions

- `github.com/bmatcuk/doublestar/v4` is the de-facto Go library for `**` globbing; `FilepathGlob` returns OS-separator paths matching `filepath.Glob`'s shape, so the set-based kind logic is reused verbatim.
- doublestar is engaged only when `**` is present, so non-recursive globs keep identical (stdlib) behavior and there is no risk of subtle semantic drift for existing scenarios.

---
*This document follows the https://specscore.md/feature-specification*
