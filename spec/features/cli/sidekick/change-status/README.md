---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Sidekick Change-Status

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/sidekick/change-status?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/sidekick/change-status?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/sidekick/change-status?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/sidekick/change-status?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Summary

`specscore sidekick change-status <slug> --to=<status>` transitions a sidekick-seed from `Queued` to a terminal status (`Implemented`, `Rejected`, or `Archived`), relocating it to `spec/ideas/archived/<slug>.md` and tagging it `type: sidekick-seed`. Implements the [lifecycle-transitions](../../lifecycle-transitions/README.md) shared contract; extends it with a seed-kind relocation side effect and a reason-required `Rejected` transition.

## Synopsis

```
specscore sidekick change-status <slug> --to=<status> [--note <markdown>] [--project <path>]
```

## Problem

A sidekick-seed has no CLI-driven path to a terminal status. `specscore idea change-status` resolves slugs to `spec/ideas/<slug>.md` (Ideas only) — pointed at a seed it exits `3` (NotFound); `feature change-status` is Feature-only; `specscore sidekick` exposes only `new`. The only way to close a seed today is a manual frontmatter hand-edit (the one closed seed in the wild used `status: completed`), which skips state-machine validation, forgets index sync, and violates the SpecScore tenet that every status transition goes through a CLI verb. This verb closes the gap for the seed kind, mirroring `idea change-status` and inheriting the shared contract.

