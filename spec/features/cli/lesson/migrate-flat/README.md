---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Lesson Flat Migration

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/migrate-flat?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/migrate-flat?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/migrate-flat?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/migrate-flat?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Summary

Migrate one committed flat Lesson into the canonical directory layout without losing its observation history or inventing enforcement evidence.

## Problem

Compatibility-era repositories can contain `spec/lessons/<slug>.md`. A canonical directory with the same slug must not coexist with that flat file, yet the directory is required for append-only child Occurrences. A manual move cannot prove which committed bytes were reviewed, can silently lose recurrence evidence, and can strand the Lesson between artifact, index, and event publication after a process interruption.

## Journey

The user selects one committed flat Lesson and reviewed classifications. Before any mutation, the command verifies the exact Git source and the complete bounded write set. The observable start result is either a durable prepared transaction or no changed byte.

The command publishes the canonical README, a provider Occurrence for the source Lesson, one Occurrence for every structured recurrence bullet, immutable provenance, and the exact index row. The observable middle result is that a second process can see one durable transaction marker and resume the same event UUID rather than starting competing work.

The command commits the prepared event and retires the marker only after artifact and index evidence verify. The observable end result is one lint-valid directory Lesson, no sibling flat file, one exact index row, and no unrelated changed file. A repeat invocation is read-only. If the process stops at any durable boundary, the same command resumes; if preflight fails, nothing is prepared or written.

## Behavior

```
specscore lesson migrate-flat <slug> --classification <value> [--classification <value>] [--control <text> --verification <text> --evidence <path>] [--format text|yaml|json]
```

#### REQ: immutable-source-and-bounded-publication

The flat bytes MUST match the current immutable Git revision. Provenance records repository, full revision, repository-relative path, commit time, byte count, whole-source hash, and exact observation ranges and hashes. Migration metadata MUST NOT copy raw source prose. Every final path MUST be preflighted before publication; the command MUST change only the selected Lesson layout, its migration manifest, its exact index row, its prepared event/outbox records, and its short-lived transaction marker.

#### REQ: every-structured-observation-survives

The flat source observation MUST become exactly one deterministic provider Occurrence. Every structured `## Recurrences` bullet MUST become exactly one further deterministic Occurrence. The command MUST reject a mismatch between `**Recurred:**` and the structured bullet count instead of manufacturing or dropping history.

#### REQ: enforcement-is-never-fabricated

Recorded, Stated, Withdrawn, and Superseded source statuses remain provenance-backed statuses. An Enforced source MUST supply reviewed, nonempty `--control`, `--verification`, and `--evidence` values that satisfy the canonical Lesson contract. Git provenance or a source hash MUST NOT be presented as proof that a control works.

#### REQ: one-resumable-transaction

Canonical artifacts, manifest, exact index row, and prepared event/outbox MUST share one deterministic transaction identity. A durable marker MUST remain until all four are verifiable. A retry after source removal MUST reuse the marker's classifications, source identity, occurrence IDs, timestamp, and event UUID. Malformed, conflicting, or incomplete state MUST fail visibly without overwriting it.

#### REQ: explicit-migration-only

Readers and lint MUST continue to support a flat Lesson until this command runs. `lesson new`, `spec lint --fix`, and unrelated scaffold commands MUST NOT migrate it implicitly. A new directory Lesson MUST be refused while a sibling flat file exists.

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [Lesson](../README.md) | Establishes the canonical directory and flat compatibility layouts. |
| [Occurrence](../occurrence/README.md) | Validates provider and recurrence child JSON. |
| [Lesson lint rules](../../spec/lint/lesson-rules/README.md) | Verifies the canonical README, children, and exact index row after publication. |
| [Event](../../event/README.md) | Holds the prepared event until the artifact and index evidence are durable. |

## Acceptance Criteria

### AC: migration-is-bounded-and-lint-clean

**Given** a committed Recorded flat Lesson with one structured recurrence and a configured classification
**When** the user runs `lesson migrate-flat`
**Then** one canonical README, two Occurrences, immutable provenance, one exact index row, and one committed event exist; the flat file is absent; focused lint is clean; and every unrelated file is byte-identical.

### AC: enforced-source-needs-real-reviewed-evidence

**Given** a committed Enforced flat Lesson
**When** any of control, verification, or evidence is omitted
**Then** preflight fails without writing; when all three are reviewed and valid, the canonical Lesson uses those exact values and contains no fabricated Git or hash verification.

### AC: every-durable-boundary-resumes

**Given** the process stops after artifact publication, index upsert, or event commit
**When** the same command runs again
**Then** it resumes the same marker and event UUID, verifies or completes each remaining phase exactly once, removes the marker only after success, and never duplicates the index row or an Occurrence.

### AC: repeated-complete-migration-is-read-only

**Given** a verified completed migration
**When** the same command runs again
**Then** it reports `already_migrated`, creates no event, and leaves the repository byte-identical.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
