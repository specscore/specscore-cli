---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Lesson Legacy Import

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/import-legacy?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/import-legacy?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/import-legacy?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/import-legacy?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Summary

Losslessly import historical prose lessons into canonical artifacts.

## Problem

Historical append-only logs contain valuable evidence but have mixed headings, duplicate IDs, prose status suffixes, and cross-references that cannot safely become canonical slugs by guesswork. A lossy importer would destroy the evidence the new model is meant to preserve.

## Behavior

```
specscore lesson import-legacy --source <path> [--mapping <path>] --dry-run|--apply [--format json|yaml|text]
```

The importer preserves every source heading, byte range, body, link, and raw status in a deterministic import manifest, proposing one canonical Lesson/occurrence tree per entry. `--dry-run` writes nothing and reports source digest, proposed slug, legacy IDs, parsed status, and warnings. `--apply` requires the reviewed mapping and unchanged source digest; it creates only missing targets and manifest. It never edits, deletes, renumbers, or archives the source log.

Slug collisions, duplicate legacy IDs, ambiguous headings, malformed dates, and status prose that cannot map exactly require an explicit mapping choice. The importer may normalize the canonical token only after preserving original text; it MUST NOT infer `Enforced`, merge similar entries, fabricate tracking/evidence, or drop paragraphs. Re-running an approved apply is idempotent.

## Acceptance Criteria

### AC: dry-run-is-lossless-and-write-free

**Given** a legacy log containing duplicate IDs, a timestamp ID, and prose status variants
**When** `import-legacy --dry-run --format json` runs
**Then** no destination/manifest is written; each entry reports source range and raw status; ambiguous rows are reported, not guessed.

### AC: reviewed-apply-is-idempotent

**Given** a human-completed mapping and unchanged source digest
**When** `import-legacy --apply` runs twice
**Then** the first creates mapped artifacts without changing the source and the second creates none, reports the same mappings, and exits `0`.

## Open Questions

- Whether the manifest is repository-wide or beside each Lesson is owned by the meta-format; it must be queryable without reparsing the legacy source.

---
*This document follows the https://specscore.md/feature-specification*
