---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: Lesson Recur

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/recur?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/recur?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/recur?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/recur?op=request-change) |
**Status:** Draft
**Source Ideas:** —

## Summary

`specscore lesson recur <slug> [--note <text>]` records that a lesson's process gap manifested again: it increments the lesson's `**Recurred:** N` header count and appends a dated entry (with the optional note) to a `## Recurrences` section. It does NOT change `**Status:**` — a recurrence is a signal that a lesson needs to graduate, not a graduation itself.

## Synopsis

```
specscore lesson recur <slug> [--note <text>] [--project <path>]
```

## Problem

`LESSONS-LEARNED.md` already has the vocabulary for this — its house rules permit "adding a `Recurred:` line when it happens again" as one of the only mutations allowed to a past entry — but nothing machine-readable ever recorded it: a recurrence was, at best, a sentence added to the `Status:` line by hand (`check-tags-before-tagging`'s entry literally reads "recurred twice in one session" in prose). Recording a recurrence is the strongest signal a lesson needs to graduate; a dedicated verb makes the count queryable (via `lesson list` / `lesson info`) instead of buried in a sentence.

## Behavior

### Recurrence recording

#### REQ: recur-increments-count

`lesson recur <slug>` MUST increment the lesson's `**Recurred:** N` header field by one. When the field is absent (a lesson predating it), the verb MUST insert `**Recurred:** 1` immediately after `**Status:**`.

#### REQ: recur-appends-dated-entry

The verb MUST append a dated bullet — `- <YYYY-MM-DD>` plus the `--note` text when supplied — to a `## Recurrences` section, creating the section (immediately before the adherence footer, or at end-of-file absent one) when it does not already exist. An existing section's prior entries MUST be preserved; the new entry is appended, never inserted out of order.

#### REQ: recur-does-not-change-status

The verb MUST NOT modify `**Status:**`. Acting on a recurrence (promoting the lesson up the ladder) is a separate, deliberate `change-status` call.

### Slug resolution

#### REQ: recur-slug-resolution

`<slug>` MUST resolve to `spec/lessons/<slug>.md`. A slug that does not resolve MUST exit `3` (NotFound) naming the requested slug.

### Index sync

#### REQ: recur-index-sync

After the rewrite, the verb MUST run `specscore spec lint --fix` so the lessons index's `Recurred` column stays in sync.

## Parameters / Flags

| Name | Required | Description |
|---|---|---|
| `slug` | Yes | Lesson slug — resolves to `spec/lessons/<slug>.md`. |
| `--note` | No | Free-form note describing this occurrence. |
| `--project` | No | Project root (autodetected). |

## Exit codes

| Code | Condition |
|---|---|
| `0` | Recurrence recorded; stdout `<slug>: recurred <N>\n`. |
| `2` | Missing/malformed `<slug>`. |
| `3` | No lesson at `spec/lessons/<slug>.md`. |
| `10` | Unexpected I/O failure, or `spec lint --fix` failed after the rewrite. |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [lesson (CLI group)](../README.md) | Parent group. |
| [cli/lesson/list](../list/README.md), [cli/lesson/info](../info/README.md) | Both surface the `**Recurred:**` count this verb writes. |
| [cli/lesson/change-status](../change-status/README.md) | The deliberate follow-on action a recurrence signals — never triggered automatically by this verb. |

## Acceptance Criteria

### AC: recur-increments-count (verifies REQ:recur-increments-count)

**Given** `spec/lessons/kinder-fake.md` with `**Recurred:** 0`
**When** the user runs `specscore lesson recur kinder-fake --note "happened again"`
**Then** the command exits `0` with stdout `kinder-fake: recurred 1\n`, and the file's `**Recurred:**` value is `1`.

### AC: recur-appends-dated-entry (verifies REQ:recur-appends-dated-entry)

**Given** a lesson with an existing `## Recurrences` section carrying one entry
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

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
