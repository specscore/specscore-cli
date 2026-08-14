---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Lesson New

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/new?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/new?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/new?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/new?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Summary

`specscore lesson new <slug>` creates one canonical compact Lesson at `spec/lessons/<slug>/README.md`, a Git-preserved empty `occurrences/` store, the exact index row, and its durable created event.

## Synopsis

```
specscore lesson new <slug> [--title <text>] [--owner <id>] [--force] [--project <path>]
```

## Problem

Recording a process gap must remain quick, but a mutating scaffold cannot silently choose an unconfigured classification, retain both flat and directory layouts, run a repository-wide fixer, or leave a partial Lesson whose index/event says something else. The canonical README stays compact so agents read the current rule without loading occurrence history.

## Journey

The user configures the repository's Lesson classification vocabulary and chooses a slug. Before any write, the command validates configuration, content policy, both layout targets, and the complete bounded write set. The observable start result is either zero changed bytes or one prepared event naming the intended mutation.

The command publishes the directory scaffold and exact index row, then runs lint read-only and durably fences both files and parent directories. The observable middle result is one canonical Lesson and no changed unrelated spec file. It commits and attempts the prepared event only after the artifact is valid and durable. The observable end result is a lint-clean Lesson plus independently replayable event delivery. A pre-publication collision or validation failure changes no bytes. Any post-publication failure retains the complete visible state and prepared event for explicit recovery; it never restores a whole-file/tree snapshot that could erase concurrent work.

## Behavior

### Configuration and slug preflight

#### REQ: config-before-write

The project MUST have a valid `specscore.yaml` with a nonempty, unique `lessons.classifications` vocabulary before the command creates any path. The slug MUST be lowercase, hyphen-separated, URL-safe, and contain no `/`. Invalid configuration or input MUST leave the project byte-identical.

#### REQ: one-layout-only

The command MUST refuse a sibling compatibility file at `spec/lessons/<slug>.md`; the user must run explicit flat migration first. An existing canonical README MUST exit Conflict unless `--force` is supplied.

### Canonical compact scaffold

#### REQ: canonical-directory-scaffold

The command MUST create `spec/lessons/<slug>/README.md` and `spec/lessons/<slug>/occurrences/`. An otherwise empty occurrence store MUST contain an empty `.gitkeep` marker so a Git checkout retains the required directory; the marker is not an occurrence. The README MUST carry format/status frontmatter, Recorded lifecycle metadata, configured classifications, relation and immutable-provenance fields, concise `## Lesson` and `## Process Gap` prompts, a `## Tracking` line with the exact published occurrence-schema URL, structured Enforcement fields, `## Open Questions`, and the matching adherence footer.

#### REQ: bounded-index-upsert

The command MAY materialize only missing `spec/README.md` and `spec/lessons/README.md` ancestors, then MUST upsert only the selected Lesson's canonical index row. It MUST run lint read-only for the owned Lesson, occurrence store, and exact index finding. It MUST NOT invoke `spec lint --fix`; in particular it MUST NOT rewrite an unrelated `## Outstanding Questions` heading.

#### REQ: durable-created-event

Before artifact publication, the command MUST prepare `lesson.created` with the complete nonempty configured subscriber set. The bounded transaction covers the requested Lesson paths, declared ancestor/index paths, and prepared event/outbox. After artifact validation it commits the event and attempts every subscriber independently. An explicitly empty subscriber configuration opts out without creating a recipientless ledger.

#### REQ: rollback-owned-write-set

Any failure proven to precede publication MUST leave the declared write set byte-identical. After a path becomes visible, the command MUST NOT delete or restore an artifact/index snapshot without an atomic ownership proof. It MUST retain the visible state and prepared event as uncertain for explicit recovery, preserving every unrelated or concurrently added file/index row. Exact-row index writes MUST serialize through the project-private Lesson-index writer lock. Event commit MUST follow successful file and parent-directory durability fences for the Lesson, occurrence store, and both declared indexes.

## Parameters

| Name | Required | Description |
|---|---|---|
| `slug` | Yes | Canonical directory name. |
| `--title` | No | Lesson title; defaults from the slug. |
| `--owner` | No | Owner/author; defaults from the local user. |
| `--force` | No | Replace the canonical README while preserving its occurrence directory. |
| `--project` | No | Project root; otherwise autodetected. |

## Exit Codes

| Code | Condition |
|---|---|
| `0` | The canonical Lesson/index/event transaction completed, or durable subscriber failures remain independently pending and are reported as warnings. |
| `1` | A flat sibling or unforced canonical target already exists. |
| `2` | The slug or reviewed scaffold content is invalid. |
| `3` | The project root is not found. |
| `10` | Configuration, I/O, index, focused lint, or event preparation/commit failed. |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [Lesson](../README.md) | Defines canonical Lesson identity and progressive disclosure. |
| [Lesson lint rules](../../spec/lint/lesson-rules/README.md) | Supplies the narrow index upsert and read-only verification rules. |
| [Flat migration](../migrate-flat/README.md) | Is the only path from a sibling compatibility file to this canonical layout. |
| [Events](../../event/README.md) | Owns prepared delivery, independent acknowledgement, replay, and reconciliation. |

## Acceptance Criteria

### AC: configuration-preflight-is-zero-write

**Given** missing/invalid configuration or an empty classification vocabulary
**When** `lesson new` runs
**Then** it exits nonzero before an event, index, directory, or file is created.

### AC: scaffolded-lesson-is-canonical-and-lint-clean

**Given** a lint-clean configured project
**When** `lesson new verify-before-merge` runs
**Then** the canonical README and Git-preserved empty occurrence store exist, the exact canonical index row exists, focused and whole-project read-only lint report no new error, and no sibling flat file exists.

### AC: unrelated-files-remain-byte-identical

**Given** unrelated specs including an obsolete `## Outstanding Questions` heading
**When** `lesson new` succeeds or an owned-phase failure is injected
**Then** every unrelated byte is unchanged and only explicit `spec lint --fix` can perform the heading migration.

### AC: created-event-is-durable

**Given** a nonempty subscriber configuration
**When** creation succeeds
**Then** one immutable `lesson.created` ledger names the slug and complete canonical subscriber set, its decision is committed, and each failed sink remains independently replayable.

### AC: flat-sibling-is-refused

**Given** `spec/lessons/<slug>.md`
**When** `lesson new <slug>` runs, even with `--force`
**Then** it exits Conflict before any write and directs the user to explicit flat migration.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
