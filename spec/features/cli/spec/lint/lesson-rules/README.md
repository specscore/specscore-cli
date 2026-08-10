---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Lesson Lint Rules

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/lesson-rules?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/lesson-rules?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/lesson-rules?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/lesson-rules?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Summary

Adds the Lesson lint family and underlying parser to `specscore spec lint`. Existing rules `L-001`–`L-004` retain compatibility. The extension adds directory-form discovery, flat-file compatibility, required tracking/evidence metadata, relation integrity, occurrence integrity, and exact index integrity. The concurrently reviewed SpecScore meta-format owns the final required fields and layout.

## Problem

A lint rule that checked section *content* would recreate the exact failure mode this artifact kind is designed to avoid: friction that makes agents stop recording lessons under time pressure. But a lint rule that checked *nothing* would let `specscore lesson new` scaffold silently drift, or let an entry with an `## Incident` and no `## Process gap` — the precise shape of a useless lesson — pass unnoticed. `L-001` threads that needle: presence-only, so a scaffold with four `<!-- TODO -->` prompts is always lint-clean, while an entry someone deleted a required heading from is not.

## Behavior

### Lesson artifact detection

#### REQ: lesson-detection-layouts

The rules MUST discover both legacy flat Lessons at `spec/lessons/<slug>.md` and directory Lessons at `spec/lessons/<slug>/README.md`. A slug present in both layouts is an error naming both paths; lint and read commands never choose one silently. Relocation is explicit, never an autofix side effect.

#### REQ: lesson-detection-title-prefix

A file is recognized as a Lesson when its first H1 heading matches `# Lesson: <title>` (exact prefix). A file at the same path without this prefix MUST be silently skipped.

### L-001 — required sections present

#### REQ: rule-l-001-registered

`L-001` MUST be registered at severity `error` and MUST execute as part of the default rule suite.

#### REQ: rule-l-001-presence-only

`L-001` MUST report a violation when any of the four required H2 sections (`Incident`, `Process gap`, `Check`, `Enforcement`) is absent from the body, naming every missing section. `L-001` MUST NOT inspect a present section's content, length, or wording — a section holding only a `<!-- TODO: ... -->` prompt is lint-clean.

### L-002 — status vocabulary

#### REQ: rule-l-002-status-values

`L-002` MUST report a violation when a present `**Status:**` value is not one of `Recorded`, `Stated`, `Enforced`, `Withdrawn`, `Superseded`. An absent `**Status:**` line emits nothing (presence is a different concern).

### L-003 / L-004 — lessons index

#### REQ: rule-l-003-completeness

`L-003` MUST report a violation naming every discovered lesson slug absent from `spec/lessons/README.md`'s `## Index` table. `--fix` MUST regenerate the table from the discovered Lesson set.

#### REQ: rule-l-004-row-sync

`L-004` MUST report a violation naming every index row whose `Status`/`Recurred`/`Date`/`Owner` cells do not match the corresponding Lesson file. `--fix` MUST regenerate drifted rows. A missing `spec/lessons/README.md` emits neither `L-003` nor `L-004` — that gap is the generic `readme-exists` rule's job.

### Required ownership, evidence, and graph integrity

#### REQ: extended-lesson-rules

The default suite MUST register error rules `L-005`–`L-009`:

| Rule | Requirement |
|---|---|
| L-005 | Required Status/Date/Owner/non-negative Recurred metadata is present and valid. |
| L-006 | Exactly one actionable tracking reference, or a structured reviewer-gate exception, is present. |
| L-007 | An Enforced Lesson carries a repository-relative enforcement-evidence path which resolves under project root. |
| L-008 | Supersession/human-confirmed relations resolve, are non-self, non-duplicated, symmetric where required, and acyclic where directed. |
| L-009 | Occurrences have unique IDs, RFC-3339 times, opaque JSON-object context, and resolvable parents. |

Lint checks syntax, identity, local paths, and graph consistency only. It does not claim a remote issue is open, a test ran, or prose is semantically duplicate.

#### REQ: generic-document-registry

Flat and directory Lesson READMEs MUST join the generic status-bearing document registry so shared format-field, adherence-footer, footer-format-mirror, and status-mirror checks apply. Occurrence JSON is not a Markdown document artifact and is excluded from those walkers.

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [spec lint](../../README.md) | Parent — registers `L-001`–`L-004` in the rule catalog. |
| [cli/lesson](../../../lesson/README.md) | Consumes this family: focused writers preflight configuration, update only their declared Lesson/index paths, then run lint read-only. Only an explicit `spec lint --fix` invocation runs fixers. |
| [cli/spec/lint/plan-rules](../plan-rules/README.md) | Closest structural sibling — `L-003`/`L-004` mirror `idea-index-completeness`/`idea-index-row-sync` rather than Plan's (Plan currently has no equivalent index-sync rule). |

## Acceptance Criteria

### AC: l001-missing-section-flagged (verifies REQ:rule-l-001-presence-only)

**Given** a Lesson body with `## Incident` but no `## Process gap`, `## Check`, or `## Enforcement`
**When** `specscore spec lint` runs
**Then** an `L-001` violation is reported naming all three missing sections.

### AC: l001-todo-placeholder-is-clean (verifies REQ:rule-l-001-presence-only)

**Given** a Lesson scaffolded by `lesson new`, whose four sections each hold only a `<!-- TODO: ... -->` prompt
**When** `specscore spec lint` runs
**Then** no `L-001` violation is reported.

### AC: l002-invalid-status-flagged (verifies REQ:rule-l-002-status-values)

**Given** a Lesson with `**Status:** Banana`
**When** `specscore spec lint` runs
**Then** an `L-002` violation is reported naming `Banana` and the canonical set.

### AC: l003-missing-row-fixed (verifies REQ:rule-l-003-completeness)

**Given** a Lesson not listed in `spec/lessons/README.md`
**When** `specscore spec lint --fix` runs
**Then** a row for the lesson is inserted and a subsequent lint run reports no `L-003` violation.

### AC: l004-drifted-row-fixed (verifies REQ:rule-l-004-row-sync)

**Given** an index row whose `Status` cell disagrees with the Lesson file's `**Status:**`
**When** `specscore spec lint --fix` runs
**Then** the row is rewritten to match and a subsequent lint run reports no `L-004` violation.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
