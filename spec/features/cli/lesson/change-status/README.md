---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Lesson Change-Status

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/change-status?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/change-status?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/change-status?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/change-status?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Summary

`specscore lesson change-status <slug> --to=<status> [--note] [--successor]` transitions a canonical directory or compatibility flat Lesson from its current `**Status:**` up the enforcement ladder (`Recorded` → `Stated` → `Enforced`), into `Withdrawn`/`Superseded`, or back down via an audited correction (`Stated` → `Recorded` or `Enforced` → `Recorded`), after which re-enforcement requires a fresh Enforcement receipt. It implements the [lifecycle-transitions](../../lifecycle-transitions/README.md) shared contract and never relocates the resolved artifact.

## Synopsis

```
specscore lesson change-status <slug> --to=<status> [--note <markdown>] [--successor <lesson-slug>] [--project <path>]
```

## Problem

A hand-maintained markdown log has no notion of a status transition at all — `LESSONS-LEARNED.md`'s own convention is a hand-edited `**Status:**` line ("updating its `Status:` line" is explicitly listed as one of the only permitted edits to a past entry), which is precisely the anti-pattern `idea`/`feature`/`plan change-status` already eliminate for their kinds. A dedicated verb closes the gap for Lesson: every promotion up the ladder, and every retirement, goes through the same validated, fail-closed, index-syncing path every other kind uses.

## Behavior

This verb inherits the strict state machine (exit `4`), `--to` parsing, slug resolution, atomic rewrite, success line, shared exit-code mapping, and optional `--note` → `## Resolution` mechanism from [lifecycle-transitions](../../lifecycle-transitions/README.md). Lesson index synchronization is deliberately narrower than the generic repository-wide fixer. Once the rewrite is visible, rollback requires atomic ownership; absent that proof, the artifact/index and prepared event remain recoverable instead of overwriting concurrent data.

### Legal-transition matrix

| From | To | Notes |
|---|---|---|
| `Recorded` | `Stated` | advisory: loaded somewhere an agent reads before acting |
| `Recorded` | `Enforced` | skip-ahead: reached a machine gate directly — refused while the receipt equals one an audited correction recorded (REQ: re-enforcement-requires-fresh-receipt) |
| `Stated` | `Enforced` | a machine now refuses — same fresh-receipt condition |
| `Recorded` | `Withdrawn` | disposition — **reason required** |
| `Stated` | `Withdrawn` | disposition — **reason required** |
| `Enforced` | `Withdrawn` | disposition — **reason required** |
| `Recorded` | `Superseded` | disposition — **reason + successor required** |
| `Stated` | `Superseded` | disposition — **reason + successor required** |
| `Enforced` | `Superseded` | disposition — **reason + successor required** |
| `Stated` | `Recorded` | audited correction — **reason required** — demotes a Stated lesson whose Enforcement section names no binding control |
| `Enforced` | `Recorded` | audited correction — **reason required** — demotes an Enforced lesson whose claimed control turns out not to bind; the lesson returns to the not-enforced queue |

#### REQ: legal-transition-matrix

The verb MUST accept only the `(from, to)` pairs in the matrix above; any other pair MUST exit `4` (InvalidTransition) with a stderr message naming both the current status and the legal target statuses. Per the Meta's not-idempotent invariant, re-running on the current status exits `4`.

#### REQ: target-status-flag

The verb MUST accept the target status via a required `--to=<status>` flag: `Stated`, `Enforced`, `Withdrawn`, `Superseded`, or `Recorded` (case-insensitive). `Recorded` is the initial state a freshly scaffolded lesson starts in, and is also reachable as the audited correction target from `Stated` or `Enforced` (see REQ: audited-correction-requires-reason). A missing `--to` exits `2`. An unrecognized value exits `2` naming the offending value.

#### REQ: lesson-slug-resolution