Captured in `specstudio-skills` seeds `cli-and-sidekick-skill-need-a-seed-change-status-close-verb` and `should-there-be-a-close-lifecycle-skill-that-retires-an`; tracked as [specscore-cli#72](https://github.com/specscore/specscore-cli/issues/72). The cross-repo seed link is recorded here in prose because `**Source Ideas:**` resolves only same-repo slugs.

## Behavior

This verb inherits every cross-cutting rule from [lifecycle-transitions](../../lifecycle-transitions/README.md) — strict state machine, atomic mutation + `spec lint --fix` index sync, rollback, positional slug, `<id>: <from> → <to>` success line, exit-code mapping, and the shared `--note`/reason-required mechanism. The REQs below are the seed-specific declarations.

### Legal-transition matrix

| From | To | Side effects |
|---|---|---|
| `Queued` | `Implemented` | Status rewrite + relocate to `spec/ideas/archived/` + add `type: sidekick-seed` + index sync |
| `Queued` | `Rejected` | Same side effects (**reason required** — `--note` mandatory) |
| `Queued` | `Archived` | Same side effects |

#### REQ: legal-transition-matrix

The verb MUST accept only the `(from, to)` pairs in the matrix above; any other pair MUST exit `4` (InvalidTransition) per [lifecycle-transitions#req:state-machine-strictness](../../lifecycle-transitions/README.md#req-state-machine-strictness), naming the current status and the legal target set. The legal source set for every target is `{Queued}`; per the Meta's [not-idempotent](../../lifecycle-transitions/README.md#req-not-idempotent) invariant no target is its own source, so re-running on an already-terminal seed is rejected (in practice it has already left `seeds/`, so the canonical lookup misses it and the verb exits `3`).

#### REQ: target-status-flag

The verb MUST accept the target via a required `--to=<status>` flag whose value is one of `Implemented`, `Rejected`, `Archived`. Matching is case-insensitive on input; the canonical title-case value is what gets written to the file and the success line. An unrecognized value MUST exit `2` (InvalidArgs) BEFORE state-machine validation. Mirrors [`idea change-status` target-status-flag](../../idea/change-status/README.md#req-target-status-flag).

#### REQ: seed-slug-resolution

The `<slug>` positional MUST resolve to `spec/ideas/seeds/<slug>.md` within the project root (autodetected or `--project`). Seeds already relocated to `spec/ideas/archived/<slug>.md` MUST NOT be matched, per [lifecycle-transitions#req:slug-resolves-to-existing-artifact](../../lifecycle-transitions/README.md#req-slug-resolves-to-existing-artifact). A missing file at the seeds path MUST exit `3` (NotFound) naming the expected path.

#### REQ: seed-status-surface

A sidekick-seed carries its status in the frontmatter `status:` key and has no body `**Status:**` line today. For the seed kind, the Meta's [status-line-rewrite](../../lifecycle-transitions/README.md#req-status-line-rewrite) is satisfied by line-targeted rewrite of the frontmatter `status:` value (every other byte unchanged). Introducing a body `**Status:**` line for seeds is owned by the upstream `artifact-frontmatter-convention` follow-on and is out of scope here.

#### REQ: terminal-relocation

All three targets are terminal. After a successful status rewrite, the verb MUST move the file from `spec/ideas/seeds/<slug>.md` to `spec/ideas/archived/<slug>.md` (mkdir-p the `archived/` directory if absent) and add the frontmatter key `type: sidekick-seed` — seeds are identified by location while under `seeds/`, and the explicit type is added at archive time, mirroring [`idea promote`](../../idea/promote/README.md). If `spec/ideas/archived/<slug>.md` already exists, the verb MUST exit `1` (Conflict) and restore the source seed (original frontmatter `status:`, original `seeds/` location, no `type:` added) before returning.

#### REQ: rollback-includes-relocation

Extends the Meta's [rollback-on-lint-failure](../../lifecycle-transitions/README.md#req-rollback-on-lint-failure): on any failure after the status rewrite (archive collision, file-move failure, lint failure, I/O error, or a `--note` write failure), the verb MUST restore the on-disk state to its exact pre-invocation form — file at `spec/ideas/seeds/<slug>.md`, original frontmatter `status:`, no `type:` key added, no `## Resolution` section written. Partial state MUST NOT be observable after the command returns. Exit `1` for collision, `10` for I/O or post-relocation lint failure.

#### REQ: reason-required-rejected

The `Queued → Rejected` transition is **reason-required** per [lifecycle-transitions#req:reason-required-transitions](../../lifecycle-transitions/README.md#req-reason-required-transitions): `--note <markdown>` is mandatory. A missing or empty/whitespace-only `--note` on `--to=Rejected` MUST exit `2` (InvalidArgs) before any mutation, with a stderr message naming the `Rejected` transition and stating that a reason is required. The `Implemented` and `Archived` transitions keep `--note` optional; when supplied, the note is written per [lifecycle-transitions#req:optional-transition-note](../../lifecycle-transitions/README.md#req-optional-transition-note) (a `## Resolution` section in the relocated seed).

## Rehearse Integration

Every AC below has a CLI surface (exit code + on-disk effect) and is exercised by the verb's Go test suite at implementation time. Standalone Rehearse `_tests/` stubs are deferred — the heuristic's CLI-surface stubs would duplicate the Go tests one-to-one; revisit if a black-box scenario suite is wanted later.

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [lifecycle-transitions](../../lifecycle-transitions/README.md) | Defines every cross-cutting REQ this verb satisfies; this verb extends `status-line-rewrite` with relocation and consumes the `reason-required-transitions` mechanism for `Rejected`. |
| [`idea/change-status`](../../idea/change-status/README.md) | Sibling verb; the archive-relocation + collision pattern is mirrored from it. |
| [`idea/promote`](../../idea/promote/README.md) | Precedent for moving a seed to `spec/ideas/archived/` and adding `type: sidekick-seed`. Sets `status: promoted`; this verb sets `Implemented`/`Rejected`/`Archived`. Neither sets the consilium-owned `deprecated`. |
| [spec lint](../../spec/lint/README.md) | Invoked internally for index sync; also owns the `sidekick-seed` rule whose status enum must accept the new terminal values (see Open Questions). |

## Dependencies

- cli/lifecycle-transitions

## Acceptance Criteria

### AC: implemented-relocates-and-tags

**Scenario:** shipped-directly seed closed as Implemented
**Given** a seed at `spec/ideas/seeds/foo.md` with frontmatter `status: queued`
**When** `specscore sidekick change-status foo --to=implemented` runs
**Then** the file is at `spec/ideas/archived/foo.md` with frontmatter `status: Implemented` and `type: sidekick-seed`, no file remains at `spec/ideas/seeds/foo.md`, stdout is exactly `foo: Queued → Implemented`, and `specscore spec lint` reports no new violations.

### AC: unrecognized-target-rejected

**Scenario:** `--to` value outside the seed terminal set
**Given** a seed at `spec/ideas/seeds/foo.md` with `status: queued`
**When** `specscore sidekick change-status foo --to=stable` runs
**Then** the verb exits `2` (InvalidArgs) before any mutation, and `foo.md` is unchanged at `spec/ideas/seeds/foo.md`.

### AC: rejected-requires-note

**Scenario:** negative transition without reasoning
**Given** a seed at `spec/ideas/seeds/bar.md` with `status: queued`
**When** `specscore sidekick change-status bar --to=rejected` runs with no `--note`
**Then** the verb exits `2` (InvalidArgs), stderr names the `Rejected` transition and states a reason is required, and `bar.md` is unchanged at `spec/ideas/seeds/bar.md`.

### AC: rejected-with-note-writes-resolution

**Scenario:** negative transition with reasoning
**Given** a seed at `spec/ideas/seeds/bar.md` with `status: queued`
**When** `specscore sidekick change-status bar --to=rejected --note "Superseded by the consolidated close skill"` runs
**Then** the file is at `spec/ideas/archived/bar.md` with `status: Rejected` and `type: sidekick-seed`, its body contains a `## Resolution` section whose text includes `Superseded by the consolidated close skill`, and stdout is exactly `bar: Queued → Rejected`.

### AC: note-optional-on-implemented

**Scenario:** optional note on a positive transition
**Given** a seed at `spec/ideas/seeds/baz.md` with `status: queued`
**When** `specscore sidekick change-status baz --to=implemented --note "Shipped in skills/implement step 4b"` runs
**Then** the archived seed body at `spec/ideas/archived/baz.md` contains a `## Resolution` section including `Shipped in skills/implement step 4b`; and the same command without `--note` writes no `## Resolution` section.

### AC: non-queued-source-rejected

**Scenario:** strict source-state check
**Given** a file at `spec/ideas/seeds/qux.md` whose frontmatter `status:` is not `queued`
**When** `specscore sidekick change-status qux --to=implemented` runs
**Then** the verb exits `4` (InvalidTransition), stderr names the current status and the legal source set `{Queued}`, and `qux.md` is unchanged.

### AC: archive-path-collision

**Scenario:** a file already occupies the archive destination
**Given** a seed at `spec/ideas/seeds/foo.md` with `status: queued` AND an existing file at `spec/ideas/archived/foo.md`
**When** `specscore sidekick change-status foo --to=archived` runs
**Then** the verb exits `1` (Conflict), `spec/ideas/seeds/foo.md` still has `status: queued`, and `spec/ideas/archived/foo.md` is untouched.

### AC: slug-not-found

**Scenario:** no seed at the canonical path
**Given** no file at `spec/ideas/seeds/nope.md`
**When** `specscore sidekick change-status nope --to=implemented` runs
**Then** the verb exits `3` (NotFound) with a message naming the expected `spec/ideas/seeds/nope.md` path.

## Open Questions

- **Seed status enum (upstream dependency).** Does the `sidekick-seed` lint rule — owned upstream by `specscore/specscore`'s [`artifact-frontmatter-convention`](https://github.com/specscore/specscore/blob/main/spec/features/artifact-frontmatter-convention/README.md) — accept `status ∈ {Implemented, Rejected, Archived}` for archived seeds? `idea promote` already produces lint-clean archived seeds (`status: promoted` + `type: sidekick-seed`), so the rule accepts archived-location seeds with a non-`queued` status and a `type`; confirming/extending the enum to these three values is a tracked `specscore/specscore` dependency, not resolved by this verb.
- **Body-size cap at terminal states.** ~~The 2000-char seed body cap is a capture-time forcing function for `Queued` seeds; a `## Resolution` note on a terminal seed may exceed it.~~ **Resolved:** the `sidekick-seed` lint rule is now status-dependent — a queued seed gets a 3000-char hard cap (2500-char advisory warning), and a closed/terminal seed gets a 5000-char cap, leaving room for a `## Resolution` note.
- **`Rejected` vs `Deprecated`.** This verb's manual `Rejected` coexists with the consilium's `Deprecated`. Reconciling the two into one reject terminal is a deferred follow-up (would touch the consilium feature).

---
*This document follows the https://specscore.md/feature-specification*
