# Plan: Lint fixed-files report

**Status:** Implemented
**Mode:** full
**Source Feature:** cli/spec/lint
**Date:** 2026-06-02
**Owner:** alexandertrakhimenok
**Supersedes:** —

## Summary

Implementation plan for the new *Fixed-files reporting* increment on the established `cli/spec/lint` Feature. Three tasks cover the four new ACs: capture the set of files `--fix` modifies in `pkg/lint`, then surface that set through the two CLI output paths (structured JSON/YAML envelope and the text-mode stderr summary). The fifteen pre-existing Spec Lint ACs are already implemented and shipped, so they are deferred here rather than re-planned.

## Approach

Foundation-then-consumers, linearized. Task 1 lands the change-capture in `pkg/lint`'s `Lint()` (content-hash snapshot before/after the fix pass) and the `Result` it returns; it is a hard prerequisite for both output tasks because they render the captured set. Tasks 2 and 3 are independent of each other (one touches structured output, the other text output) and both depend only on Task 1 — the linear order 1 → 2 → 3 is a valid linearization of that 1→{2,3} graph. No per-checker interface change is planned; the snapshot is internal to `Lint()`, keeping the ~14 existing fixer write-sites untouched.

The fifteen pre-existing ACs (`clean-tree-exits-0` … `consumer-path-multi-glob-parsed`) describe already-shipped behavior of `spec lint`; they are listed under Deferred AC Coverage with that reason so the P-001 coverage contract is satisfied without re-planning landed work.

## Tasks

### Task 1: Capture the fix-modified file set in `pkg/lint`

**Status:** complete
**Depends-On:** —
**Verifies:** cli/spec/lint#ac:fix-reports-only-changed-files

In `pkg/lint`, when `Lint()` runs with `Fix:true`, snapshot a content hash of every spec-tree file before the fix pass and diff after; collect the modified files as project-relative, de-duplicated, sorted paths (a file whose bytes are unchanged MUST NOT appear). Surface them by returning a `Result{ Violations, Fixed }` (or an equivalent second return value) and update the in-repo callers (`internal/cli/feature.go`, `idea.go`, `decision.go`). The fixer interface (`fix(specRoot) error`) and the ~14 write-sites stay untouched.

**Notes:** This task lands the capture logic that AC `fix-reports-only-changed-files` rests on; that AC is only *end-to-end* observable once the captured set is rendered, which happens in Task 2 (hence Task 2 also lists it under `**Verifies:**`). Unit tests on the `pkg/lint` return value can assert the captured set directly within this task.

### Task 2: Emit the `{fixed, violations}` envelope under `--fix --format json|yaml`

**Status:** complete
**Depends-On:** 1
**Verifies:** cli/spec/lint#ac:fix-json-envelope-only-under-fix, cli/spec/lint#ac:fix-report-needs-no-flag, cli/spec/lint#ac:fix-reports-only-changed-files

In `internal/cli/spec.go`, when `--fix` is set and the format is `json` or `yaml`, marshal a single stdout object carrying both a `fixed` array (the captured paths) and a `violations` array. When `--fix` is absent, keep emitting the bare violations array unchanged, preserving the existing non-fix contract. No opt-in flag gates the report — it is produced whenever `--fix` is set.

### Task 3: Print the text-mode "Fixed N file(s)" summary to stderr under `--fix`

**Status:** complete
**Depends-On:** 1
**Verifies:** cli/spec/lint#ac:fix-text-summary-on-stderr

In default text format, when the fix pass modified one or more files, write a "Fixed N file(s):" summary naming each modified path to **stderr**, leaving stdout as the remaining-violations report (unchanged from a non-`--fix` run). Print no summary when zero files changed. Like Task 2, this output is default-on and requires no flag.

### Task 4: Keep the canonical Plans index derived from single-file Plans

**Status:** complete
**Depends-On:** —
**Verifies:** cli/spec/lint#ac:plan-index-sync-detects-and-fixes-row-drift

Register the `plan-index-sync` checker in the default lint suite. For the canonical Plans-table schema, detect missing rows, stale row values, and duplicate rows, then have the standard `--fix` pass regenerate exactly one current row per direct single-file Plan while preserving surrounding author-maintained prose. Cover both drift detection and idempotent repair with focused tests.

## Deferred AC Coverage

- cli/spec/lint#ac:clean-tree-exits-0 — Already implemented and shipped before this increment; out of scope for this plan, which covers only the new Fixed-files reporting behavior.
- cli/spec/lint#ac:violations-exit-1 — Already implemented and shipped before this increment; this plan covers only the new Fixed-files reporting behavior.
- cli/spec/lint#ac:unknown-rule-name-exits-2 — Already implemented and shipped before this increment; this plan covers only the new Fixed-files reporting behavior.
- cli/spec/lint#ac:fix-idempotent — Already implemented and shipped before this increment; this plan covers only the new Fixed-files reporting behavior.
- cli/spec/lint#ac:adherence-footer-fix-replaces-trailing-wrong-url — Already implemented and shipped before this increment; this plan covers only the new Fixed-files reporting behavior.
- cli/spec/lint#ac:index-entries-fix-removes-phantom-row — Already implemented and shipped before this increment; this plan covers only the new Fixed-files reporting behavior.
- cli/spec/lint#ac:index-entries-fix-inserts-orphan-row — Already implemented and shipped before this increment; this plan covers only the new Fixed-files reporting behavior.
- cli/spec/lint#ac:index-entries-flags-orphan-child — Already implemented and shipped before this increment; this plan covers only the new Fixed-files reporting behavior.
- cli/spec/lint#ac:index-entries-rejects-loose-child-link — Added by a later canonical-index increment; outside this plan's Fixed-files reporting scope.
- cli/spec/lint#ac:oq-section-missing-flagged — Already implemented and shipped before this increment; this plan covers only the new Fixed-files reporting behavior.
- cli/spec/lint#ac:oq-section-legacy-heading-flagged-and-fixed — Already implemented and shipped before this increment; this plan covers only the new Fixed-files reporting behavior.
- cli/spec/lint#ac:dogfood-version-bump-flags-stale-pin — Already implemented and shipped before this increment; this plan covers only the new Fixed-files reporting behavior.
- cli/spec/lint#ac:missing-specscore-yaml-exits-3 — Already implemented and shipped before this increment; this plan covers only the new Fixed-files reporting behavior.
- cli/spec/lint#ac:entity-and-property-rules-selectable — Already implemented and shipped before this increment; this plan covers only the new Fixed-files reporting behavior.
- cli/spec/lint#ac:managed-section-fix-idempotent — Already implemented and shipped before this increment; this plan covers only the new Fixed-files reporting behavior.
- cli/spec/lint#ac:consumer-path-multi-glob-parsed — Already implemented and shipped before this increment; this plan covers only the new Fixed-files reporting behavior.

## Open Questions

- The exact `Lint()` return shape (a new `Result{ Violations, Fixed }` struct vs. a second return value vs. a sibling `LintWithResult`) is settled during Task 1 implementation; it is tracked as an Open Question on the source Feature.
- A terse `--format paths` (one fixed path per line on stdout) is explicitly out of scope for this plan, deferred on the source Feature.

---
*This document follows the https://specscore.md/plan-specification*