The `<slug>` positional MUST resolve canonical `spec/lessons/<slug>/README.md` first, then compatibility `spec/lessons/<slug>.md`, and MUST reject a duplicate layout. A slug that does not resolve exits `3` (NotFound) naming the slug.

#### REQ: disposition-reason-required

Both disposition transitions — to `Withdrawn` and to `Superseded` — are reason-required: `--note <markdown>` is mandatory. A missing or empty/whitespace-only `--note` on `--to=withdrawn` or `--to=superseded` MUST exit `2` before any mutation. Ladder-climbing transitions (`Stated`, `Enforced`) keep `--note` optional; when supplied, the note is written per the shared `## Resolution` mechanism.

#### REQ: audited-correction-requires-reason

The audited corrections `Stated` → `Recorded` and `Enforced` → `Recorded` are reason-required under one identical audit: a missing or empty/whitespace-only `--note` on `--to=recorded` MUST exit `2` before any mutation, naming the audited correction and the required reason (e.g. the Enforcement section named no binding control). Because a Recorded Lesson carries the L-010 obligation, a demotion whose `**Control:**` names no mechanism and is not `none-yet: <why>` MUST be refused with exit `4` before any write, never left to fail the post-write lint. On success the note is written per the shared `## Resolution` mechanism, followed by its correction-receipt line (REQ: re-enforcement-requires-fresh-receipt); every earlier section and Resolution entry is kept, and the demoted lesson is again matched by `lesson list --not-enforced`. No other downward arc is legal (`Enforced` → `Stated` stays illegal).

#### REQ: re-enforcement-requires-fresh-receipt

Threat model: this guard stops accidental re-enforcement on a disproved receipt and any bypass made through the CLI or the exported `lesson.ChangeStatus` API. Hand-editing the Lesson file is out of scope; the Git diff and review cover that.

- **Receipt.** A Lesson's receipt is the lowercase hex SHA-256 of `Control: <c>\nVerification: <v>\nEvidence: <e>\n`, where each value is the first `**Control:**`, `**Verification:**`, or `**Evidence:**` field (with hand-wrapped continuation lines) inside the first `## Enforcement` section, with every whitespace run collapsed to one space. Fields anywhere else (`## Resolution`, other sections, frontmatter, fenced or indented code, HTML comments) never contribute. A Lesson whose `## Enforcement` has none of the three values (e.g. a compatibility Lesson with prose only) has no receipt: its demotion stamps none and its re-enforcement is never refused.
- **Stamp.** An audited correction to `Recorded` (from `Stated` or `Enforced`) MUST end its `## Resolution` note with its own unindented paragraph `Enforcement receipt at correction: sha256:<hex>` for the receipt read under the per-Lesson lock.
- **Refusal.** A `--to=enforced` (from `Recorded` or `Stated`) MUST exit `4`, before any write, when the current receipt equals any receipt line ever recorded in a `## Resolution` section, naming the receipt and the fix (replace the disproved receipt with a fresh positive one). Only structural, unindented receipt lines count, exactly where the tool writes them; lines inside fenced or indented code, HTML comments, or frontmatter never do. The CLI checks before preparing any event; `lesson.ChangeStatus` repeats the check under the per-Lesson lock and returns a pre-publication failure, so a raced refusal still exits `4` with its prepared event aborted. `--dry-run` MUST report the same refusal.
- **Note hygiene.** Every `lesson change-status --note` (all transitions, CLI and `lesson.ChangeStatus`) MUST be refused with exit `2` before any mutation when it contains a line mentioning `Enforcement receipt at correction`, a `**Control:**`, `**Verification:**`, or `**Evidence:**` field line, a `## ` heading (it would end the `## Resolution` section before the receipt), or an unclosed code fence or HTML comment (it would swallow the receipt line).
- **Earlier corrections.** A correction recorded before this requirement carries no receipt line and is not refused: it has no baseline, and no migration fabricates one.

No other transition, verb, or event payload changes.

#### REQ: superseded-requires-successor

