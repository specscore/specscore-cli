# Idea: Lifecycle Verbs for Idea and Feature

**Status:** Implemented
**Date:** 2026-05-18
**Owner:** alexander.trakhimenok
**Promotes To:** cli/feature/change-status, cli/idea/change-status, cli/lifecycle-transitions
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we let the specscore CLI mutate the lifecycle Status of Idea and Feature artifacts — with validated state-machine transitions and atomic index/lint synchronization — so spec authors and downstream tooling can advance state programmatically without hand-editing Markdown files, while staying out of the coordination/concurrency lane that external workflow orchestrators own?

## Context

> **Implementation note.** Both `change-status` verbs and the shared `pkg/lifecycle/` package have shipped — `cli/idea/change-status`, `cli/feature/change-status`, and `cli/lifecycle-transitions` are all live Features with `**Source Ideas:** lifecycle-verbs-for-idea-and-feature`. The walker fix in d7b52fa unblocked the auto-derivation that promoted this Idea from Approved to Implementing. The original-state framing below is preserved as the design rationale for the shipped contract; it is no longer a description of the present.

Before this Idea, the `specscore` CLI was read-only after `<kind> new` for every document kind. The command tree exposed `feature`, `idea`, `task`, `code`, `spec`, and `init` — and every mutation surface was creation-only. Transitioning an Idea `Draft → Approved` required hand-editing the `**Status:**` line followed by `specscore spec lint --fix` to sync the ideas-index row. There was no command to validate that the transition was legal, no shared state-machine package, and no way for downstream tooling (CI, agent skills, web dashboards) to advance state programmatically.

The CLI's own task command spec ([`spec/features/cli/task/README.md:41`](../features/cli/task/README.md)) names this gap directly: *"transition semantics (who can claim, how conflicts are resolved, when status becomes terminal) warrant their own feature spec."* Its Outstanding Questions list asks: *"When should lifecycle commands (`task claim`, `task release`, `task status`) land?"* This Idea is the architectural answer to that question — but only for Idea and Feature; Task is deliberately deferred per the positioning below.

Task lifecycle requires concurrency control, claim/release semantics, sync policy, and conflict-aware exit codes — concerns that are out of scope for `specscore`'s local-file-mutation model. Those concerns are owned by external workflow orchestrators, not by `specscore`. This leaves Idea and Feature lifecycle as clean greenfield for `specscore`, with the `task` doc kind explicitly out of scope.

## Recommended Direction

Ship **one `change-status` verb per doc kind** on the `specscore` CLI — `specscore idea change-status <slug> --to=<status>` and `specscore feature change-status <feature_id> --to=<status>`. Each verb validates that the target status appears in the kind's legal-transition matrix given the artifact's current `**Status:**`, mutates the file atomically, and invokes `spec lint --fix` to keep the corresponding index row in sync. State-machine logic lives in a shared internal package (`pkg/lifecycle/`) so the legal-transition graph for both kinds is defined once.

**Legal-transition matrix.**

| Kind | From → To | Side effects |
|---|---|---|
| idea | `Draft` → `Approved` | Status rewrite + ideas-index sync |
| idea | `{Draft, Under Review, Approved, Implementing, Specified}` → `Archived` | Status rewrite + file move from `spec/ideas/<slug>.md` to `spec/ideas/archived/<slug>.md` + active-index + archived-index sync |
| feature | `Draft` → `Under Review` | Status rewrite + features-index sync |
| feature | `{Draft, Under Review}` → `Approved` | Status rewrite + features-index sync |
| feature | `Approved` → `Implementing` | Status rewrite + features-index sync |
| feature | `Implementing` → `Stable` | Status rewrite + features-index sync |
| feature | `Stable` → `Deprecated` | Status rewrite + features-index sync |

Any transition outside the matrix exits `4` (`InvalidTransition`) per the CLI's [shared exit-code contract](../features/cli/README.md#shared-exit-code-contract) with a stderr message naming both the current status and the legal target statuses from the current state. Re-running on the target status is also exit `4`: idempotence is NOT carved out (state-machine strict).

The Idea `Specified` and `Implementing` transitions stay externally managed by lint and plan tools and are NOT user-facing in `change-status`; only `Approved` and `Archived` are legal `--to` values for Ideas. For Features, all five forward transitions in the matrix are legal `--to` values. `idea reopen` (`Approved → Draft`) and `feature undeprecate` (`Deprecated → Stable`) are **deferred** until a real reuse pattern surfaces.

