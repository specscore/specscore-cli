# Idea: Audit lint rules for parallel bugs

**Status:** Archived
**Date:** 2026-05-18
**Owner:** alexander.trakhimenok
**Promotes To:** —
**Supersedes:** —
**Related Ideas:** —
**Archive Reason:** Audit completed — 106 rules audited, findings table committed to this document.

## Problem Statement

How might we systematically check every registered lint rule for the same classes of bug we just found in index-entries — silent-pass directionality gaps, missing autofix support where mechanically safe, and same-pass idempotency violations — without doing a 30-rule manual code review every time we ship a fix?

## Context

While shipping the `index-entries-autofix` Idea, three distinct classes of bug surfaced in code that had been live and presumed-working for some time:

1. **Silent-pass directionality gap.** `index-entries` was originally one-directional — it flagged phantom links in the index but not orphan child directories on disk. The asymmetry compiled, lint passed, and contributors who added a feature dir without updating the parent index got zero signal.
2. **`FeatureSourceIdeas` only walked top-level feature dirs.** Nested feature READMEs with `**Source Ideas:**` headers were invisible to the lint engine, so every nested-feature promotion silently no-op'd. `lifecycle-verbs-for-idea-and-feature` had been stuck at Approved for that reason despite three nested features referencing it.
3. **Same-pass idempotency violation in `idea-sync-lint-strict`.** The rule rewrote Idea files on disk in step 6 of the chain, but step 7 (`ideaIndexRules`) read a stale in-memory `parsed` snapshot, so `--fix` needed two passes to fully promote an Idea. The `fix-is-idempotent` REQ on cli/spec/lint says pass 2 must be a no-op.

Each was a one-line or one-loop fix once found, but each had been hiding because nobody had looked. The lint package today registers ~30 rules. We have no reason to believe `index-entries` was special — the same bug classes could be lurking anywhere.

Prior art for the audit approach: the `cli/spec/lint` feature already documents the REQs each rule must satisfy (severity, exit code, idempotency, bidirectional checks where applicable). The audit is checking each registered rule against that contract, not inventing new contracts.

## Recommended Direction

**Spreadsheet, not subsystem.** Audit by hand using a fixed checklist; do not build infrastructure to automate the audit. The audit happens once per rule and the time saved by tooling does not pay back the maintenance.

For each rule registered in `pkg/lint/lint.go` `allRuleNames`:

1. **Check direction.** Does the rule have an inherent direction (X must agree with Y)? If yes, does the check fire both X→Y and Y→X? Write the answer.
2. **Autofix mechanical safety.** Is there a class of violations the rule reports that would be unambiguous to fix? If yes, does the rule implement `fixer`? If no fixer, is the abstention deliberate (per `fix-is-safe-subset`) or accidental?
3. **Same-pass idempotency.** Does the rule mutate state (on disk or in-memory) that a later rule in the chain reads? If yes, is the downstream rule's read refreshed after the upstream's write?
4. **Test coverage.** Does the rule have tests for the directions / fix paths / idempotency it claims to support?

The output is a 30-row Markdown table committed to the repo at `spec/ideas/audit-lint-rules-for-parallel-bugs.md` (this file, updated post-audit) with one row per rule, columns: rule name, direction-gap?, fix-gap?, idempotency-risk?, coverage-gap?, follow-up Idea (if any).

Each cell with a "yes" promotes to a new follow-up Idea (or, if the fix is small, lands directly on a branch with this Idea cited in the commit message).

## Alternatives Considered

**Build a meta-linter that lints the linters.** Tempting — code that walks `allRuleNames` and reports rules missing `fix()` implementations, rules whose check function returns no violations on any fixture, etc. Rejected because the heuristics are noisy (a rule legitimately may not need `fix()`) and the maintenance is real. Manual audit with a checklist catches the same gaps with a fraction of the code.

**Wait for bugs to surface organically.** Defensible — each of the three bugs above was found because a real workflow hit it. We could simply react. Rejected because the cost of an undetected lint gap is months of silent rot (orphan features, stuck Ideas, mis-synced indices) that compound; the audit is a few hours and front-loads the discovery.