`--to=superseded` MUST additionally require a `--successor <lesson-slug>` flag naming the lesson that replaces this one. The successor MUST resolve to an existing lesson; an absent flag, empty value, or unresolvable successor MUST exit `2` before any mutation. On success the verb writes a `**Superseded By:** <lesson-slug>` header line. For every non-`Superseded` transition, supplying `--successor` MUST exit `2`.

#### REQ: lessons-index-sync

The verb MUST hold the resolved Lesson's private lifecycle lock continuously from transition validation through rewrite, exact-row reconciliation, read-only lint, and durability fences. Inside that scope it acquires the shared Lesson-index writer lock only for the index mutation; the total lock order is always per-Lesson first, shared index second. It then durably fences both artifact/index files and parent directories before event commit. It MUST NOT invoke a repository-wide fixer or migrate an unrelated `## Outstanding Questions` heading. Exit `0` depends on every step succeeding. A post-rewrite failure exits `10`, retains the prepared event, and MUST preserve the published state and concurrent index rows rather than restoring whole-file snapshots.

## Flags

| Flag | Required | Description |
|---|---|---|
| `--to` | Yes | Target status: `stated`, `enforced`, `withdrawn`, `superseded` (case-insensitive). |
| `--note` | Conditional | Markdown appended as a `## Resolution` section. **Required** for `--to=withdrawn`, `--to=superseded`, and `--to=recorded`; optional otherwise. Must not contain a receipt-like line, an Enforcement field line, a `## ` heading, or an open code fence / HTML comment. |
| `--successor` | Conditional | Slug of the lesson that replaces this one. **Required** for `--to=superseded`; rejected for every other transition. |
| `--project` | No | Project root (autodetected). |

## Exit codes

| Code | Condition |
|---|---|
| `0` | Transition succeeded; file rewritten; lessons index synced. |
| `1` | The Lessons index (`spec/lessons/README.md`) carries unresolved git merge-conflict markers; nothing was modified. |
| `2` | Missing/malformed `<slug>`; missing/unrecognized `--to`; missing required `--note` on a disposition or on `--to=recorded`; a `--note` refused by REQ: re-enforcement-requires-fresh-receipt (receipt-like line, Enforcement field line, `## ` heading, or open code fence / HTML comment); missing/unresolvable `--successor` on `--to=superseded`; `--successor` on a non-superseded transition. |
| `3` | No canonical or compatibility Lesson for the slug. |
| `4` | `(current_status, --to)` is not a legal transition, a demotion to `Recorded` whose `**Control:**` would fail L-010, or a `--to=enforced` whose Enforcement receipt equals one an audited correction recorded. |
| `10` | I/O, narrow index-upsert, read-only lint, or durability-fence failure; post-publication state and its prepared recovery event are retained. |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [lifecycle-transitions](../../lifecycle-transitions/README.md) | Defines every cross-cutting REQ this verb satisfies. Also implements the Meta's [`--dry-run`](../../lifecycle-transitions/README.md#req-dry-run-mode) and [`transitions`](../../lifecycle-transitions/README.md#req-transitions-query-verb) verbs. |
| [cli/plan/change-status](../../plan/change-status/README.md) | Closest sibling — also a flat, relocation-free `change-status` whose disposition-reason-required and successor-reference mechanics this verb reuses directly. |
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

### AC: stated-to-recorded-requires-reason (verifies REQ:audited-correction-requires-reason)

**Given** `spec/lessons/kinder-fake.md` in `**Status:** Stated`
**When** the user runs `specscore lesson change-status kinder-fake --to=recorded` with no `--note`
**Then** the command exits `2`, naming the audited correction and stating a reason is required. The lesson is unchanged.

### AC: stated-to-recorded-with-reason-demotes (verifies REQ:audited-correction-requires-reason, REQ:legal-transition-matrix)

