# Idea: index-entries autofix for spec lint --fix

**Status:** Specified
**Date:** 2026-05-18
**Owner:** alexander.trakhimenok
**Promotes To:** cli/spec/lint
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we let `specscore spec lint --fix` repair drift between feature index READMEs and their child directories so authors no longer have to hand-edit index tables when adding or removing features?

## Context

The `index-entries` lint rule (see [`cli/spec/lint`](../features/cli/spec/lint/README.md#req-index-entries-bidirectional)) flags drift in both directions: a parent index that links a non-existent child, AND a child directory that is not linked from its parent index. Without `--fix`, every drift requires the author to hand-edit the parent README — exactly the kind of mechanical edit a linter should own.

The bidirectional check landed alongside a fix for a silent-pass bug where orphan child directories were never flagged. The `## Behavior > Features index synchronization` section of `cli/spec/lint` defines the structural contract; this Idea closes the loop by making the rule self-healing.

Prior art in the same package: `adherence-footer` already implements `--fix` (rewrites trailing footer URLs and appends missing footers), constrained by the `fix-is-safe-subset` REQ that forbids autofixes "that require semantic interpretation of document intent beyond structural conventions." That REQ is the design constraint this Idea must respect.

A second piece of prior art turned out to matter even more, and was missed during initial scoping: `specscore feature new` already writes rows into both the root features index and nested `## Contents` tables via the public helpers `feature.UpdateFeatureIndex` and `feature.UpdateParentContents`. Status comes from `feature.ParseFeatureStatus` (an existing structural parser). Kind and Description use a codified placeholder convention — em-dash for Kind, `TODO: Add description.` for Description — that the project has shipped with for as long as the row format has existed. The autofix can reuse all of it.

## Recommended Direction

**Two phases, both ship.** Phase 1 (delete) is mechanically trivial. Phase 2 (insert) is byte-identical to what user-driven scaffolding already produces.

**Phase 1 — delete phantom rows.** When `index-entries` reports `Index mentions non-existent directory: <name>`, `--fix` removes the offending row from the parent README's index table. Deleting a row that points at nothing is unambiguous: there is no child README to read, so the row carries no live information. Implementation locates the table row whose link target ends in the phantom dirname and drops that single line; table delimiters and every other row are preserved byte-for-byte.

**Phase 2 — insert rows for orphan children.** When `index-entries` reports `Child directory not listed in index: <name>`, `--fix` appends a row that links the missing child. The row shape matches what `feature new` writes:

- At the root features index, a 4-cell row: link | Status | em-dash | TODO placeholder. Status is parsed from the child README's `**Status:**` header.
- At a nested feature index, a 2-cell row inside the `## Contents` table. The `## Contents` block is created if absent.

The cli/spec/lint Feature carries the precise REQs: `index-entries-fix-deletes-phantom-rows` for Phase 1 and `index-entries-fix-inserts-orphan-rows` for Phase 2. Both satisfy `fix-is-safe-subset` because every cell either flows from a parsed field or matches a project-codified placeholder; the fixer never invents content the user has authority over.

Phase 1 runs before Phase 2 inside the same `--fix` pass so the insertion phase reads a phantom-free index. Idempotency holds: pass 2 finds no phantom rows to delete and no unlinked children to insert.

## Alternatives Considered

**Skip the rule entirely from `--fix` and keep it forever report-only.** Defensible — `index-entries` violations are loud and quick to fix by hand. Rejected because the autofix output is byte-identical to user-driven `feature new` scaffolding; refusing to apply it during lint is an arbitrary distinction.

**Defer Phase 2 until a richer feature-metadata parser exists.** Argued during initial scoping that Phase 2 needed a new parser that could read Status, Kind, and a Description from a child README without re-implementing structural rules. Rejected on closer inspection of the codebase: `feature.ParseFeatureStatus` already exists, Kind and Description have no per-feature source-of-truth and use codified placeholders in `feature new` today. There is no parser to wait for.

**Append fully-invented placeholder rows.** Inserting a row like `[name]/README.md | TBD | TBD | TBD` with no parsed values. Rejected because it conflates "the linter knows the directory exists" with "the linter is guessing what the feature is." The shipped approach parses Status (a real structural field) and uses the same hand-maintained placeholders for Kind/Description that the project already accepts as visibly under-filled — inviting the author to populate them rather than masking missing intent.

## MVP Scope

`specscore spec lint --fix` keeps every feature index in sync with the filesystem in one pass: phantom rows are deleted, orphan children get a row appended whose shape matches `feature new`. The fix is idempotent and limits its mutations to the rows it owns.

## Not Doing (and Why)

- Rewriting non-table index formats (bullet lists, free-form prose Contents sections) — out of scope until a canonical index format is settled. The fixer assumes pipe-table indices throughout.
- Filling Kind or Description from anything other than the codified placeholders — that would be semantic interpretation per `fix-is-safe-subset`.
- Cross-repository index sync — a feature defined in another repo and linked from this one is out of scope; `index-entries` only walks the local `spec/features/` tree.

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | Every existing index link sits on a single line within a Markdown table (no multi-line continuations, no link-reference style). | Grep every `spec/features/**/README.md` in this repo and upstream; confirm 100% sit on a single line. Verified during Phase 1 implementation; no exceptions found. |
| Should-be-true | Inserting a row into the root features-index or a nested `## Contents` table never breaks adjacent rules (heading levels, OQ section, adherence footer, feature-index-row-sync). | Test fixtures that lint clean before, apply `--fix`, lint clean after. Verified via `TestIndexEntries_FixInsertsOrphanRowAtRoot` and `TestIndexEntries_FixInsertsOrphanRowNested`. |
| Might-be-true | The em-dash Kind / TODO Description placeholders remain the project-codified convention. If a future change to `feature new` revises that convention, the autofix MUST follow in lockstep so its output stays byte-identical to user-driven scaffolding. | Track changes to `feature.UpdateFeatureIndex` / `feature.UpdateParentContents`; the autofix calls them directly so any convention change propagates automatically. |


## SpecScore Integration

- **New Features this would create:** none — extends the existing `cli/spec/lint` Feature with two new REQs (`index-entries-fix-deletes-phantom-rows`, `index-entries-fix-inserts-orphan-rows`) and matching ACs.
- **Existing Features affected:** [`cli/spec/lint`](../features/cli/spec/lint/README.md) (adds autofix support to the `index-entries` rule).
- **Dependencies:** `feature.ParseFeatureStatus`, `feature.UpdateFeatureIndex`, `feature.UpdateParentContents` — all already public in `pkg/feature/`.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/idea-specification*