**Generate stress-test fixtures for every rule.** Property-based testing or fuzz-style coverage. Rejected as a poor fit — lint rules check structural invariants on small fixed-format documents, not search spaces with adversarial inputs. The cost-to-coverage ratio is bad.

## MVP Scope

Hand-walk every rule in `pkg/lint/lint.go` `allRuleNames`, fill in the 4-column checklist for each, write the output table back into this Idea's body, and file one follow-up Idea per "yes" cell. Single deliverable: the populated table. Timeboxed: two focused sessions of ~90 minutes each.

## Audit Results

Audit completed 2026-05-27. 106 rules across 7 categories. Below is the consolidated findings table — only rules with at least one "yes" cell are listed. Rules with all "no" cells are clean and omitted for brevity.

### Findings

| Rule | Direction gap? | Fix gap? | Idempotency risk? | Coverage gap? | Notes |
|------|---------------|----------|-------------------|---------------|-------|
| **readme-exists** | no | yes — could scaffold blank README | no | no | |
| **oq-not-empty** | no | yes — could insert placeholder bullet | no | no | |
| **heading-levels** | n/a | n/a | n/a | n/a | Stub (`TODO: Phase 2`) — dead code |
| **feature-ref-syntax** | n/a | n/a | n/a | n/a | Stub — dead code |
| **internal-links** | n/a | n/a | n/a | n/a | Stub — dead code |
| **forward-refs** | n/a | n/a | n/a | n/a | Stub — dead code |
| **code-annotations** | n/a | n/a | n/a | n/a | Stub — dead code |
| **plan-hierarchy** | no | yes — no fixer | no | no | |
| **plan-roi-metadata** | no | yes — no fixer | no | no | |
| **dogfood-version-bump** | no | yes — intentionally no autofix | no | no | By design: bumping is a human action |
| **sidekick-seed** | no | yes — no fixer | no | no | |
| **feature-index-row-sync** | no | no | no | yes — orphaned row path untested | |
| **idea-location** | no | yes — could auto-move | no | no | |
| **idea-slug-format** | no | yes — rename cascades | no | no | |
| **idea-single-file** | no | yes — could flatten | no | no | |
| **idea-title-format** | no | yes — could rewrite | no | no | |
| **idea-header-fields** | no | yes — could reorder | no | no | |
| **idea-id-is-slug** | no | yes — could strip `Id:` line | no | no | |
| **idea-supersedes-target-archived** | yes — no reverse check for orphaned archived ideas | no | no | yes — "target not found" path untested | |
| **idea-related-ideas-target-exists** | yes — no symmetry check (A→B doesn't verify B→A) | no | no | no | May be by design |
| **idea-feature-cross-reference** | yes — only checks feature→idea; not idea→feature `Promotes To` | no | no | no | Partially covered by sync-lint-strict |
| **entity-title-format** | no | no (has autofix) | no | yes — autofix path untested | |
| **entity-properties-list-shape** | no | no | no | yes — only 1 of 3 sub-checks tested | |
| **entity-required-sections** | no | yes — could scaffold stubs | no | no | Unlike property analog, no order check |
| **entity-location** | no | yes — could auto-move | no | no | |
| **entity-slug-format** | no | yes — rename cascades | no | no | |
| **property-location** | no | yes — could auto-move | no | no | |
| **property-slug-format** | no | yes — rename cascades | no | no | |
| **property-required-sections** | no | yes — could scaffold stubs | no | no | |
| **D-title-format** | no | yes — could insert prefix | no | no | |
| **D-header-fields** | no | yes — could reorder/insert | no | no | |
| **D-required-sections** | no | yes — could scaffold | no | no | |
| **D-observed-consequences-placeholder** | no | yes — could insert placeholder | no | no | |
| **D-number-assignment** | no | no | no | yes — backfill branch unreachable (dead code) | `isBackfill` is always false because `allNumbers` always contains every discovered decision |
| **D-supersedes-bidirectional** | **yes — only fires from superseding side** | no | no | yes — no test for orphan `Superseded By` | Stale `Superseded By` with no matching `Supersedes` goes undetected |
| **D-immutability-once-accepted** | **partial — skips check when status changes from Accepted** | no | no | yes — no test for simultaneous status change + frozen section edit | Transitioning away from Accepted allows editing frozen sections |
| **DI-completeness** | **yes — active index only** | yes — no fix for archived index | no | no | No completeness check for archived decisions index |
| **DI-status-excludes-archived** | **partial — one direction only** | no | no | yes — no test for Proposed/Accepted in archived index | |
| **I-015** | no | yes — intentionally no autofix | no | no | By design: rewriting would destroy authorial intent |

### Additional Structural Findings

1. **Six idea rules missing from `allRuleNames`**: `idea-type-values`, `idea-type-title-consistency`, `idea-targets-required`, `idea-targets-exists`, `idea-change-request-location`, `idea-phase-non-empty` are registered in `ideaRuleNames` but absent from `allRuleNames` in `lint.go`. They fire correctly but cannot be targeted with `--rules` or `--ignore`.

2. **`idea-index-row-sync` is an unregistered violation name**: emitted by `idea_index.go` but not in any rule registry. Same targeting issue as above.

3. **Five stub rules are dead code**: `heading-levels`, `feature-ref-syntax`, `internal-links`, `forward-refs`, `code-annotations` are registered but always return empty violations.

### Summary by Category

| Category | Rules | Direction gaps | Fix gaps | Idempotency risks | Coverage gaps |
|----------|-------|---------------|----------|-------------------|---------------|
| General/Feature | 16 | 0 | 7 (2 intentional) | 0 | 1 |
| Idea | 21 | 3 (partial) | 7 | 0 | 1 |
| Entity | 15 | 0 | 4 | 0 | 2 |
| Property | 10 | 0 | 3 | 0 | 0 |
| Decision | 17 | 2 | 4 | 0 | 3 |
| Decision Index | 6 | 2 | 1 | 0 | 1 |
| Issue | 15 | 0 | 1 (intentional) | 0 | 0 |
| **Total** | **106** | **7** | **27 (3 intentional)** | **0** | **8** |

### Priority Follow-ups

**Direction gaps (bugs):**
- `D-supersedes-bidirectional` — real gap: orphan `Superseded By` undetected
- `D-immutability-once-accepted` — frozen sections editable during status transitions
- `DI-completeness` — archived index not checked
- `DI-status-excludes-archived` — one-direction only

**Dead code:**
- 5 stub rules should be removed or implemented
- `D-number-assignment` backfill branch is unreachable
- 7 rule names missing from `allRuleNames` registration

## Not Doing (and Why)

- Rewriting any individual rule preemptively — this Idea covers detection and prioritization only. Each rule that warrants a fix promotes to its own follow-up.
- Building a meta-linter that lints the linters — scope creep; manual audit with a checklist is the MVP.

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | Every registered rule's check function is reachable from a fixture in the lint test suite (i.e., we are auditing live code, not dead code). | Cross-reference `allRuleNames` against test file mentions; any rule with zero references is dead. |
| Should-be-true | The four checklist axes (direction, fix safety, idempotency, coverage) catch every class of bug we found in this session. No fifth class exists that the checklist would miss. | Apply the checklist to the three known bugs retroactively; each must map cleanly to one of the four axes. |
| Might-be-true | The audit will find at least one bug per ~10 rules on average. If we find zero across 30 rules, the checklist is too coarse and needs revision. | Count findings at the halfway point (15 rules); recalibrate if the rate is materially lower than expected. |


## SpecScore Integration

- **New Features this would create:** none directly. Findings may promote into follow-up Ideas that themselves promote to existing or new lint Feature REQs.
- **Existing Features affected:** [`cli/spec/lint`](../features/cli/spec/lint/README.md) — the audit's contract is whatever this Feature documents about rule behavior.
- **Dependencies:** none. The audit reads code that already exists.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/idea-specification*
