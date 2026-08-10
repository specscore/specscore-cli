---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Lesson Recur

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/recur?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/recur?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/recur?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/recur?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Summary

`specscore lesson recur <slug> [--note <text>]` records that a lesson's process gap manifested again. For a canonical Lesson it appends exactly one immutable typed child Occurrence and derives the count without rewriting the README. For a compatibility flat Lesson it retains the historical `**Recurred:**` plus `## Recurrences` rewrite until explicit migration. Neither path changes `**Status:**`. A recurrence against a retired Lesson still records the evidence and exits `0`, but warns on stderr.

## Synopsis

```
specscore lesson recur <slug> [--note <text>] [--project <path>]
```

## Problem

`LESSONS-LEARNED.md` already has the vocabulary for this — its house rules permit "adding a `Recurred:` line when it happens again" as one of the only mutations allowed to a past entry — but nothing machine-readable ever recorded it: a recurrence was, at best, a sentence added to the `Status:` line by hand (`check-tags-before-tagging`'s entry literally reads "recurred twice in one session" in prose). Recording a recurrence is the strongest signal a lesson needs to graduate; a dedicated verb makes the count queryable (via `lesson list` / `lesson info`) instead of buried in a sentence.

## Behavior

### Recurrence recording

#### REQ: recur-increments-count

For a canonical Lesson, `lesson recur <slug>` MUST append one schema-valid occurrence child with a fresh UUID and UTC `Z` time, then derive the recurrence count from valid children. It MUST leave the README byte-identical. For a flat Lesson it MUST increment `**Recurred:** N`, inserting `1` after `**Status:**` when absent.

#### REQ: recur-appends-dated-entry

For a canonical Lesson the optional note becomes the bounded occurrence summary and no prose recurrence section is created. For a flat Lesson the verb MUST append a dated bullet — `- <YYYY-MM-DD>` plus the note when supplied — to `## Recurrences`, preserving all prior entries.

#### REQ: recur-does-not-change-status

The verb MUST NOT modify `**Status:**`. Acting on a recurrence (promoting the lesson up the ladder) is a separate, deliberate `change-status` call.

### Retired-lesson guard

#### REQ: recur-warns-on-retired-status

When the target lesson's `**Status:**` is a terminal disposition (`Withdrawn` or `Superseded` — a recognized status with no legal outgoing transition), the verb MUST print a warning to stderr naming the slug and its status, stating that a recurrence against a retired lesson suggests the retirement should be revisited. The verb MUST still record the recurrence (increment the count, append the entry) and exit `0` — the recurrence is itself evidence worth keeping, and this verb never blocks on it. A non-terminal status (`Recorded`, `Stated`, `Enforced`) MUST NOT produce this warning. An unrecognized or absent `**Status:**` value MUST NOT produce this warning either — status vocabulary validity is rule `L-002`'s concern, not this verb's.

### Slug resolution

#### REQ: recur-slug-resolution

`<slug>` MUST resolve canonical `spec/lessons/<slug>/README.md` first, then compatibility `spec/lessons/<slug>.md`, rejecting a duplicate layout. A slug that does not resolve MUST exit `3` (NotFound) naming the requested slug.

### Index sync

#### REQ: recur-index-sync

Canonical index recurrence metadata is derived from child Occurrences and the README/index remain byte-identical. After a flat rewrite, the verb MUST upsert only that Lesson's compatibility index row; it MUST NOT run a repository-wide fixer.

## Parameters / Flags

| Name | Required | Description |
|---|---|---|
| `slug` | Yes | Lesson slug — resolves canonical directory or compatibility flat layout. |
| `--note` | No | Free-form note describing this occurrence. |
| `--project` | No | Project root (autodetected). |

## Exit codes

| Code | Condition |
|---|---|
| `0` | Recurrence recorded; stdout `<slug>: recurred <N>\n`. |
| `2` | Missing/malformed `<slug>`. |
| `3` | No canonical or compatibility Lesson for the slug. |
| `10` | Unexpected occurrence, I/O, event, or narrow index-upsert failure. |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [lesson (CLI group)](../README.md) | Parent group. |
| [cli/lesson/list](../list/README.md), [cli/lesson/info](../info/README.md) | Both surface the `**Recurred:**` count this verb writes. |
| [cli/lesson/change-status](../change-status/README.md) | The deliberate follow-on action a recurrence signals — never triggered automatically by this verb. |

## Acceptance Criteria

### AC: recur-increments-count (verifies REQ:recur-increments-count)

**Given** canonical `spec/lessons/kinder-fake/README.md` with no child Occurrences
**When** the user runs `specscore lesson recur kinder-fake --note "happened again"`
**Then** the command exits `0` with stdout `kinder-fake: recurred 1\n`, appends one schema-valid child containing the bounded summary, and leaves the README byte-identical.

### AC: recur-appends-dated-entry (verifies REQ:recur-appends-dated-entry)

**Given** a compatibility flat Lesson with an existing `## Recurrences` section carrying one entry
**When** the user runs `specscore lesson recur <slug> --note "second occurrence"`
**Then** the existing entry is preserved and a new dated entry containing "second occurrence" is appended after it.

### AC: recur-does-not-change-status (verifies REQ:recur-does-not-change-status)

**Given** a lesson in `**Status:** Stated`
**When** the user runs `specscore lesson recur <slug>`
**Then** `**Status:**` remains `Stated` after the command exits `0`.

### AC: not-found-exits-3 (verifies REQ:recur-slug-resolution)

**Given** no lesson named `ghost`
**When** the user runs `specscore lesson recur ghost`
**Then** the command exits `3` naming `ghost`.

### AC: warns-on-withdrawn (verifies REQ:recur-warns-on-retired-status)

**Given** a lesson in `**Status:** Withdrawn`
**When** the user runs `specscore lesson recur <slug>`
**Then** the command exits `0`, the `**Recurred:**` count still increments, and stderr contains a warning naming the slug and `Withdrawn`.

### AC: warns-on-superseded (verifies REQ:recur-warns-on-retired-status)

**Given** a lesson in `**Status:** Superseded`
**When** the user runs `specscore lesson recur <slug>`
**Then** the command exits `0` and stderr contains a warning naming the slug and `Superseded`.

### AC: no-warning-on-active-statuses (verifies REQ:recur-warns-on-retired-status)

**Given** a lesson in `**Status:** Recorded`, `Stated`, or `Enforced`
**When** the user runs `specscore lesson recur <slug>`
**Then** stderr contains no warning.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
