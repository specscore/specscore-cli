---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: Parked (Shared Scheduling Axis)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/parked?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/parked?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/parked?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/parked?op=request-change) |
**Status:** Draft
**Source Ideas:** —

## Summary

This is **not a command group** — like [`cli/lifecycle-transitions`](../lifecycle-transitions/README.md), it is a shared cross-cutting contract: a `Parked` axis, orthogonal to `**Status:**`, that any doc kind carrying a `**Status:**` header line can adopt. `specscore idea park`/`unpark` and `specscore feature park`/`unpark` are its two current adopters. Parking marks an artifact as deliberately deferred — a scheduling decision ("when will we build it?") — without touching `**Status:**`, which answers a different, independent question ("how ready is it?").

The shared engine lives at `pkg/lifecycle.SetParked`/`ClearParked`/`ReadParked` (kind-agnostic: it operates on any file carrying a `**Status:**` line, the same way `pkg/lifecycle.Rewrite` powers every kind's `change-status`). Per-kind CLI verbs are thin wrappers that resolve the artifact path and call the shared engine — mirroring how `idea change-status`/`feature change-status` are thin wrappers over `pkg/lifecycle.Validate`/`Rewrite`/`Rollback`.

## Problem

A founder ratified nine sub-Features; six ship in v1 and three are good ideas deliberately held back. None of the eight statuses in the closed vocabulary (`Draft`, `In Review`, `Approved`, `Implementing`, `Stable`, `Amending`, `Rejected`, `Deprecated`) fit: `Approved` implies queued toward `Implementing`, and `Rejected`/`Deprecated` both read as "wrong or retired" — which is false; the three deferred sub-Features are fully specced and ratified (high maturity), just not-this-release (a scheduling decision). Recording the deferral only as prose under a `## Decided` heading, while `**Status:**` kept saying `Draft`, left the index disagreeing with the decision that was actually made — only a human reading the prose could tell. This is recorded as lesson L151 in `sneat-co/backstage/LESSONS-LEARNED.md`.

## Behavior

### The Parked axis

#### REQ: orthogonal-to-status

Parked is a SCHEDULING axis; `**Status:**` is a MATURITY axis. They are independent: a fully-specced, ratified (high-maturity) artifact can be parked, and an early Draft can be parked too. Parking MUST NEVER change `**Status:**`. Unparking restores nothing, because nothing was taken away. Park/unpark are NOT lifecycle transitions: they carry no entry in any kind's legal-transition matrix (`pkg/lifecycle`'s `transitionMatrix`) and are never gated on the artifact's current status.

#### REQ: header-representation

The axis is represented by a contiguous three-line body-metadata block, inserted immediately after the `**Status:**` line — the same convention `**Superseded By:**`/`**Supersedes:**` and Idea's `**Archived:**`/`**Archive Note:**` already use for structured facts orthogonal to (or paired with) `**Status:**`. Immediately following an existing `**Status:** Approved` line, `park` inserts, in order: a `**Parked:**` line valued `true`; a `**Parked Reason:**` line carrying the `--reason` text; and a `**Parked Date:**` line carrying today's date (UTC, `YYYY-MM-DD`).

An absent `**Parked:**` line, or any value other than `true`, means not parked — mirroring Idea's `**Archived:**` convention. The block is body-metadata only; there is no frontmatter mirror (the same choice Idea's `**Archived:**` axis makes — see [`cli/idea/archive`](../idea/archive/README.md)).

#### REQ: reason-and-date-required

`park` MUST accept a required `--reason` flag. A missing, empty, or whitespace-only `--reason` MUST be rejected (exit `2`, InvalidArgs) BEFORE any mutation — a bare `**Parked:** true` with no explanation is exactly the graveyard this axis exists to prevent. `**Parked Date:**` is stamped automatically (today, UTC, `YYYY-MM-DD`) and is never a user-supplied flag. Both fields are REQUIRED companions of `**Parked:** true`; the `parked-shape` lint rule (below) catches a hand-edited file that sets the flag without them.

#### REQ: repark-overwrites