**Side-effect for `--to=archived` (Idea only).** Subsumed in the verb: when an Idea transitions to `Archived`, the verb rewrites the `**Status:**` line AND moves the file from `spec/ideas/<slug>.md` to `spec/ideas/archived/<slug>.md` (creating the archive directory if absent). A collision at the target path exits `1` (Conflict). Rollback covers both the status rewrite and the relocation. Documented explicitly in the Idea verb spec so the side effect is not surprising.

**Atomic mutation and rollback.** Index/lint sync is part of every transition — partial state (file says new status, index says old) MUST NOT be observable after the command returns. On any failure after the status rewrite (collision, lint failure, I/O), the verb restores the file to its pre-invocation form (status line and, for Idea-archive, file location).

**Architectural positioning** (explicit, not implicit): `specscore` ships local-file mutation primitives with state-machine validation. It does not provide concurrency, sync policies, claim/release semantics, or multi-agent coordination — those concerns are owned by external workflow orchestrators. For doc kinds where local-file mutation **is** the value (Idea, Feature) — transitions are deliberate, single-actor, contention-free — `specscore` is the canonical surface. For doc kinds where coordination **is** the value (Task) — concurrency, claim races, abort flags — an external orchestrator is the canonical surface. `specscore` MAY later mirror a thin task-status primitive as a standalone fallback for users not running an orchestrator, but that is a separate Idea whose answer hinges on the standalone use case, not on file mutation.

