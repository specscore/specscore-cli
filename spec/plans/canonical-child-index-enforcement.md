---
format: https://specscore.md/plan-specification
status: Implemented
---

# Plan: Canonical child index enforcement

**Status:** Implemented
**Source Feature:** cli/spec/lint
**Date:** 2026-08-05
**Owner:** codex
**Supersedes:** —

## Summary

Make child-Feature lint structurally unambiguous: only the canonical table beneath `## Contents` counts as the parent index. This plan covers the lint increment and its isolated negative test; related scaffolder and Proposal fixes are covered directly by their Feature acceptance tests.

## End-to-end journey

An author opens a Feature parent whose child is mentioned in later prose but missing from the `## Contents` table. With nobody changing the files, lint reports the child as unindexed. The author runs the standard index fix; the child row appears in the canonical table, and another reader's lint run is clean.

Observable good results:

1. **Start:** the child Feature exists, the parent table omits it, and a loose link elsewhere remains untouched.
2. **Middle:** lint reports the missing canonical row even though the loose link exists.
3. **End:** the fixer inserts the table row and a second lint run reports no index violation.

Divergent epilogues: if the author keeps the child, the canonical row remains the navigation source; if the child is later removed, the existing phantom-row fixer removes its table row without treating narrative links as index entries.

## Approach

Restrict `index-entries` parsing to the first contiguous Markdown table beneath `## Contents`, retaining the legacy first-table fallback only for repositories without that heading. Cover a loose-link negative case and dogfood the fixer across the CLI's own previously detached rows.

## Tasks

### Task 1: Make the Contents table canonical for lint

**Status:** complete
**Depends-On:** —
**Verifies:** cli/spec/lint#ac:index-entries-rejects-loose-child-link, cli/spec/lint#ac:index-entries-ignores-nonidentity-cell-links, cli/spec/lint#ac:index-entries-fix-removes-duplicate-row

Make `index-entries` parse the canonical table beneath `## Contents`, reject a loose link elsewhere, and retain the existing fixer behavior for the resulting missing-row violation. Cover the negative case in isolation and dogfood the rule against the repository's own indexes.

The landed hardening also derives row membership from the table header's one
unambiguous artifact identity column. This keeps row-sync-derived Summary links
out of membership, supports legacy column order, and leaves missing or
ambiguous identity schemas write-free. Each child identity is unique: lint
reports repeated rows and the fixer retains only the first row in the same
single-write composition used for phantom deletion and orphan insertion.

## Deferred AC Coverage

- cli/spec/lint#ac:clean-tree-exits-0 — Existing shipped behavior outside this canonical-index increment.
- cli/spec/lint#ac:violations-exit-1 — Existing shipped behavior outside this canonical-index increment.
- cli/spec/lint#ac:unknown-rule-name-exits-2 — Existing shipped behavior outside this canonical-index increment.
- cli/spec/lint#ac:fix-idempotent — Existing shipped behavior outside this canonical-index increment.
- cli/spec/lint#ac:adherence-footer-fix-replaces-trailing-wrong-url — Existing shipped behavior outside this canonical-index increment.
- cli/spec/lint#ac:index-entries-fix-removes-phantom-row — Existing shipped behavior retained by this increment.
- cli/spec/lint#ac:index-entries-fix-inserts-orphan-row — Existing shipped behavior retained by this increment.
- cli/spec/lint#ac:index-entries-flags-orphan-child — Existing shipped behavior refined by the covered negative case.
- cli/spec/lint#ac:plan-index-sync-detects-and-fixes-row-drift — Existing shipped behavior outside this canonical-index increment.
- cli/spec/lint#ac:oq-section-missing-flagged — Existing shipped behavior outside this canonical-index increment.
- cli/spec/lint#ac:oq-section-legacy-heading-flagged-and-fixed — Existing shipped behavior outside this canonical-index increment.
- cli/spec/lint#ac:dogfood-version-bump-flags-stale-pin — Existing shipped behavior outside this canonical-index increment.
- cli/spec/lint#ac:missing-specscore-yaml-exits-3 — Existing shipped behavior outside this canonical-index increment.
- cli/spec/lint#ac:entity-and-property-rules-selectable — Existing shipped behavior outside this canonical-index increment.
- cli/spec/lint#ac:managed-section-fix-idempotent — Existing shipped behavior outside this canonical-index increment.
- cli/spec/lint#ac:consumer-path-multi-glob-parsed — Existing shipped behavior outside this canonical-index increment.
- cli/spec/lint#ac:fix-reports-only-changed-files — Existing shipped behavior outside this canonical-index increment.
- cli/spec/lint#ac:fix-json-envelope-only-under-fix — Existing shipped behavior outside this canonical-index increment.
- cli/spec/lint#ac:fix-text-summary-on-stderr — Existing shipped behavior outside this canonical-index increment.
- cli/spec/lint#ac:fix-report-needs-no-flag — Existing shipped behavior outside this canonical-index increment.
- cli/spec/lint#ac:implements-reference-requirement-liveness — Added by the later offline requirement-reference increment; outside this canonical-index scope.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
