---
format: https://specscore.md/feature-specification
status: Amending
---

# Feature: Lesson (CLI)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson?op=request-change) |
**Status:** Amending
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
| [occurrence](occurrence/README.md) | Capture and query normalized occurrences of a process gap. |
| [import-legacy](import-legacy/README.md) | Losslessly import historical prose lessons into canonical artifacts. |
| [migrate-flat](migrate-flat/README.md) | Move one committed flat Lesson into the canonical directory layout as a resumable transaction. |
| [relations](relations/README.md) | Require human-confirmed lesson relationships and duplicate disposition. |
| [check](check/README.md) | Make recurrence and improvement policy CI-enforceable. |
| [coordination](coordination/README.md) | Expose durable Synchestra agent context without brokering live messages. |
| [events](events/README.md) | Emit reliable lifecycle and occurrence events for process improvement. |

## Problem

`chatwright/backstage/LESSONS-LEARNED.md` is a hand-maintained, append-only markdown log where AI agents record process gaps they hit and how they should be prevented. Its own thesis is that a lesson written as prose decays; only a machine-checked rule holds — yet the document's single mechanism for tracking which lessons still need to graduate is a hand-maintained "Open: lessons that need to graduate" list at the bottom, which rots exactly the way the document warns against. Its `L1`, `L2`, … numbering collides when multiple agents append in parallel. And nothing stops an agent recording an entry with no proposed enforcement path — the exact shape of a useless entry.

`specscore lesson` makes the lesson a first-class SpecScore artifact — slugged (not numbered, so parallel agents never collide), lint-checked for structure (never content), and queryable by lifecycle status — so "what have we learned but not yet enforced?" is `specscore lesson list --not-enforced`, not a grep of hand-maintained prose. "Not yet enforced" means Recorded **or** Stated — the log's own thesis is that only Tier 2 (Enforced) binds, so a filter that only matched `Recorded` would miss every advisory-but-unenforced lesson sitting at `Stated`.

This amendment makes a Lesson a durable process-improvement record. New Lessons use a directory (`spec/lessons/<slug>/README.md`) so independent occurrences do not rewrite the lesson; existing `spec/lessons/<slug>.md` files remain readable, linted, and addressable until explicit migration. The child Features own occurrence, import, evidence, policy, coordination, and reliable-delivery behavior.

## Behavior

### A lesson is about process, not a defect

A lesson is not a bug report and the log is not a record of fixed defects. A code defect may be the *evidence* that surfaced a gap, but the compact canonical README is about the durable rule and the missing check that allowed the manifestation. Canonical Lessons therefore require `## Lesson`, `## Process Gap`, `## Tracking`, `## Enforcement`, and `## Open Questions`; incident history lives in typed child Occurrences. Compatibility flat Lessons retain their historical `## Incident`, `## Process gap`, `## Check`, and `## Enforcement` sections until explicit migration.

#### REQ: process-gap-is-the-lesson

Every canonical Lesson body MUST declare a concise `## Process Gap` section naming the missing check, gate, convention, or review step and a `## Lesson` section stating the durable rule. `specscore lesson new` scaffolds the complete canonical section set; lint rule L-001 applies the canonical set to directory Lessons and the historical four-section set to compatibility flat Lessons.

### The enforcement ladder is the lifecycle

A Lesson's `**Status:**` climbs three rungs — `Recorded` (Tier 0: written down, prevents nothing), `Stated` (Tier 1: loaded by an agent before acting — a `CLAUDE.md`, a standards doc, agent memory — advisory), `Enforced` (Tier 2: a machine refuses — a CI gate, a lint rule, a boundary script, a conformance suite, a required test — binding) — or is retired into one of two dispositions reachable from any rung: `Withdrawn` (turned out not to be a real pattern) or `Superseded` (replaced by a newer, more general lesson). The vocabulary deliberately mirrors [cli/plan](../plan/README.md)'s disposition set rather than inventing new terms.

#### REQ: subcommands

`specscore lesson` MUST expose the existing `new`, `list`, `info`, `change-status`, and `recur` subcommands plus the child contracts' `occurrence`, `import-legacy`, `migrate-flat`, `relation`, `check`, and `agents` verbs. Invoking `specscore lesson` with no subcommand MUST print the group help and exit `0`.

### Recurrence is the strongest graduation signal

A lesson that recurs while still `Recorded` or `Stated` is the strongest possible evidence it needs to graduate. For a canonical Lesson, `specscore lesson recur <slug>` appends one immutable typed child and derives recurrence metadata without rewriting the README. For a compatibility flat Lesson it retains the historical count-and-prose rewrite until explicit migration. Neither path changes `**Status:**`; a human or agent still runs `change-status` deliberately. A recurrence against `Withdrawn` or `Superseded` still records the evidence and warns. `lesson list --min-recurred <N>` makes the derived count directly queryable.

#### REQ: mutation-scope

The `list` and `info` subcommands MUST NOT create, edit, or transition either layout. The `new` subcommand creates only the canonical directory layout and MUST refuse a sibling flat file. The `change-status` subcommand transitions the resolved Lesson without relocating it. For a canonical Lesson, `recur` appends one child occurrence and leaves the README byte-identical; for a flat Lesson it mutates the compatibility `**Recurred:**` count and `## Recurrences` section. Neither path touches `**Status:**`.

#### REQ: recurrence-is-queryable

`lesson list` MUST accept a `--min-recurred <N>` filter restricting output to lessons whose `**Recurred:**` count is at least `N`, composable with the status filters (`--status`, `--not-enforced`) rather than mutually exclusive with them.

### Durable facts versus live coordination

