---
format: https://specscore.md/plan-specification
status: Implemented
---
# Plan: Cli Rehearse New

**Status:** Implemented
**Source Feature:** cli/rehearse/new
**Date:** 2026-07-10
**Owner:** ai
**Supersedes:** —

## Summary

Implements Rehearse v0.5 (`cli/rehearse/new`): `specscore rehearse new <feature-slug>#ac:<ac-slug>` scaffolds a Rehearse scenario file pre-populated with the AC's Given/When/Then text, `Verifies:` metadata, correct frontmatter, and a placeholder bash block ready for implementation. Subsystems touched: new `internal/rehearse/scaffold` package (resolver, extractor, generator), `internal/cli/rehearse.go` (new `rehearseNewCommand()`), testdata fixtures, and e2e scenario stubs under `spec/features/cli/rehearse/new/_tests/`.

## Approach

New package first, CLI wiring second, integration proof last. Task 1 lands the AC resolver — the filesystem lookup that maps a `<feature-slug>#ac:<ac-slug>` reference to the raw AC body text, with all error paths (missing feature, missing AC, unparsable reference). Task 2 adds the Given/When/Then extractor — a pure function over the raw AC body that finds `Given`/`When`/`Then` lines (plain or bold), with raw-text fallback. Task 3 adds the scaffold generator — combines resolver output, extractor output, and hard-coded metadata strings into the final scaffold content string, tested against golden fixtures. Task 4 wires the `rehearse new` subcommand into the CLI, adds `--force` and `--commit` flags, and orchestrates the pipeline with all file-I/O seams. Task 5 authors testdata fixtures and e2e scenario stubs that verify all five ACs end-to-end. Each task keeps `scripts/coverage-gate.sh` at 100% and commits with `Verifies:` trailers.

## Tasks

### Task 1: AC resolver

**Verifies:** cli/rehearse/new#ac:resolve-ac-reference, cli/rehearse/new#ac:missing-ac-error
**Depends-On:** —
**Status:** planning

Parse `<feature-slug>#ac:<ac-slug>` into its two parts (invalid format → exit 2 with format guidance), read `spec/features/<feature-slug>/README.md` (missing file → exit 2 naming the path), scan for the `### AC: <ac-slug>` heading and return the raw text lines that follow up to the next heading (missing AC → exit 2 listing available ACs). The resolver is a pure function over an injected read-file seam so unit tests never touch the real filesystem. Verification: unit tests in `resolver_test.go` cover all three error paths (bad reference, missing feature, missing AC) plus a happy-path fixture; `scripts/coverage-gate.sh` stays green.

**Files:** `internal/rehearse/scaffold/resolver.go`, `internal/rehearse/scaffold/resolver_test.go`

### Task 2: Given/When/Then extractor

**Verifies:** cli/rehearse/new#ac:extract-given-when-then
**Depends-On:** 1
**Status:** planning

Implement `Extract(acBody []string) ExtractedAC` as a pure function: after an optional `Scenario: <name>` line, collect lines whose first word (stripping `**` bold markers) is `Given`, `When`, or `Then`; extraction stops at the next `### AC:` or `##` heading; if no `Given`/`When`/`Then` lines are found the function falls back to returning the raw body as-is. No filesystem access; entirely unit-testable from inline string slices. Verification: `extractor_test.go` covers plain-text lines, bold-marker lines, no-GWT fallback, and multiline bodies; `scripts/coverage-gate.sh` stays green.

**Files:** `internal/rehearse/scaffold/extractor.go`, `internal/rehearse/scaffold/extractor_test.go`

### Task 3: Scaffold generator

**Verifies:** cli/rehearse/new#ac:frontmatter-verifies-metadata, cli/rehearse/new#ac:placeholder-bash-block
**Depends-On:** 2
**Status:** planning

Implement `Generate(featureSlug, acSlug string, extracted ExtractedAC) string` returning the full scaffold markdown: YAML frontmatter (`format: https://specscore.md/scenario-specification`), `# Rehearse: <humanized-title>` heading, `**Status:** pending` and `**Verifies:** <feature-slug>#ac:<ac-slug>` metadata lines, `Scenario source:` link line, the extracted Given/When/Then (or raw fallback) text, and the `### Step: [TODO ...]` section with the placeholder bash block. Golden-fixture tested: the generator's output for a canonical input is diff-compared against a committed `.golden` file so any format drift is caught immediately. Verification: `generator_test.go` asserts the golden fixture matches; `scripts/coverage-gate.sh` stays green.

**Files:** `internal/rehearse/scaffold/generator.go`, `internal/rehearse/scaffold/generator_test.go`

### Task 4: CLI wiring

**Verifies:** cli/rehearse/new#ac:resolve-ac-reference, cli/rehearse/new#ac:missing-ac-error, cli/rehearse/new#ac:commit-flag
**Depends-On:** 3
**Status:** planning

Add `rehearseNewCommand()` to `internal/cli/rehearse.go` and register it under `rehearseCommand()` (alongside `rehearseRunCommand()`). The command accepts exactly one positional `<ac-ref>` arg (missing → exit 2), wires `--force` (overwrite existing file) and `--commit` (git commit after write) flags, orchestrates resolver → extractor → generator → file I/O (stat, mkdirAll, writeFile seams mirroring `internal/rehearse/runner/stubs.go`), and on `--commit` calls an injected git-exec seam with the conventional commit message and `Verifies:` trailer. Existing file without `--force` → exit 2. Unwritable `_tests/` → exit 2. `--commit` failure after successful write → exit 1 (scaffold survives). Verification: `rehearse_test.go` CLI-layer tests cover each exit path and the happy path using seam injection; `scripts/coverage-gate.sh` stays green.

**Files:** `internal/cli/rehearse.go`, `internal/cli/rehearse_test.go`

### Task 5: Integration — testdata fixtures and e2e scenario stubs

**Verifies:** cli/rehearse/new#ac:resolve-ac-reference, cli/rehearse/new#ac:extract-given-when-then, cli/rehearse/new#ac:frontmatter-verifies-metadata, cli/rehearse/new#ac:placeholder-bash-block, cli/rehearse/new#ac:missing-ac-error
**Depends-On:** 4
**Status:** planning

Add shared testdata fixtures (a minimal fixture Feature README at `internal/rehearse/scaffold/testdata/features/cli/studio/index/README.md` with a well-formed `### AC: index-two-repos` section and Given/When/Then lines) plus the committed `.golden` scaffold file used by Task 3. Author five `_tests/` scenario stubs (one per AC) under `spec/features/cli/rehearse/new/_tests/` with `**Status:** pending` and correct `**Verifies:**` lines, each containing the placeholder bash block from Task 3 so the corpus runner sees them as pending-but-present rather than missing. Confirm end-to-end: `go test ./...` passes and `scripts/coverage-gate.sh` reports 100%.

**Files:** `internal/rehearse/scaffold/testdata/features/cli/studio/index/README.md`, `internal/rehearse/scaffold/testdata/scaffold.golden`, `spec/features/cli/rehearse/new/_tests/resolve-ac-reference.md`, `spec/features/cli/rehearse/new/_tests/extract-given-when-then.md`, `spec/features/cli/rehearse/new/_tests/frontmatter-verifies-metadata.md`, `spec/features/cli/rehearse/new/_tests/placeholder-bash-block.md`, `spec/features/cli/rehearse/new/_tests/missing-ac-error.md`

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
