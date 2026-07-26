---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Lesson (CLI)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Summary

Record and query process-gap lessons — what we learned but not yet enforced

## Contents

| Child | Description |
|---|---|
| [new](new/README.md) | Scaffold a new Lesson artifact |
| [list](list/README.md) | List lesson slugs with an optional status filter |
| [info](info/README.md) | Show a single lesson's metadata and section coverage |
| [change-status](change-status/README.md) | Transition a lesson up the enforcement ladder or retire it |
| [recur](recur/README.md) | Record that a lesson's gap manifested again |

## Problem

`chatwright/backstage/LESSONS-LEARNED.md` is a hand-maintained, append-only markdown log where AI agents record process gaps they hit and how they should be prevented. Its own thesis is that a lesson written as prose decays; only a machine-checked rule holds — yet the document's single mechanism for tracking which lessons still need to graduate is a hand-maintained "Open: lessons that need to graduate" list at the bottom, which rots exactly the way the document warns against. Its `L1`, `L2`, … numbering collides when multiple agents append in parallel. And nothing stops an agent recording an entry with no proposed enforcement path — the exact shape of a useless entry.

`specscore lesson` makes the lesson a first-class SpecScore artifact — slugged (not numbered, so parallel agents never collide), lint-checked for structure (never content), and queryable by lifecycle status — so "what have we learned but not yet enforced?" is `specscore lesson list --not-enforced`, not a grep of hand-maintained prose. "Not yet enforced" means Recorded **or** Stated — the log's own thesis is that only Tier 2 (Enforced) binds, so a filter that only matched `Recorded` would miss every advisory-but-unenforced lesson sitting at `Stated`.

## Behavior

### A lesson is about process, not a defect

A lesson is not a bug report and the log is not a record of fixed defects. A code defect may be the *evidence* that surfaced a gap, but the entry is about the missing check, gate, convention, or review step that let the defect ship unnoticed — never about the defect itself. The four required body sections encode that distinction directly: `## Incident` (evidence only, not the point), `## Process gap` (the load-bearing section — which check was missing, ambiguous, or existed but never ran), `## Check` (the concrete check or balance that would catch it next time), and `## Enforcement` (the tier and where it binds).

#### REQ: process-gap-is-the-lesson

Every Lesson body MUST declare a `## Process gap` section naming the missing check, gate, convention, or review step. An entry that describes only an `## Incident` with no `## Process gap` is the exact useless entry this contract exists to refuse — `specscore lesson new` always scaffolds the section (with a prompt); lint rule L-001 refuses a Lesson where it (or any of the other three required sections) is absent from the body, checking presence only, never content, length, or wording.

### The enforcement ladder is the lifecycle

A Lesson's `**Status:**` climbs three rungs — `Recorded` (Tier 0: written down, prevents nothing), `Stated` (Tier 1: loaded by an agent before acting — a `CLAUDE.md`, a standards doc, agent memory — advisory), `Enforced` (Tier 2: a machine refuses — a CI gate, a lint rule, a boundary script, a conformance suite, a required test — binding) — or is retired into one of two dispositions reachable from any rung: `Withdrawn` (turned out not to be a real pattern) or `Superseded` (replaced by a newer, more general lesson). The vocabulary deliberately mirrors [cli/plan](../plan/README.md)'s disposition set rather than inventing new terms.

#### REQ: subcommands

`specscore lesson` MUST expose the `new`, `list`, `info`, `change-status`, and `recur` subcommands. Invoking `specscore lesson` with no subcommand MUST print the group help and exit `0`.

### Recurrence is the strongest graduation signal

A lesson that recurs while still `Recorded` or `Stated` is the strongest possible evidence it needs to graduate — the original `check-tags-before-tagging` lesson in `LESSONS-LEARNED.md` recurred twice in one session as prose before anyone acted on it. `specscore lesson recur <slug>` records exactly that: it increments a `**Recurred:** N` header count and appends a dated (optionally noted) entry to a `## Recurrences` section, without itself changing `**Status:**` — recurrence is a signal, not an automatic promotion; a human or agent still runs `change-status` deliberately once the signal is acted on. `lesson recur` against a lesson already `Withdrawn` or `Superseded` is evidence the retirement itself was wrong, so it warns on stderr (still exiting `0` and recording the occurrence) rather than succeeding silently. `lesson list --min-recurred <N>` makes the count directly queryable — combined with `--not-enforced`, "which lessons have recurred and are still not enforced?" is one command, not eyeballing a listing.