SpecScore owns durable, reviewable artifact facts: lifecycle/occurrence history,
evidence/tracking links, and links to agent work. A SpecScore command resolves
the Lesson locally, then may render adapter-produced links or deliberately pass
project context plus a canonical opaque external-resource reference to its
configured adapter. Synchestra owns generic effort/run/session association,
mutable agent presence, authorization, live message delivery, replay, and
resumption; it MUST NOT parse or resolve Lesson slugs, statuses, occurrences,
relations, or expose a Lesson-specific API. SpecScore MUST NOT persist live
message bodies, create a message outbox, or read Synchestra Git/SQLite storage.

### Shared flags

Every command in this group accepts the shared flags defined in the [CLI parent](../README.md): `--project`, `--format`, and `-h/--help`.

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [CLI](../README.md) | Inherits shared exit-code contract, `--format`/`--project` conventions, and project autodetection. |
| [cli/plan](../plan/README.md) | Closest structural sibling: a flat single-file artifact family whose disposition vocabulary (`Withdrawn`, `Superseded`) and non-relocating `change-status` model this group reuses directly. |
| [lifecycle-transitions](../lifecycle-transitions/README.md) | `lesson change-status` implements this shared contract for the Lesson kind. |
| [spec lint](../spec/lint/README.md) | Hosts the `L-001`–`L-009` rule family documented in [cli/spec/lint/lesson-rules](../spec/lint/lesson-rules/README.md). |
| [cli/event](../event/README.md) | Delivers lifecycle/occurrence events through the reliable per-subscriber behavior in [events](events/README.md). |
| [coordination](coordination/README.md) | Renders durable agent-work links and delegates explicit live actions to Synchestra without becoming a broker. |

## Acceptance Criteria

### AC: group-exposes-subcommands (verifies REQ:subcommands)

**Given** a project with a `spec/lessons/` directory
**When** the user runs `specscore lesson`
**Then** the group help is printed listing `new`, `list`, `info`, `change-status`, `recur`, `occurrence`, `import-legacy`, `migrate-flat`, `relation`, `check`, and `agents`, and the command exits `0`.

### AC: process-gap-required (verifies REQ:process-gap-is-the-lesson)

**Given** a lesson file with an `## Incident` section but no `## Process gap` section
**When** the user runs `specscore spec lint`
**Then** an `L-001` violation is reported naming `Process gap` as missing, and the violation message states that an entry naming no process gap is the one this rule exists to refuse.

### AC: recur-does-not-change-status (verifies REQ:mutation-scope)

**Given** a canonical Lesson in `**Status:** Stated`
**When** the user runs `specscore lesson recur <slug>`
**Then** one typed child Occurrence is appended, the README remains byte-identical, the derived count increments, and `**Status:**` remains `Stated`.

### AC: not-enforced-and-min-recurred-compose (verifies REQ:recurrence-is-queryable)

**Given** a lesson `flaky-check` in `**Status:** Stated` with `**Recurred:** 2`, and a lesson `quiet-check` in `**Status:** Stated` with `**Recurred:** 0`
**When** the user runs `specscore lesson list --not-enforced --min-recurred=1`
**Then** stdout lists only `flaky-check`.

### AC: recur-against-retired-lesson-warns (verifies the recurrence-graduation-signal behavior)

**Given** a lesson in `**Status:** Withdrawn`
**When** the user runs `specscore lesson recur <slug>`
**Then** the command exits `0`, the `**Recurred:**` count still increments, and stderr carries a warning naming the lesson and its retired status.

### AC: directory-form-remains-flat-compatible

**Given** one flat Lesson and one directory-form Lesson in the same project
**When** the user runs the Lesson read commands
**Then** both are addressable and linted, neither is relocated, and a duplicate slug across layouts is rejected explicitly.

### AC: occurrence-capture-is-lossless-and-lazy

**Given** a directory Lesson and explicit JSON context
**When** an agent records and retrieves an occurrence
**Then** context is preserved, lifecycle is unchanged, and no ambient live-agent context is read.

### AC: legacy-import-requires-reviewed-mapping

**Given** a historical prose log with ambiguous or duplicate identifiers
**When** import dry-run and reviewed apply run
**Then** dry-run writes nothing, apply preserves source bytes, and unresolved identity/status choices cannot be guessed.

### AC: relations-are-human-confirmed

**Given** two potentially overlapping Lessons
**When** a relation is proposed without its confirmation token
**Then** no relation or status changes; a confirmed duplicate retires only the retained duplicate with pointers to the unchanged canonical Lesson, remains queryable from either endpoint, and cannot form a supersession cycle.

### AC: ownership-and-enforcement-evidence-are-linted

**Given** a Lesson missing required tracking, or an Enforced Lesson with an unresolvable local evidence path
**When** `specscore spec lint` runs
**Then** it reports an error; a complete Lesson passes shared format/status/footer validation through the generic registry.

### AC: recurrence-policy-is-ci-visible

**Given** a non-Enforced Lesson with a recurrence
**When** CI runs `lesson check --not-enforced --min-recurred=1`
**Then** it lists the Lesson and exits nonzero without changing it.

### AC: lesson-events-replay-per-subscriber

**Given** a successful occurrence mutation and two durable subscribers, one failing
**When** the failed subscriber is replayed later
**Then** the mutation emits one durable event, the successful peer is not redelivered, and the failed peer receives the original UUID.

### AC: coordination-delegates-live-work

**Given** durable Synchestra links for generic efforts associated through the configured adapter with a Lesson's canonical external-resource reference
**When** a user lists them offline, then explicitly requests open/message/resume
**Then** local facts render without network access and only the configured authoritative Synchestra public interface handles the live action; SpecScore stores no message or backend-specific state, and Synchestra receives no structured Lesson semantics.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