**Given** `spec/lessons/kinder-fake.md` in `**Status:** Stated`
**When** the user runs `specscore lesson change-status kinder-fake --to=recorded --note "Enforcement section named no binding control"`
**Then** the command exits `0`, rewrites the Status line to `Recorded`, and appends a `## Resolution` section including the note text.

### AC: enforced-to-recorded-requires-reason (verifies REQ:audited-correction-requires-reason)

**Given** `spec/lessons/kinder-fake.md` in `**Status:** Enforced`
**When** the user runs `specscore lesson change-status kinder-fake --to=recorded` with no `--note`
**Then** the command exits `2`, naming the audited correction and stating a reason is required. The lesson is unchanged.

### AC: enforced-to-recorded-with-reason-demotes (verifies REQ:audited-correction-requires-reason, REQ:legal-transition-matrix)

**Given** `spec/lessons/kinder-fake.md` in `**Status:** Enforced` with an existing `## Resolution` entry
**When** the user runs `specscore lesson change-status kinder-fake --to=recorded --note "claimed control does not bind"`
**Then** the command exits `0` with stdout `kinder-fake: Enforced → Recorded\n`, rewrites the Status line to `Recorded`, appends a `## Resolution` section including the note text while keeping the earlier entry, updates the lessons index row, and `specscore lesson list --not-enforced` lists the lesson.

### AC: re-enforce-unchanged-receipt-refused (verifies REQ:re-enforcement-requires-fresh-receipt)

**Given** `spec/lessons/kinder-fake/README.md` demoted `Enforced` → `Recorded` by `change-status --to=recorded --note "claimed control does not bind"`, its `## Resolution` note now ending with `Enforcement receipt at correction: sha256:<hex>`, and Control/Verification/Evidence unedited since
**When** the user runs `specscore lesson change-status kinder-fake --to=enforced`, with or without `--dry-run`
**Then** the command exits `4`, naming the disproved receipt and telling the user to replace it; stdout is empty and the project tree, including `.specscore/events.jsonl`, is byte-identical afterwards.

### AC: re-enforce-whitespace-only-edit-refused (verifies REQ:re-enforcement-requires-fresh-receipt)

**Given** the demoted lesson above with only whitespace changed in its Enforcement fields — an intra-line run such as `go  test\t./pkg/lint`, or a field re-wrapped across lines
**When** the user runs `specscore lesson change-status kinder-fake --to=enforced`
**Then** the command exits `4`: re-wrapping is not a fresh receipt.

### AC: re-enforce-non-enforcement-edit-refused (verifies REQ:re-enforcement-requires-fresh-receipt)

**Given** the demoted lesson above with only a section other than `## Enforcement` edited
**When** the user runs `specscore lesson change-status kinder-fake --to=enforced`
**Then** the command exits `4` with the tree byte-identical.

### AC: re-enforce-any-disproved-receipt-refused (verifies REQ:re-enforcement-requires-fresh-receipt)

**Given** the lesson demoted on receipt R1, re-enforced on a fresh receipt R2, demoted again on R2, and then edited back to R1
**When** the user runs `specscore lesson change-status kinder-fake --to=enforced`
**Then** the command exits `4`: every recorded receipt stays disproved, not only the last.

### AC: re-enforce-with-fresh-receipt-allowed (verifies REQ:re-enforcement-requires-fresh-receipt)

**Given** the demoted lesson above with its `**Evidence:**` replaced by new evidence
**When** the user runs `specscore lesson change-status kinder-fake --to=enforced`
**Then** the command exits `0` with stdout `kinder-fake: Recorded → Enforced\n`, and the earlier correction note and receipt line are kept.

### AC: stated-correction-also-requires-fresh-receipt (verifies REQ:re-enforcement-requires-fresh-receipt)

**Given** `spec/lessons/kinder-fake/README.md` demoted `Stated` → `Recorded` with an unchanged receipt since
**When** the user runs `--to=enforced`, or `--to=stated` followed by `--to=enforced`
**Then** each `--to=enforced` exits `4` with the tree byte-identical; `--to=stated` stays legal.

