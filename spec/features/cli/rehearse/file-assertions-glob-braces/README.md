---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: rehearse file assertions with brace expansion (`{a,b}`)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/file-assertions-glob-braces?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/file-assertions-glob-braces?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/file-assertions-glob-braces?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/file-assertions-glob-braces?op=request-change) |
**Status:** Stable
**Source Ideas:** —
**Supersedes:** —

## Summary

Add brace-expansion (`{a,b}`) to file-assertion glob patterns, resolved via `doublestar`. `### Assert: file build/**/*.{o,a} exists` matches both `.o` and `.a` files. Braces compose with the recursive `**` matching from v0.8 and use the same set-based semantics. This lifts the "braces are literal / out of scope" restriction that v0.7.1 and v0.8 deliberately left in place.

## Problem

Build-verification scenarios frequently need to assert over several extensions at once — object files `{o,a}`, archives `{tar,tgz,zip}`, docs `{md,txt}`. Without brace expansion, each extension needs its own assertion line, and a set-based "all of these must exist / contain X" cannot be written as one pattern. doublestar already supports `{a,b}`; this feature wires it into file assertions.

## Behavior

### Brace syntax

#### REQ: brace-expansion-support

A file-assertion path is treated as a glob when it contains any of `*`, `?`, `[`, or `{`. Patterns containing `{alt1,alt2,…}` are resolved with `doublestar.FilepathGlob`, which expands the alternation (`*.{o,a}` matches `*.o` and `*.a`). Braces compose with `**` (`build/**/*.{o,a}`) and with single-level wildcards.

Resolution routing: a pattern that contains `**` **or** `{` is resolved by doublestar; all other globs (plain `*`/`?`/`[…]`) continue to use the stdlib `filepath.Glob`, so their behavior is byte-for-byte unchanged.

#### REQ: brace-set-semantics

Set-based evaluation is identical to the single-level and recursive glob features, applied to the expanded match set:
- `exists`: pass if ≥1 file matches any alternative.
- `missing`: pass if 0 files match.
- `contains`, `not-contains`, `permissions`: pass if ALL matched files satisfy the condition; vacuously true on 0 matches.

#### REQ: brace-malformed-errors

A malformed brace pattern (e.g. an unbalanced `{`) fails with an `invalid glob pattern` error rather than a silent mismatch, consistent with how other malformed globs are reported.

## Architecture & Components

- `internal/rehearse/blocks/fileblock/fileblock.go` — add `{` to the glob-character detection in `Eval`, and route brace patterns through doublestar in `resolveGlob` (doublestar when the pattern contains `**` or `{`, else `filepath.Glob`). The set-based switch is reused unchanged.

No new dependencies (doublestar was added in v0.8). No new packages.

## Testing Strategy

Unit tests in `fileblock_test.go`:
- `TestEvalGlob_Brace_MatchesAlternatives` — `*.{o,a}` matches both `.o` and `.a` files (exists).
- `TestEvalGlob_Brace_Recursive` — `**/*.{o,a}` matches alternatives across depths.
- `TestEvalGlob_Brace_ContainsAll` — `*.{log,txt} contains "INFO"` passes only if every matched file contains it.
- `TestEvalGlob_Brace_SingleAlternative` — a single-alternative brace (`file.{txt}`) is detected and resolved.
- `TestEvalGlob_Brace_Malformed` — an unbalanced brace errors.

E2e: a scenario in `_tests/` creates `.o` and `.a` files and asserts `*.{o,a} exists`. 100% coverage of the fileblock package.

## Not Doing / Out of Scope

- No negation patterns (`!*.tmp`).
- No numeric ranges (`{1..3}`) — only comma alternation, per doublestar's `{a,b}` syntax.

## Acceptance Criteria

### AC: brace-matches-alternatives

Scenario: brace expands to multiple extensions
Given files `a.o` and `b.a` in the working directory
When I run `### Assert: file *.{o,a} exists`
Then the assertion passes (both alternatives matched)

### AC: brace-composes-with-recursive

Scenario: braces compose with `**`
Given `.o` and `.a` files nested under `build/`
When I run `### Assert: file build/**/*.{o,a} exists`
Then the assertion passes (alternatives matched at any depth)

### AC: brace-contains-all

Scenario: set-based contains over a brace match set
Given `a.log` and `b.txt`, each containing "INFO"
When I run `### Assert: file *.{log,txt} contains` with "INFO"
Then the assertion passes (every matched file contains "INFO")

### AC: brace-malformed-errors

Scenario: an unbalanced brace is a clear error
Given any working directory
When I run `### Assert: file *.{o,a exists`
Then the assertion fails with an `invalid glob pattern` error

## Open Questions

None at this time.

## Autonomous Decisions

- Detection adds `{` to the glob-character set; a path with `{` is now a glob (alternation) rather than a literal, which is the intended behavior change and is doublestar's syntax.
- Routing reuses the v0.8 `resolveGlob` helper: doublestar handles both `**` and `{`; plain globs stay on `filepath.Glob` so existing single-level scenarios are untouched.

---
*This document follows the https://specscore.md/feature-specification*