Re-running `park` on an already-parked artifact overwrites `**Parked Reason:**` and resets `**Parked Date:**` to today, rather than erroring or duplicating the block. This is the mechanism for confirming "still deliberately deferred": re-park with a fresh `--reason`.

#### REQ: unpark-requires-parked

`unpark` removes all three header lines. If the artifact carries no `**Parked:** true` axis, `unpark` MUST exit `4` (InvalidState) naming the artifact rather than silently succeeding — a no-op would mask a mistyped slug/feature_id the caller believed was already parked.

### Verbs

#### REQ: idea-verbs

`specscore idea park <slug> --reason "..."` and `specscore idea unpark <slug>` act on `spec/ideas/<slug>.md` in place. Unlike `idea archive`/`unarchive`, neither verb relocates the file.

#### REQ: feature-verbs

`specscore feature park <feature_id> --reason "..."` and `specscore feature unpark <feature_id>` act on `spec/features/<feature_id>/README.md` in place.

#### REQ: post-mutation-lint-and-rollback

Both verb pairs, for both kinds, run `specscore spec lint --fix` after a successful write (syncing any index row) and roll back to the pre-invocation byte-for-byte state if a fix-pass I/O error or a post-fix error-severity violation occurs anywhere in the tree — the same rollback contract every `change-status`/`archive` verb in [`cli/lifecycle-transitions`](../lifecycle-transitions/README.md) honors.

### Listing

#### REQ: list-parked-filter

`idea list` and `feature list` each accept a `--parked` boolean flag that filters the listing to ONLY parked artifacts, excluding unparked ones. Parked is NOT a visibility axis like Idea's `**Archived:**`: the default listing (no `--parked`) shows both parked and unparked artifacts — a parked Idea/Feature is still active work, just deferred. `idea list --format=yaml/json` additionally surfaces a `parked: <bool>` field per entry; `feature list --fields=parked` does the same via the existing metadata-fields mechanism.

### Lint

#### REQ: parked-shape

The `parked-shape` lint rule (error severity) fires when `**Parked:** true` is present without a non-empty `**Parked Reason:**`, without a `**Parked Date:**`, or with a `**Parked Date:**` that does not parse as `YYYY-MM-DD`. It scans every markdown artifact generically (the same kind-agnostic `**Key:** value` scan `grade-value` uses for `**Grade:**`), so it applies uniformly regardless of which kinds adopt the axis. An absent `**Parked:**` axis is valid and produces no violations.

#### REQ: parked-stale

The `parked-stale` lint rule (warning severity) fires when a parked artifact's `**Parked Date:**` is older than the configured review window — surfacing parked artifacts the same way `specscore lesson list --not-enforced` surfaces lessons that still need graduating, so a park is never silently forgotten. Warning (not error) severity was chosen deliberately: the founder's own precedent (`lesson --not-enforced`) is a non-blocking surfacing query, and an error-severity gate would suddenly fail `spec lint` for every pre-existing parked artifact the moment the window elapses, with no action available except re-affirming or unparking — too blunt for a scheduling signal.

#### REQ: stale-threshold-configurable

The review window defaults to 90 days (one quarter) — long enough that a deliberately-deferred artifact is not nagged the week after it is parked, short enough to force at least a quarterly look, matching a typical roadmap-review cadence. It is configurable via an optional `parked.stale_days` key in `specscore.yaml`:

```yaml
parked:
  stale_days: 30
```

A zero, negative, or absent value falls back to the 90-day default (`pkg/projectdef.SpecConfig.EffectiveParkedStaleDays`).

#### REQ: decided-not-parked-not-implemented

A lint rule that flags "an artifact's `## Decided` (or equivalent) section records a deferral while nothing machine-readable says parked" — the exact contradiction that motivated this feature — was investigated and deliberately NOT implemented. No canonical `## Decided` (or equivalent) heading exists anywhere in this repo's Feature/Idea schema; the founder's originating case used it as ad-hoc prose, not a recognized section. Every existing lint rule in this codebase checks a MECHANICAL fact (a field is present, a value is in an enum, a cross-reference resolves) — none classify prose meaning. Detecting "records a deferral" requires natural-language content classification: a coarser presence-of-heading heuristic (flag any `## Decided` section on a non-parked artifact) would false-positive on any unrelated "we decided X" content and false-negative on deferred content phrased without that heading — precisely the "fragile heuristic" this rule must not become. `parked-stale` and the `--parked` list filter are the mechanical mitigations this feature ships instead: they make an ALREADY-parked artifact impossible to forget, and `park`'s required `--reason` makes parking a deliberate, auditable, one-command action — so the fix for the original incident is "make parking easy and durable enough that a founder reaches for `idea park --reason` instead of writing prose," not "detect after the fact that prose should have been a flag."

