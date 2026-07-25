---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: Lesson Change-Status

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/change-status?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/change-status?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/change-status?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/change-status?op=request-change) |
**Status:** Draft
**Source Ideas:** —

## Summary

`specscore lesson change-status <slug> --to=<status> [--note] [--successor]` transitions a Lesson artifact from its current `**Status:**` up the enforcement ladder (`Recorded` → `Stated` → `Enforced`, including a direct `Recorded` → `Enforced` skip-ahead for a lesson that reaches a machine gate without ever passing through an advisory stage) or into one of two dispositions — `Withdrawn` or `Superseded` — reachable from every rung. It implements the [lifecycle-transitions](../../lifecycle-transitions/README.md) shared contract for the Lesson kind. Lessons are flat single files that never relocate, so — like `plan change-status` — this verb has no file-relocation side effect: every transition is a pure status rewrite plus index sync, plus the optional `## Resolution` note and `**Superseded By:**` reference.

## Synopsis

```
specscore lesson change-status <slug> --to=<status> [--note <markdown>] [--successor <lesson-slug>] [--project <path>]
```

## Problem

A hand-maintained markdown log has no notion of a status transition at all — `LESSONS-LEARNED.md`'s own convention is a hand-edited `**Status:**` line ("updating its `Status:` line" is explicitly listed as one of the only permitted edits to a past entry), which is precisely the anti-pattern `idea`/`feature`/`plan change-status` already eliminate for their kinds. A dedicated verb closes the gap for Lesson: every promotion up the ladder, and every retirement, goes through the same validated, rollback-safe, index-syncing path every other kind uses.

## Behavior

This verb inherits every cross-cutting rule from [lifecycle-transitions](../../lifecycle-transitions/README.md) — strict state machine (exit `4`), `--to` parse + slug resolution, atomic rewrite + `spec lint --fix` index sync, rollback (exit `10`), `<slug>: <from> → <to>` success line, the shared exit-code mapping, and the optional `--note` → `## Resolution` mechanism. The REQs below are the Lesson-specific declarations.

### Legal-transition matrix

| From | To | Notes |
|---|---|---|
| `Recorded` | `Stated` | advisory: loaded somewhere an agent reads before acting |
| `Recorded` | `Enforced` | skip-ahead: reached a machine gate directly |
| `Stated` | `Enforced` | a machine now refuses |
| `Recorded` | `Withdrawn` | disposition — **reason required** |
| `Stated` | `Withdrawn` | disposition — **reason required** |
| `Enforced` | `Withdrawn` | disposition — **reason required** |
| `Recorded` | `Superseded` | disposition — **reason + successor required** |
| `Stated` | `Superseded` | disposition — **reason + successor required** |
| `Enforced` | `Superseded` | disposition — **reason + successor required** |

#### REQ: legal-transition-matrix

The verb MUST accept only the `(from, to)` pairs in the matrix above; any other pair MUST exit `4` (InvalidTransition) with a stderr message naming both the current status and the legal target statuses. Per the Meta's not-idempotent invariant, re-running on the current status exits `4`.

#### REQ: target-status-flag

The verb MUST accept the target status via a required `--to=<status>` flag: `Stated`, `Enforced`, `Withdrawn`, or `Superseded` (case-insensitive; `Recorded` is never a settable target — it is only the initial state a freshly scaffolded lesson starts in). A missing `--to` exits `2`. An unrecognized value exits `2` naming the offending value.

#### REQ: lesson-slug-resolution

The `<slug>` positional MUST resolve to an existing Lesson file at `spec/lessons/<slug>.md` within the project root (autodetected, or `--project`). A slug that does not resolve exits `3` (NotFound) naming the expected path.

#### REQ: disposition-reason-required

Both disposition transitions — to `Withdrawn` and to `Superseded` — are reason-required: `--note <markdown>` is mandatory. A missing or empty/whitespace-only `--note` on `--to=withdrawn` or `--to=superseded` MUST exit `2` before any mutation. Ladder-climbing transitions (`Stated`, `Enforced`) keep `--note` optional; when supplied, the note is written per the shared `## Resolution` mechanism.

#### REQ: superseded-requires-successor

`--to=superseded` MUST additionally require a `--successor <lesson-slug>` flag naming the lesson that replaces this one. The successor MUST resolve to an existing lesson; an absent flag, empty value, or unresolvable successor MUST exit `2` before any mutation. On success the verb writes a `**Superseded By:** <lesson-slug>` header line. For every non-`Superseded` transition, supplying `--successor` MUST exit `2`.

#### REQ: lessons-index-sync

The post-mutation `specscore spec lint --fix` MUST sync the lessons index (`spec/lessons/README.md`) to the new status (rule L-004). The verb's exit `0` depends on the rewrite AND the lint pass both succeeding; a lint failure rolls back every mutation and exits `10`.

## Flags

