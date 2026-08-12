---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Lesson Legacy Import

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/import-legacy?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/import-legacy?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/import-legacy?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/import-legacy?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Summary

Inventory historical lessons losslessly, then apply only reviewed compact Lessons and occurrences.

## Problem

Historical append-only logs contain valuable evidence but have mixed headings, duplicate IDs, prose status suffixes, and cross-references that cannot safely become canonical slugs by guesswork. A lossy importer would destroy the evidence the new model is meant to preserve.

## Behavior

```
specscore lesson import-legacy --source <path> [--mapping <path>] --dry-run|--apply [--format json|yaml|text]
```

The inventory preserves every recognized heading and inline recurrence-marker identity as immutable source repository/path/revision plus exact byte ranges and hashes. `--dry-run` writes nothing and reports the whole-source digest, heading and marker counts, an ordered entry-projection digest binding every key/parent/range/hash, proposed slugs, raw statuses, duplicate-ID collisions, aggregate-marker ambiguity, and unmatched candidates. It does not infer an exact incident count from prose such as “at least twice”.

`--apply` requires the authoritative bytes to equal a committed immutable Git source and requires one reviewed mapping row per inventory row. Each row explicitly chooses `new`, `occurrence`, or `manual`; any remaining `manual` row fails the complete apply before the prepared event or artifact writes. A `new` row MUST supply a unique slug, `status: Recorded`, at least one classification present in the target repository's non-empty `lessons.classifications` vocabulary, and nonempty safe compact `lesson` and `process_gap` text. The source heading title is the default title when the mapping omits one. Placeholder, unsafe, unconfigured, duplicate, unknown, or accepted-but-ignored fields fail preflight.

A `new` row creates the canonical Lesson and exactly one deterministic provider Occurrence for that source observation. An `occurrence` row adds exactly one deterministic child to a canonical Lesson. One aggregate marker row remains one ambiguity-tagged evidence record. Source status remains in the immutable manifest and apply reports the conservative `Recorded` status decision; this importer does not create `Enforced` Lessons or fabricate control, verification, or evidence.

Committed migration artifacts MUST NOT contain raw/base64 legacy prose. The manifest retains source identity, raw status, exact ranges, and block hashes, while compact reviewed text is allowed because it is the current rule rather than a copy of the archive. All targets and evidence paths are collision-preflighted. New Lesson publication reserves its directory, durably records a deterministic `.legacy-import-owner` marker, and exclusively links immutable children. Once any path is visible, failure retains the complete/partial state as uncertain; the importer never performs a verify-then-delete rollback that could erase a concurrent foreign child. Re-running an approved apply reconciles its marker, README, occurrence directory, manifest, and deterministic provider Occurrence byte-identically without changing foreign children or creating a duplicate.

## Acceptance Criteria

### AC: dry-run-is-lossless-and-write-free

**Given** a legacy log containing duplicate IDs, a timestamp ID, and prose status variants
**When** `import-legacy --dry-run --format json` runs
**Then** no destination/manifest is written; each entry reports source range and raw status; ambiguous rows are reported, not guessed.

### AC: new-mapping-is-usable-and-configured

**Given** a `new` mapping row missing compact Lesson text, Process Gap text, explicit Recorded status, or a configured classification
**When** apply preflight runs
**Then** it fails before preparing an event or writing an artifact; a valid row emits no scaffold TODO and reports the source-to-Recorded status decision.

### AC: reviewed-apply-is-idempotent

**Given** a human-completed mapping and unchanged source digest
**When** `import-legacy --apply` runs twice
**Then** the first creates mapped artifacts without changing the source and the second creates none, reports the same mappings, and exits `0`.

### AC: publication-failure-is-transactional

**Given** an exclusive README or manifest link succeeds and its following directory sync fails
**When** apply returns the durability error
**Then** no published path is deleted, the error is classified uncertain, every concurrent foreign child remains byte-identical, and a clean retry finishes the same deterministic import without a second provider Occurrence. (The AC identifier is retained for traceability to the superseded destructive rollback behavior.)

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