**Plugin-skill behavior (downstream).** The corresponding skill in [`ai-plugin-specscore`](https://github.com/specscore/ai-plugin-specscore) invokes `specscore <kind> change-status` directly when wrapping these verbs. `specscore` is the canonical surface for Idea and Feature lifecycle.

## Alternatives Considered

**Sugared per-transition verbs** (`specscore idea approve`, `idea archive`, `feature review`, `feature approve`, `feature implement`, `feature stabilize`, `feature deprecate`) — seven verbs across two doc kinds, each a thin wrapper around the shared contract. The previous version of this Idea recommended this approach; specs were authored and approved for two of the verbs before the direction reversed. **Rejected on revision** because (a) the CLI surface bloats to seven verbs across two kinds for what is fundamentally one operation per kind; (b) adding a new status value to a kind's enumeration requires a new feature spec and a new verb instead of a one-line transition-matrix edit; (c) the discoverability win of sugared verbs is recoverable via `--to=` validation in `--help` output. The cost we accept: less semantically rich commit messages.

**Generic primitive `specscore patch <kind> <id> --status <X>`** — one command total across kinds. Rejected because (a) per-kind subcommand placement matches the existing CLI command tree (`specscore <kind> <verb>`), keeping group ownership of state-machine matrices intact; (b) a top-level `patch` verb conflates fields beyond status (owner, tags, etc.) that are out of scope today and would muddy the `change-status` contract.

**Hybrid (primitive + sugared aliases)** — both `change-status` and a per-transition alias set ship. Rejected as **redundant for MVP**: two ways to do the same thing forces docs to explain both, doubles the test matrix. Revisit only if a heavy alias set proves valuable to script authors.

**Orchestrator-only ownership (specscore-cli stays read-only)** — defer all lifecycle to an external workflow orchestrator. Rejected because `specscore`'s standalone positioning requires it to be functionally complete on its own for the doc kinds where coordination isn't the bottleneck.

**Build event emission into transition commands from day one** — every transition writes to an event log or POSTs to a webhook. Rejected for MVP: event transport, schema, retry semantics, and authentication are real cross-cutting concerns that deserve a dedicated Idea (now realized as the [`event-emit-dispatcher`](event-emit-dispatcher.md) Idea, which ships independently and can later be wired into lifecycle transitions if a real consumer asks).

## MVP Scope

One cycle. Implement `specscore idea change-status` and `specscore feature change-status` with the full legal-transition matrix above. Ship a shared `pkg/lifecycle/` package that encodes the matrix per doc kind and is consumed by both cobra command files. Each verb runs `spec lint --fix` after the file mutation so the corresponding index row stays consistent. The `--to=archived` Idea transition triggers the file move to `spec/ideas/archived/` as a built-in side effect with rollback coverage. Exit code `4` (`InvalidTransition`) is wired through (already reserved in the CLI shared exit-code contract). Two new feature specs at `spec/features/cli/idea/change-status/` and `spec/features/cli/feature/change-status/`, each with a worked example per legal transition. Unit tests exhaustively cover the legal-transition matrix and every illegal-target rejection. Smoke test: a single integration test drives an Idea from `Draft` through `Approved` to `Archived` and a Feature from `Draft` through `Under Review`, `Approved`, `Implementing`, `Stable`, `Deprecated` — all via the two `change-status` verbs, no hand-edits. Out of MVP: Task lifecycle, event emission, owner mutation, `--reason` flags, plugin-skill updates, batch transitions.

## Not Doing (and Why)

- Task lifecycle in `specscore` — out of scope by design. The `task` doc kind's lifecycle requires concurrency control, sync policy, claim semantics, and conflict-aware exit codes that don't fit `specscore`'s local-file-mutation model; it is owned by external workflow orchestrators. Whether `specscore` should later mirror a thin `task status` primitive for standalone users is its own Idea.
- Owner mutation / assignment verbs — different shape (field overwrite, not state-machine transition). Belongs in a separate `field-mutation` Idea if a real need surfaces.
- Event emission inside transition commands — covered separately by the [`event-emit-dispatcher`](event-emit-dispatcher.md) Idea. Lifecycle transitions can later emit events via `specscore event emit` if a consumer asks; not in this Idea's MVP.
- Plugin (`ai-plugin-specscore`) skill updates — deferred. Once the CLI ships, the plugin update is mechanical (add `references/change-status.md` per the existing `feature/references/info.md` pattern, update the SKILL.md verb table). Separate follow-on Idea or a docs-only PR.
- `--reason` / `--message` audit-trail flags on transitions — deferred. The git commit produced by the user after the transition is the audit trail for MVP. Revisit if multi-actor workflows demand structured reason capture.
- Concurrency, locking, sync policies — by design, out of scope. These belong to external workflow orchestrators. `specscore` assumes single-actor file mutation.
- Programmatic transitions triggered from the lint engine — keep state changes user-initiated for MVP. Auto-transitions (e.g., "lint detects all sub-tasks completed → auto-stabilize feature") are tempting but mask author intent.
- Batch operations (`specscore idea change-status <slug-1> <slug-2> ... --to=approved`) — single-id MVP. Useful at planning time but adds complexity (partial-failure semantics, atomic rollback) that doesn't earn its weight until usage proves it.
- `idea reopen` (`Approved → Draft`) and `feature undeprecate` (`Deprecated → Stable`) — both real but rare. Defer until asked.

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | Idea and Feature transitions are genuinely contention-free in real usage — single-actor, deliberate, no claim races requiring coordination. | Survey actual workflows on 2–3 real consumer repos over the first month post-ship. Acceptance: zero reports of two actors racing on the same Idea/Feature transition; if any surface, escalate to an external-orchestrator-integration Idea. |
| Must-be-true | The legal-transition graph for Idea and Feature can be defined as a directed graph in code without controversy — a single canonical set of transitions is agreed upon. | Enumerate every legal transition before implementation; capture in a `pkg/lifecycle/states.go` table; review the matrix as a standalone PR; lock into a unit-test matrix that exhaustively covers `(from, verb, to)` triples. |
| Must-be-true | Running `spec lint --fix` after every transition stays fast enough not to harm UX on representative repos. | Benchmark on a 200-feature repo and a 50-Idea repo. Acceptance: every transition command (mutate + lint --fix) completes in under 250ms on a developer laptop; if it exceeds, the lint scope is narrowed to the affected index rather than full repo. |
| Should-be-true | The `change-status --to=<value>` shape is discoverable enough — users find the legal target statuses via `--help` output rather than guessing. | Ensure `specscore <kind> change-status --help` enumerates the legal-transition matrix for that kind in human-readable form. Acceptance: a new user can transition an artifact end-to-end using only `--help` output, no external docs. |
| Should-be-true | Standalone users (no external workflow orchestrator wired up) get a complete enough lifecycle experience from `specscore` alone that they do not fall back to hand-editing `**Status:**` lines as a workaround. | Dogfood on the SpecScore meta-spec repo itself for one development cycle. Acceptance: zero `**Status:**` hand-edits in the git log after the verbs ship. |
| Might-be-true | The same `pkg/lifecycle/` abstraction will later serve additional doc kinds (e.g., `proposal`, `entity`, `property` from the in-flight `entity-and-property-definitions` Idea) without refactor. | Design the package interface (`Transition(kind, from, to) (err error)`) parameterized on doc kind and target status, not hardcoded to `idea`/`feature`. Verify at design-review time. |
| Might-be-true | Exit code `4` (`InvalidTransition`) does not collide with any existing CLI exit code. | Audit the shared exit-code contract in `spec/features/cli/README.md#shared-exit-code-contract` before locking the number; pick the next free slot if `4` is taken. |


## SpecScore Integration

- **New Features this would create:**
  - `spec/features/cli/idea/change-status/` — `specscore idea change-status <slug> --to=<status>`. Legal `--to` values: `approved`, `archived`. `--to=archived` triggers file relocation to `spec/ideas/archived/`.
  - `spec/features/cli/feature/change-status/` — `specscore feature change-status <feature_id> --to=<status>`. Legal `--to` values: `under review`, `approved`, `implementing`, `stable`, `deprecated`. No file relocation.
  - `spec/features/cli/lifecycle-transitions/` (cross-cutting Meta) — already approved; defines the shared contract every `change-status` invocation satisfies.
- **Existing Features affected:**
  - [`spec/features/cli/idea/README.md`](../features/cli/idea/README.md) — Contents table gains `change-status/`; the *"MVP surface is `idea new`"* line revised to reflect lifecycle coverage; `REQ: ideas-only` reaffirmed (the verb MUST NOT mutate `spec/features/`).
  - [`spec/features/cli/feature/README.md`](../features/cli/feature/README.md) — Contents table gains `change-status/`.
  - [`spec/features/cli/task/README.md`](../features/cli/task/README.md) — the standing Outstanding Question *"When should lifecycle commands (`task claim`, `task release`, `task status`) land?"* is **closed** with a reference to this Idea's *Architectural positioning*: Task lifecycle is out of scope by design (owned by external workflow orchestrators).
  - [`spec/features/cli/README.md`](../features/cli/README.md) — shared exit-code contract already reserves exit code `4` (`InvalidTransition`); no change.
  - `pkg/lifecycle/` (new internal package) — `Transition(kind, from, to) (err error)` plus a per-kind legal-transition matrix; consumed by `internal/cli/idea.go` and `internal/cli/feature.go`.
  - `pkg/lint/` — gains the `feature-index-row-sync` rule mirroring `idea-index-row-sync`; re-invoked from each transition command after the file mutation. The rule's row-sync contract is defined in the meta-spec's [`features-index`](https://github.com/specscore/specscore/blob/main/spec/features/features-index/README.md) feature.
  - `internal/cli/idea.go`, `internal/cli/feature.go` — new cobra subcommand wiring (one new subcommand per file).
  - [`ai-plugin-specscore`](https://github.com/specscore/ai-plugin-specscore) plugin — **follow-on, not in this Idea.** The downstream skill updates (one `references/change-status.md` per kind, plus SKILL.md table updates) invoke `specscore <kind> change-status` directly. `specscore` is the canonical path for Idea and Feature lifecycle.
- **Dependencies:** None blocking. Builds on existing `spec lint --fix` infrastructure (specifically `idea-index-row-sync` + the new `feature-index-row-sync` rule) and the existing exit-code contract.

## Open Questions

Resolved post-ship (the shipped Features carry the canonical answers; this list is the historical record):

- **`idea reopen` (`Approved → Draft`) in MVP?** Deferred; not in the legal-transition matrix that shipped. Tracked in [cli/idea/change-status#outstanding-questions](../features/cli/idea/change-status/README.md).
- **`--reason` / `--message` flag in MVP?** Deferred; verbs ship without it. Tracked in [cli/idea/change-status#outstanding-questions](../features/cli/idea/change-status/README.md) and [cli/feature/change-status#outstanding-questions](../features/cli/feature/change-status/README.md).
- **`feature deprecate` requires reason + successor in MVP?** Deferred; deprecation transition ships unconditionally. Tracked in [cli/feature/change-status#outstanding-questions](../features/cli/feature/change-status/README.md).

Still open (downstream / cross-repo, not gated by this Idea):

- None at this time.

---
*This document follows the https://specscore.md/idea-specification*