### AC: note-cannot-forge-receipt-state (verifies REQ:re-enforcement-requires-fresh-receipt)

**Given** a demoted lesson
**When** the user runs `change-status` with a `--note` carrying a receipt-like line, a `**Control:**`/`**Verification:**`/`**Evidence:**` field line (including a demotion note quoting the bad Control), a `## ` heading, or an unclosed code fence
**Then** the command exits `2` and the tree and ledger are byte-identical.

### AC: re-enforce-refused-under-lock (verifies REQ:re-enforcement-requires-fresh-receipt)

**Given** a re-enforcement on a disproved receipt that passes the CLI pre-check (a race)
**When** `lesson.ChangeStatus` runs under the per-Lesson lock
**Then** it refuses with a pre-publication failure, the command exits `4` (not `10`), the prepared event is aborted, and the lesson and ledger are unchanged.

### AC: prose-only-enforcement-has-no-receipt (verifies REQ:re-enforcement-requires-fresh-receipt)

**Given** a compatibility Lesson whose `## Enforcement` is prose with no Control/Verification/Evidence fields
**When** it is demoted to `Recorded` and later promoted to `Enforced`
**Then** the demotion stamps no receipt line and the promotion is not refused.

### AC: pre-receipt-correction-not-refused (verifies REQ:re-enforcement-requires-fresh-receipt)

**Given** a `Recorded` lesson whose `## Resolution` records an audited correction but no `Enforcement receipt at correction:` line
**When** the user runs `specscore lesson change-status kinder-fake --to=enforced`
**Then** the command exits `0`: a correction recorded before receipts existed has no baseline and does not block.

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
**Then** the command exits `10`, retains the rewritten Lesson, its row, every concurrent foreign row, and the prepared lifecycle event for explicit recovery; it does not restore a whole-file snapshot. (The AC identifier is retained for traceability to the superseded rollback behavior.)

### AC: conflicted-lessons-index-fails-closed (verifies REQ:lessons-index-sync)

**Given** `spec/lessons/README.md` carries unresolved git merge-conflict markers — around this Lesson's own row, elsewhere in the index, as a one-sided block, or inside a legacy five-column table
**When** the user runs `specscore lesson change-status kinder-fake --to=stated`
**Then** the Lessons-index pre-write check ([cli/spec/lint](../../spec/lint/README.md#req-writers-refuse-conflicted-files)) refuses before the Lesson file is rewritten or any event is prepared: the command exits `1` naming the index, and the Lesson file, the index, and the event ledger are byte-identical afterwards.

### AC: enforced-to-recorded-requires-mechanism-control (verifies REQ:audited-correction-requires-reason)

**Given** `spec/lessons/kinder-fake/README.md` in `**Status:** Enforced`, dated after the L-010 threshold, whose `**Control:**` is free text naming no mechanism and not `none-yet: <why>`
**When** the user runs `specscore lesson change-status kinder-fake --to=recorded --note "claimed control does not bind"`
**Then** the command exits `4` before any write, explaining that `**Control:**` must name a mechanism or `none-yet: <why>` first; no file or event changes.

### AC: unrelated-files-remain-byte-identical

**Given** an unrelated Markdown file containing `## Outstanding Questions`
**When** a valid Lesson status transition succeeds
**Then** the unrelated bytes remain identical; only explicit `specscore spec lint --fix` performs that migration.

## Open Questions

- The canonical lifecycle has no resurrection from a disposition status — re-pursuing a withdrawn or superseded lesson means recording a new one. Whether to relax this (e.g., an explicit `Withdrawn → Recorded` "reopen" arc) is deferred until real usage shows the need. (This is distinct from the `Stated → Recorded` and `Enforced → Recorded` audited corrections added above, which address a lesson found to lack a binding control — not a retired disposition.)

---
*This document follows the https://specscore.md/feature-specification*
