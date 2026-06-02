# Idea: Event Emit Dispatcher

**Status:** Implemented
**Date:** 2026-05-22
**Owner:** alexandertrakhimenok
**Promotes To:** cli/event, cli/event/emit
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we let SpecScore skills emit events through a single `specscore event emit` command that fans out to 0, 1, or many subscribers — so skills stay transport-agnostic and operators can swap in arbitrary consumers (file sink, workflow orchestrator, NoOp, anything else) without touching skill code?

## Context

The skill-side event contract is already documented in the SDD skills repo's cross-repo event contract document at `skills/shared/` (filename owned by that repo): *"Skills invoke `specscore event emit <event.yaml>` (CLI) when available; fall back to direct file append otherwise."* The CLI hook does not exist yet. Six skills (`ideate`, `specify`, `sidekick`, `consilium`, `verify`, `recap`) each duplicate the JSONL-append fallback in their own bash blocks, hard-coding `.specscore/events.jsonl` and the JSON envelope shape. Every new event-aware skill pays this duplication tax, and no consumer other than the file exists. A downstream workflow orchestrator that wants to consume these events to wire up post-approval automation (e.g., react to `idea.approved` by scheduling `specstudio:specify`) has no extension point today — it would have to tail the JSONL file out-of-band. This Idea is orthogonal to the existing `cli-telemetry` Idea: that one emits anonymous product-analytics to PostHog from `PersistentPreRun`/`PostRun`; this one is a structured-event verb invoked explicitly by skills with a payload they construct.

## Recommended Direction

Add a `specscore event emit` subcommand backed by a new `pkg/event` package. The package defines a `Subscriber` interface and ships two compiled-in implementations — `JsonlWriter` (appends an envelope+payload JSON line to a configurable file path, default `.specscore/events.jsonl`) and `NoOp` (discards) — plus a generic `Exec` subscriber that runs a configured command with the event JSON piped to its stdin. Subscribers are configured under a new top-level `events:` block in `specscore.yaml`. When no `events:` block exists, the dispatcher auto-attaches a single `JsonlWriter` pointed at `.specscore/events.jsonl` resolved relative to the project root — preserving today's zero-config behavior. `NoOp` is the explicit opt-out: set `events: { subscribers: [{ type: noop }] }` to disable. Multiple subscribers are dispatched in declared order with fan-out, log-and-continue semantics: every subscriber is invoked; per-subscriber failures are logged to stderr; the command exits 0 if at least one subscriber succeeded and non-zero only if all failed. Validation in v1 is envelope-only — the dispatcher checks the common envelope (event name, version, timestamp, actor, artifact, uuid) and treats the payload as opaque, so new event names can ship in skills without coupling CLI releases to skill releases. The verb accepts the event payload as a YAML/JSON file via `--file <path>` or on stdin; this matches the two forms that appear in existing skill bash blocks. Skills lose their inline fallback-append branch — `specscore event emit` becomes the only emission path documented in the SDD skills repo's event contract document. This decouples skill cadence from transport cadence and gives any downstream workflow orchestrator a clean integration point: register an Exec subscriber pointing at the orchestrator's ingest command (or equivalent) in `specscore.yaml` and that consumer receives every event without specscore-cli ever knowing it exists.

## Alternatives Considered

- **Status quo: every skill appends to `.specscore/events.jsonl` directly.** Loses: six skills duplicate the same JSONL-shaped bash block, every new event-aware skill pays the same tax, there's no extension point for any downstream consumer, and disabling events requires per-skill changes. The "fall back to direct file append" branch already documented in the SDD skills repo's event contract was meant as a transitional bridge — keeping it as the only path freezes the worst case as the default.
- **Compiled-in subscribers only (no Exec).** Loses: every new consumer (workflow orchestrators, Hub, future tools) requires a `specscore-cli` release that knows about it. Couples skill-side iteration cadence to CLI release cadence and forces the CLI to grow knowledge of every downstream system. Exec subscribers solve this once with a contract narrow enough to be stable (pipe JSON to stdin, exit code is the verdict).
- **Pure pub/sub broker (NATS, daemon, in-process channels with a long-running process).** Loses: massive operational weight for a CLI tool that runs synchronously and exits. Wrong shape — there's no long-lived process to host a broker, and skills are short bash blocks that wouldn't benefit from async fan-out.
- **HTTP webhooks as the v1 extension mechanism instead of Exec.** Loses: every consumer needs a running HTTP server, every dev needs one on localhost for local-only workflows, and we'd be designing retry/timeout/auth before we have a single concrete consumer. Exec subscribers fronting `curl` cover the webhook case at one-tenth the v1 design cost, and an HTTP subscriber can land additively later when a remote consumer exists.

## MVP Scope

Ship in the next `specscore` minor release: `specscore event emit` verb, `pkg/event` with `JsonlWriter`, `NoOp`, and `Exec` subscribers, `events:` block in `specscore.yaml` loader, default-when-absent behavior matching today's JSONL append, fan-out + log-and-continue dispatch, envelope-only validation. One skill (`verify` — smallest, single-event payload) is updated end-to-end as the proof point and lands in the same release window: its bash block drops the conditional and the fallback branch in favor of an unconditional `specscore event emit --file <event.yaml>` call. `docs/event-emit.md` documents the `events:` schema, the three subscriber types, and the fan-out failure semantics. Migration of the other five skills (`ideate`, `specify`, `sidekick`, `consilium`, `recap`) is tracked in a follow-up Feature in `specstudio-skills` — not gated on this MVP.

## Not Doing (and Why)