#### REQ: mutation-scope

The `list` and `info` subcommands MUST NOT create, edit, or transition lesson files — they read `spec/lessons/*.md` only. The `new` subcommand MAY create a new lesson file but MUST NOT edit or transition existing lessons. The `change-status` subcommand transitions a lesson's lifecycle status. The `recur` subcommand mutates a lesson's `**Recurred:**` count and its `## Recurrences` section but MUST NOT touch `**Status:**`.

#### REQ: recurrence-is-queryable

`lesson list` MUST accept a `--min-recurred <N>` filter restricting output to lessons whose `**Recurred:**` count is at least `N`, composable with the status filters (`--status`, `--not-enforced`) rather than mutually exclusive with them.

### Shared flags

Every command in this group accepts the shared flags defined in the [CLI parent](../README.md): `--project`, `--format`, and `-h/--help`.

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [CLI](../README.md) | Inherits shared exit-code contract, `--format`/`--project` conventions, and project autodetection. |
| [cli/plan](../plan/README.md) | Closest structural sibling: a flat single-file artifact family whose disposition vocabulary (`Withdrawn`, `Superseded`) and non-relocating `change-status` model this group reuses directly. |
| [lifecycle-transitions](../lifecycle-transitions/README.md) | `lesson change-status` implements this shared contract for the Lesson kind. |
| [spec lint](../spec/lint/README.md) | Hosts the `L-001`–`L-004` rule family documented in [cli/spec/lint/lesson-rules](../spec/lint/lesson-rules/README.md). |

## Acceptance Criteria

### AC: group-exposes-subcommands (verifies REQ:subcommands)

**Given** a project with a `spec/lessons/` directory
**When** the user runs `specscore lesson`
**Then** the group help is printed listing `new`, `list`, `info`, `change-status`, and `recur`, and the command exits `0`.

### AC: process-gap-required (verifies REQ:process-gap-is-the-lesson)

**Given** a lesson file with an `## Incident` section but no `## Process gap` section
**When** the user runs `specscore spec lint`
**Then** an `L-001` violation is reported naming `Process gap` as missing, and the violation message states that an entry naming no process gap is the one this rule exists to refuse.

### AC: recur-does-not-change-status (verifies REQ:mutation-scope)

**Given** a lesson in `**Status:** Stated`
**When** the user runs `specscore lesson recur <slug>`
**Then** the lesson's `**Recurred:**` count increments and a dated entry is appended to `## Recurrences`, but `**Status:**` remains `Stated`.

### AC: not-enforced-and-min-recurred-compose (verifies REQ:recurrence-is-queryable)

**Given** a lesson `flaky-check` in `**Status:** Stated` with `**Recurred:** 2`, and a lesson `quiet-check` in `**Status:** Stated` with `**Recurred:** 0`
**When** the user runs `specscore lesson list --not-enforced --min-recurred=1`
**Then** stdout lists only `flaky-check`.

### AC: recur-against-retired-lesson-warns (verifies the recurrence-graduation-signal behavior)

**Given** a lesson in `**Status:** Withdrawn`
**When** the user runs `specscore lesson recur <slug>`
**Then** the command exits `0`, the `**Recurred:**` count still increments, and stderr carries a warning naming the lesson and its retired status.

## Open Questions

- Should a Lesson ever record a structured link to the code change that closed its `## Process gap` (mirroring a Plan task's `**Implemented-by:**`), or does the free-form `## Enforcement` prose stay sufficient? Deferred until a second consumer repo shows the need.
- Should `lesson recur` past some threshold count refuse to exit `0` for an *active* (non-terminal) lesson — turning "this keeps recurring" into a CI-visible signal on its own — or does `lesson list --min-recurred` stay sufficient for a human/agent to act on deliberately? Recurrence against a *retired* lesson already warns (see `recur`'s own spec); this question is only about the active-lesson case. Left to lint/CI policy in the consuming repo for now.

---
*This document follows the https://specscore.md/feature-specification*