| Flag | Required | Description |
|---|---|---|
| `--to` | Yes | Target status: `stated`, `enforced`, `withdrawn`, `superseded` (case-insensitive). |
| `--note` | Conditional | Markdown appended as a `## Resolution` section. **Required** for `--to=withdrawn` and `--to=superseded`; optional otherwise. |
| `--successor` | Conditional | Slug of the lesson that replaces this one. **Required** for `--to=superseded`; rejected for every other transition. |
| `--project` | No | Project root (autodetected). |

## Exit codes

| Code | Condition |
|---|---|
| `0` | Transition succeeded; file rewritten; lessons index synced. |
| `2` | Missing/malformed `<slug>`; missing/unrecognized `--to`; missing required `--note` on a disposition; missing/unresolvable `--successor` on `--to=superseded`; `--successor` on a non-superseded transition. |
| `3` | No Lesson file at `spec/lessons/<slug>.md`. |
| `4` | `(current_status, --to)` is not a legal transition. |
| `10` | I/O failure, or `spec lint --fix` failed after a successful rewrite (rollback applied). |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [lifecycle-transitions](../../lifecycle-transitions/README.md) | Defines every cross-cutting REQ this verb satisfies. |
| [cli/plan/change-status](../../plan/change-status/README.md) | Closest sibling — also a flat, relocation-free `change-status` whose disposition-reason-required and successor-reference mechanics this verb reuses directly. |
| [lesson (CLI group)](../README.md) | Parent group. |
| [spec lint](../../spec/lint/README.md) | Invoked internally for index sync; rules L-002/L-004 validate what this verb writes. |

## Dependencies

- cli/lifecycle-transitions

## Acceptance Criteria

### AC: recorded-to-stated-happy-path (verifies REQ:legal-transition-matrix, REQ:target-status-flag)

**Given** `spec/lessons/kinder-fake.md` in `**Status:** Recorded`
**When** the user runs `specscore lesson change-status kinder-fake --to=stated`
**Then** the command exits `0`, writes exactly `kinder-fake: Recorded → Stated\n` to stdout, and rewrites the Status line to `Stated`.

### AC: recorded-to-enforced-skip-ahead (verifies REQ:legal-transition-matrix)

**Given** `spec/lessons/kinder-fake.md` in `**Status:** Recorded`
**When** the user runs `specscore lesson change-status kinder-fake --to=enforced`
**Then** the command exits `0` with stdout `kinder-fake: Recorded → Enforced\n`. The skip-ahead arc is legal.

### AC: withdrawn-requires-reason (verifies REQ:disposition-reason-required)

**Given** `spec/lessons/kinder-fake.md` in `**Status:** Stated`
**When** the user runs `specscore lesson change-status kinder-fake --to=withdrawn` with no `--note`
**Then** the command exits `2`, naming the `Withdrawn` transition and stating a reason is required. The lesson is unchanged.

### AC: withdrawn-with-reason-writes-resolution (verifies REQ:disposition-reason-required)

**Given** `spec/lessons/kinder-fake.md` in `**Status:** Stated`
**When** the user runs `specscore lesson change-status kinder-fake --to=withdrawn --note "turned out to be a one-off"`
**Then** the command exits `0`, rewrites the Status line to `Withdrawn`, and appends a `## Resolution` section including the note text.

### AC: superseded-writes-successor-reference (verifies REQ:superseded-requires-successor)

**Given** `spec/lessons/kinder-fake.md` in `**Status:** Enforced` and an existing `spec/lessons/kinder-fake-v2.md`
**When** the user runs `specscore lesson change-status kinder-fake --to=superseded --note "generalized" --successor kinder-fake-v2`
**Then** the command exits `0`, rewrites the Status line to `Superseded`, and writes a `**Superseded By:** kinder-fake-v2` header line.

### AC: illegal-transition-rejected (verifies REQ:legal-transition-matrix)

**Given** `spec/lessons/kinder-fake.md` in `**Status:** Enforced`
**When** the user runs `specscore lesson change-status kinder-fake --to=stated`
**Then** the command exits `4`, naming `Enforced` as the current status. No file change.

### AC: not-found-exits-3 (verifies REQ:lesson-slug-resolution)

**Given** no lesson named `nonexistent`
**When** the user runs `specscore lesson change-status nonexistent --to=stated`
**Then** the command exits `3` naming the expected `spec/lessons/nonexistent.md` path.

### AC: lint-failure-rolls-back (verifies REQ:lessons-index-sync)

**Given** `spec/lessons/kinder-fake.md` in `**Status:** Recorded`
**When** `spec lint --fix` fails after a successful Status rewrite
**Then** a full rollback restores the original `**Status:**`, and the command exits `10`.

## Open Questions

- The canonical lifecycle has no resurrection from a disposition status — re-pursuing a withdrawn or superseded lesson means recording a new one. Whether to relax this (e.g., an explicit `Withdrawn → Recorded` "reopen" arc) is deferred until real usage shows the need.

---
*This document follows the https://specscore.md/feature-specification*