### Scope: which kinds

#### REQ: kinds-covered

Feature and Idea are covered — the founder's originating case (nine ratified sub-Features) and its closest sibling artifact. Both already carry a `**Status:**` header line via [`cli/lifecycle-transitions`](../lifecycle-transitions/README.md)'s shared engine, so `pkg/lifecycle.SetParked`/`ClearParked` applies to both without modification.

#### REQ: kinds-deliberately-not-covered

Plan, Task, Lesson, Decision, and Issue are deliberately NOT covered by this feature:

- **Plan** shares the `**Status:**` convention and COULD reuse the identical engine; Plan's `Withdrawn` disposition has the same "abandoned" framing gap this feature closes for Feature/Idea. It is the closest follow-on candidate, out of scope here because the founder's concrete case is about Features, and a Plan is normally scaffolded only once its source Feature is already Approved and queued — the natural place to park is one level up, before the Plan exists.
- **Issue** uses YAML frontmatter exclusively and is not a member of `pkg/lifecycle`'s `Kind` enum at all (it has no body `**Status:**` line), so it does not share this feature's file convention; adopting it would need a structurally different (frontmatter-key) implementation. Substantively "this bug is real but not being worked now" is a plausible parked use case, making Issue the second-best follow-on candidate.
- **Task** is a fast-moving execution unit inside an already-approved Plan (`planning → queued → in_progress → blocked → complete/failed/aborted`); "parked" as a scheduling-horizon concept does not fit — the closest existing state is simply not-yet-`queued`.
- **Lesson** climbs an enforcement ladder (`Recorded → Stated → Enforced`) or retires (`Withdrawn`/`Superseded`); it is not a piece of future work to schedule.
- **Decision** records a choice already made; `D-immutability-once-accepted` freezes its content specifically because it documents history, not a future intent. Parking a Decision is a category error — you would write a new Draft Decision when the time comes, not defer the old one.

## Acceptance Criteria

### AC: park-requires-reason

**Given** an Idea or Feature with any `**Status:**`
**When** `park <id>` is run with a missing, empty, or whitespace-only `--reason`
**Then** the command exits `2` (InvalidArgs) and the artifact is byte-for-byte unchanged.

### AC: park-preserves-status

**Given** an Idea or Feature in any status (Draft, Approved, Implementing, ...)
**When** `park <id> --reason "..."` succeeds
**Then** the `**Status:**` line is unchanged, and `**Parked:** true`, a non-empty `**Parked Reason:**`, and a `**Parked Date:** YYYY-MM-DD` are present immediately after it.

### AC: unpark-round-trips

**Given** an Idea or Feature that was parked via `park --reason "..."`
**When** `unpark <id>` is run
**Then** the artifact is restored byte-for-byte to its pre-park state (the three header lines removed, `**Status:**` untouched throughout), and `unpark` on an artifact that was never parked exits `4` (InvalidState) instead of silently succeeding.

### AC: list-parked-filter-isolates-parked

**Given** a project with one parked and one unparked Idea (or Feature)
**When** `idea list --parked` (or `feature list --parked`) is run
**Then** only the parked artifact's identifier is printed, and the same listing WITHOUT `--parked` prints both.

### AC: staleness-lint-fires-at-boundary

**Given** `parked.stale_days` configured to N and an artifact parked exactly N days ago, and a second artifact parked N+1 days ago
**When** `specscore spec lint` runs
**Then** the N-day artifact reports no `parked-stale` violation and the N+1-day artifact reports exactly one `parked-stale` warning naming the age and the window.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
