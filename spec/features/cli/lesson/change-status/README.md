---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Lesson Change-Status

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/change-status?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/change-status?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/change-status?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/change-status?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Summary

`specscore lesson change-status <slug> --to=<status> [--note] [--successor]` transitions a canonical directory or compatibility flat Lesson from its current `**Status:**` up the enforcement ladder (`Recorded` → `Stated` → `Enforced`) or into `Withdrawn`/`Superseded`. It implements the [lifecycle-transitions](../../lifecycle-transitions/README.md) shared contract and never relocates the resolved artifact.

## Synopsis

```
specscore lesson change-status <slug> --to=<status> [--note <markdown>] [--successor <lesson-slug>] [--project <path>]
```

## Problem

A hand-maintained markdown log has no notion of a status transition at all — `LESSONS-LEARNED.md`'s own convention is a hand-edited `**Status:**` line ("updating its `Status:` line" is explicitly listed as one of the only permitted edits to a past entry), which is precisely the anti-pattern `idea`/`feature`/`plan change-status` already eliminate for their kinds. A dedicated verb closes the gap for Lesson: every promotion up the ladder, and every retirement, goes through the same validated, rollback-safe, index-syncing path every other kind uses.

## Behavior

This verb inherits the strict state machine (exit `4`), `--to` parsing, slug resolution, atomic rewrite, rollback (exit `10`), success line, shared exit-code mapping, and optional `--note` → `## Resolution` mechanism from [lifecycle-transitions](../../lifecycle-transitions/README.md). Lesson index synchronization is deliberately narrower than the generic repository-wide fixer.

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

The `<slug>` positional MUST resolve canonical `spec/lessons/<slug>/README.md` first, then compatibility `spec/lessons/<slug>.md`, and MUST reject a duplicate layout. A slug that does not resolve exits `3` (NotFound) naming the slug.

#### REQ: disposition-reason-required

Both disposition transitions — to `Withdrawn` and to `Superseded` — are reason-required: `--note <markdown>` is mandatory. A missing or empty/whitespace-only `--note` on `--to=withdrawn` or `--to=superseded` MUST exit `2` before any mutation. Ladder-climbing transitions (`Stated`, `Enforced`) keep `--note` optional; when supplied, the note is written per the shared `## Resolution` mechanism.

#### REQ: superseded-requires-successor

`--to=superseded` MUST additionally require a `--successor <lesson-slug>` flag naming the lesson that replaces this one. The successor MUST resolve to an existing lesson; an absent flag, empty value, or unresolvable successor MUST exit `2` before any mutation. On success the verb writes a `**Superseded By:** <lesson-slug>` header line. For every non-`Superseded` transition, supplying `--successor` MUST exit `2`.

#### REQ: lessons-index-sync

After the rewrite, the verb MUST upsert only the resolved Lesson's exact row in `spec/lessons/README.md`, then run lint read-only. It MUST NOT invoke a repository-wide fixer or migrate an unrelated `## Outstanding Questions` heading. Exit `0` depends on the rewrite, index upsert, and read-only lint all succeeding; failure restores the Lesson and index bytes and exits `10`.

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
| `3` | No canonical or compatibility Lesson for the slug. |
| `4` | `(current_status, --to)` is not a legal transition. |
| `10` | I/O, narrow index-upsert, or read-only lint failure after a successful rewrite (Lesson and index rollback applied). |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [lifecycle-transitions](../../lifecycle-transitions/README.md) | Defines every cross-cutting REQ this verb satisfies. |
| [cli/plan/change-status](../../plan/change-status/README.md) | Lifecycle sibling whose disposition-reason and successor-reference mechanics this verb reuses. |
| [lesson (CLI group)](../README.md) | Parent group. |
| [spec lint](../../spec/lint/README.md) | Invoked read-only after the exact L-004 index row upsert; only an explicit CLI `--fix` runs repository-wide fixers. |

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

**Given** no canonical or compatibility Lesson named `nonexistent`
**When** the user runs `specscore lesson change-status nonexistent --to=stated`
**Then** the command exits `3` naming `nonexistent`.

### AC: lint-failure-rolls-back (verifies REQ:lessons-index-sync)

**Given** `spec/lessons/kinder-fake.md` in `**Status:** Recorded`
**When** read-only lint fails after a successful Status rewrite and exact index-row upsert
**Then** a full rollback restores the original Lesson and index bytes, and the command exits `10`.

### AC: unrelated-files-remain-byte-identical

**Given** an unrelated Markdown file containing `## Outstanding Questions`
**When** a valid Lesson status transition succeeds
**Then** the unrelated bytes remain identical; only explicit `specscore spec lint --fix` performs that migration.

## Open Questions

- The canonical lifecycle has no resurrection from a disposition status — re-pursuing a withdrawn or superseded lesson means recording a new one. Whether to relax this (e.g., an explicit `Withdrawn → Recorded` "reopen" arc) is deferred until real usage shows the need.

---
*This document follows the https://specscore.md/feature-specification*
