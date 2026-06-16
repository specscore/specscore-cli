---
format: https://specscore.md/plan-specification
status: Implementing
---
# Plan: Cli Sidekick Change Status

**Status:** Implementing
**Source Feature:** cli/sidekick/change-status
**Date:** 2026-06-16
**Owner:** alexandertrakhimenok
**Supersedes:** —

## Summary

Implements the `specscore sidekick change-status` verb in `specscore-cli` (Go), retiring a sidekick-seed from `Queued` to a terminal status (`Implemented`, `Rejected`, `Archived`) with relocation to `spec/ideas/archived/` and a `type: sidekick-seed` tag. It also lands the shared `lifecycle-transitions` `--note`/reason-required plumbing (F0) — this verb is its first consumer, and the verb's ACs are what verify it.

## Approach

Build bottom-up. First the shared plumbing in the lifecycle-transition path (optional `--note` → `## Resolution` append; a reason-required designation → exit `2`), because the seed verb's note/reason ACs are what verify that contract. Then the seed kind itself: slug resolution + frontmatter status surface, the legal-transition matrix + `--to` validation + strict source check, the terminal relocation + collision handling (mirroring the seed→archived move in `internal/cli/idea_promote.go`), and finally wiring the `sidekick change-status` cobra command + `spec lint --fix` index sync and the end-to-end happy / rejected-with-note paths. The order is strictly linear: shared plumbing → resolution → matrix → relocation → command wiring. The verb mirrors `runIdeaChangeStatus` in `internal/cli/idea.go`.

## Tasks

### Task 1: Shared optional `--note` → `## Resolution` plumbing

**Verifies:** cli/sidekick/change-status#ac:note-optional-on-implemented
**Depends-On:** —
**Status:** done

Add the optional `--note <markdown>` flag to the shared lifecycle-transition path used by the change-status verbs. When non-empty, append a `## Resolution` section to the artifact body verbatim (create it before the footer / at EOF, or append a paragraph to an existing one), as part of the same atomic mutation as the status rewrite, with rollback on any failure. An empty or absent `--note` is a no-op.

### Task 2: Shared reason-required-transition mechanism

**Verifies:** cli/sidekick/change-status#ac:rejected-requires-note
**Depends-On:** 1
**Status:** done

Let a verb designate specific transitions as reason-required. For a designated transition, a missing or empty/whitespace-only `--note` MUST exit `2` (InvalidArgs) before any mutation, with a message naming the transition. Non-designated transitions keep `--note` optional.

### Task 3: Seed slug resolution + frontmatter status surface

**Verifies:** cli/sidekick/change-status#ac:slug-not-found
**Depends-On:** 2
**Status:** pending

Resolve `<slug>` to `spec/ideas/seeds/<slug>.md` within the project root, excluding already-relocated seeds under `spec/ideas/archived/`. Read and line-target-rewrite the frontmatter `status:` value (the seed kind's canonical status surface — no body `**Status:**` line). A missing seed at the canonical path exits `3` (NotFound).

### Task 4: Legal-transition matrix + `--to` flag + strict source check

**Verifies:** cli/sidekick/change-status#ac:unrecognized-target-rejected, cli/sidekick/change-status#ac:non-queued-source-rejected
**Depends-On:** 3
**Status:** pending

Declare the seed matrix `Queued → {Implemented, Rejected, Archived}`. Validate `--to` (unrecognized value exits `2` before state-machine validation; case-insensitive, canonical title-case persisted). Enforce the strict source check (a non-`Queued` source exits `4`). Designate `Queued → Rejected` as reason-required, consuming Task 2's mechanism.

### Task 5: Terminal relocation + `type` tag + collision + rollback

**Verifies:** cli/sidekick/change-status#ac:archive-path-collision
**Depends-On:** 4
**Status:** pending

After the status rewrite, move the seed to `spec/ideas/archived/<slug>.md` (mkdir-p) and add the frontmatter key `type: sidekick-seed`, mirroring the seed→archived move in `internal/cli/idea_promote.go`. A pre-existing file at the archived path exits `1` (Conflict) and restores the source seed; any post-rewrite failure rolls back to the original `seeds/` state (original `status:`, no `type:`, no `## Resolution`).

### Task 6: Wire `sidekick change-status` command + index sync + end-to-end

**Verifies:** cli/sidekick/change-status#ac:implemented-relocates-and-tags, cli/sidekick/change-status#ac:rejected-with-note-writes-resolution
**Depends-On:** 5
**Status:** pending

Register `sidekickChangeStatusCommand` / `runSidekickChangeStatus` in `internal/cli/sidekick.go`, mirroring `runIdeaChangeStatus`. Wire the `spec lint --fix` index sync and the `<slug>: <from> → <to>` success line. Complete and exercise end-to-end the happy path (`--to=implemented` relocates + tags) and the `--to=rejected --note …` path (relocates + writes `## Resolution`).

## Open Questions

- **Upstream lint-enum prerequisite (gates Tasks 5–6).** The relocation tasks write archived seeds with `status: Implemented|Rejected|Archived` + `type: sidekick-seed`, then run `spec lint --fix` for index sync. This stays clean only if the upstream `sidekick-seed` lint rule (owned by `specscore/specscore` `artifact-frontmatter-convention`) accepts those three terminal values — the source Feature's first Open Question. `idea promote` already produces lint-clean archived seeds (`status: promoted` + `type`), so the rule accepts archived seeds; confirm/extend the enum before implementing Tasks 5–6 or their `lint --fix` will fail and roll back.

---
*This document follows the https://specscore.md/plan-specification*