- HTTP webhook subscribers — defer until a remote consumer (HTTP-receiving orchestrator, Hub) exists; Exec subscribers fronting curl cover the same need at v1 cost
- Go plugin / wasm subscribers — operational weight outweighs flexibility; Exec subscribers handle the extensibility case adequately
- Async / queued dispatch — skills already block on emission today; preserving synchronous semantics keeps the failure surface tiny
- Per-subscriber event filtering / topic routing — every subscriber receives every event in v1; subscribers filter in-process if they care
- Retry / backoff on subscriber failure — log-and-continue is the contract; durable delivery is the consumer's job (or a future Idea)
- Per-event payload schema validation — envelope-only validation in v1; per-event schemas couple CLI releases to skill releases and can land later as additive
- Replay / seek API over `.specscore/events.jsonl` — the file is append-only by convention; consumers tail it themselves

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | The six event-emitting skills (`ideate`, `specify`, `sidekick`, `consilium`, `verify`, `recap`) can be ported to call `specscore event emit` with no behavioral regression: same event written to `.specscore/events.jsonl` in the default case, same envelope, same payload. | Port `verify` end-to-end in this MVP, run its existing test suite, diff a recorded `events.jsonl` line before/after the change — bytes must match modulo timestamp/uuid. |
| Must-be-true | A single `events:` block in `specscore.yaml` can express both compiled-in subscribers (by `type` enum) and Exec subscribers (by `command`) in a schema small enough to fit in `docs/event-emit.md` on one page. | Write the schema at `/plan` time, walk through three example configs (default, NoOp opt-out, JSONL + Exec→external consumer), confirm each fits the schema without escape hatches. |
| Must-be-true | Synchronous, fan-out, log-and-continue dispatch keeps p99 emission overhead small enough to be invisible to interactive skill use (target: under 50 ms for the JSONL-only default; under 200 ms with one Exec subscriber). | Benchmark in CI: emit 100 events back-to-back with each config, record p50/p99 wall-clock per invocation. |
| Should-be-true | The Exec-subscriber contract (pipe event JSON to stdin, non-zero exit = failure) is sufficient for an external workflow orchestrator to consume events without specscore-cli needing to know what the consumer is. | Sketch a sample ingest shim at `/plan` time; confirm with the consuming-system owners that no specscore-cli changes are needed. |
| Should-be-true | Resolving `.specscore/events.jsonl` relative to the project root (`projectdef.Root()`) rather than cwd is the right default for monorepos and nested-repo layouts. | Audit the six skill bash blocks to confirm they assume "repo root" semantics today (`git rev-parse --show-toplevel`), not cwd. |
| Should-be-true | The follow-up migration of the other five skills can happen incrementally over a release or two without coordinating a flag-day in `specstudio-skills`. | Skills retain their current bash blocks until each is ported; the CLI's default behavior is a strict superset of the JSONL append they do today, so an un-migrated skill continues to work. |
| Might-be-true | One Exec subscriber per consumer is the right granularity. (Could imagine a single multiplexing Exec process that fans out internally.) | Defer until a second Exec consumer appears. |
| Might-be-true | The `events:` block belongs in `specscore.yaml` and not in a separate `.specscore/events.yaml`. | Defer until the `events:` schema grows past ~20 lines; if it does, revisit. |


## SpecScore Integration

- **New Features this would create:** A parent `cli/event` Feature for the verb surface and shared envelope-validation logic, with one child Feature per first-class subscriber the MVP ships (`cli/event/jsonl-writer`, `cli/event/noop`, `cli/event/exec`). A separate `cli/event/dispatcher` Feature covers the registry, fan-out semantics, and the `specscore.yaml` `events:` block loader. Final hierarchy to be confirmed at `/specstudio:specify` time.
- **Existing Features affected:** None in `specscore-cli` itself — this is additive. In `specstudio-skills`, the shared cross-repo event contract document and the bash blocks of all six event-emitting skills (`ideate`, `specify`, `sidekick`, `consilium`, `verify`, `recap`) are downstream consumers; only the `verify` skill is updated as part of this MVP, the rest are tracked as follow-up work in `specstudio-skills`.
- **Dependencies:** Existing `projectdef` package (for project-root resolution) and the existing `specscore.yaml` loader. No new external Go modules — `gopkg.in/yaml.v3` is already in use, `os/exec` is stdlib.

## Open Questions

- **Exact `specscore.yaml` `events:` schema.** Likely shape: `events: { subscribers: [{ type: jsonl, path: ... } | { type: noop } | { type: exec, command: [...], env: {...} }] }`. Final shape (named map vs. list, error semantics for unknown `type`) settles at `/specstudio:specify` time.
- **Input mode: `--file <path>` vs. stdin vs. both.** Existing skill bash blocks show both flavors (sidekick assembles a JSONL line directly; consilium does the same; verify references `<event.yaml>`). Pick one canonical and one accepted-but-not-promoted at specify time.
- **Deprecation path for the fallback-append branch in the SDD skills repo's event contract.** Either (a) remove it from the contract immediately and require `specscore event emit` (cleanest, but pins every event-emitting skill to a specscore-cli minimum version), or (b) keep it as a documented v0.x compat path until all six skills are migrated. Choose at specify time.
- **Per-subscriber-failure stderr format.** Human-readable line vs. structured JSON. Likely structured JSON when stdout is non-TTY, human when TTY — but defer the decision until the first interactive use surfaces a real preference.
- **Exec subscriber timeout.** A hung subscriber would block the calling skill. Reasonable default candidates: 2 s for envelope-only events, 5 s for events with large payloads. Settle at specify time; document as configurable per-subscriber later if needed.

---
*This document follows the https://specscore.md/idea-specification*
