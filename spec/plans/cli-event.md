---
format: https://specscore.md/plan-specification
status: Implemented
---

# Plan: Events (CLI) — pkg/event Library + Dispatcher

**Status:** Implemented
**Mode:** full
**Source Feature:** cli/event
**Date:** 2026-05-22
**Owner:** alexandertrakhimenok
**Supersedes:** —

## Summary

Implementation plan for the parent `cli/event` Feature — the `pkg/event` Go package, three Subscriber implementations (`JsonlWriter`, `NoOp`, `Exec`), the envelope validator, the `events:` config-block loader, the fan-out dispatcher, durable per-subscriber outbox/reconciliation, and the `docs/events.md` surface. The verb that consumes this library (`specscore event emit`) is planned separately in `spec/plans/cli-event-emit.md`.

## Approach

Layer-first decomposition: package skeleton → subscribers → validator → config → dispatcher → docs. The package skeleton (task 1) is a prerequisite for everything else; the three subscribers (tasks 2–4) are independent within their layer; the envelope validator (task 5) is a pure function with no other dependencies; the config loader (task 6) needs the subscriber constructors; the dispatcher (task 7) composes the validator, the config loader, and the subscriber list. Docs (task 8) land last so any details that shifted during implementation are captured rather than drift from the user-facing prose.

Within the subscribers layer the order is `JsonlWriter` → `NoOp` → `Exec`. `JsonlWriter` first because it is the default-config sink (REQ:default-and-empty-config depends on it existing for the synthesis path to compile end-to-end); `NoOp` next as a trivial check that the interface implementation pattern is correct before the larger `Exec` work; `Exec` last because it carries the most implementation cost (per-call timeout, stdin pipe, SIGTERM/SIGKILL signal handling).

No ACs are deferred. Every AC in `cli/event` is covered by a task.

## Tasks

### Task 1: Bootstrap `pkg/event` package with Subscriber interface and Event struct

**Status:** complete
**Depends-On:** —
**Verifies:** cli/event#ac:subscriber-interface-shape

Create `pkg/event/` with `subscriber.go` defining the `Subscriber` interface (`Deliver(ctx context.Context, e Event) error`, `Name() string`) and the `Event` struct (`Name`, `Version`, `UUID`, `Timestamp`, `Actor`, `Artifact`, `Payload` where `Payload` is `json.RawMessage`). Package name is the singular `event` matching the `pkg/` house style (`pkg/feature`, `pkg/idea`, `pkg/lifecycle`, `pkg/plan`, `pkg/lint`, `pkg/task`). Compiles cleanly under `go vet ./pkg/event/...` with no other types yet — subscriber implementations and the dispatcher come in later tasks.

### Task 2: Implement `JsonlWriter` subscriber with project-root path resolution

**Status:** complete
**Depends-On:** 1
**Verifies:** cli/event#ac:jsonl-writer-appends-line, cli/event#ac:jsonl-writer-resolves-against-project-root

Add `pkg/event/jsonl.go` implementing `JsonlWriter`. `Deliver` serializes the `Event` to single-line JSON, appends with `O_APPEND|O_CREATE|O_WRONLY` mode `0644`, creates parent dirs at mode `0755` if absent. Relative paths resolve against the project root (use `pkg/projectdef` for discovery), never against cwd. `Name()` returns `jsonl:<path>`. Land first among subscribers because the default-config path (task 6) synthesizes a `JsonlWriter` and needs the constructor to exist.

### Task 3: Implement `NoOp` subscriber

**Status:** complete
**Depends-On:** 1
**Verifies:** cli/event#ac:noop-discards

Add `pkg/event/noop.go` implementing `NoOp`. `Deliver` returns nil with no side effects (no filesystem, no network, no stdout, no stderr). `Name()` returns the literal `noop`. Trivial implementation, but a discrete task so the explicit-opt-out path has its own dedicated commit and test coverage.

### Task 4: Implement `Exec` subscriber with timeout + SIGTERM/SIGKILL

**Status:** complete
**Depends-On:** 1
**Verifies:** cli/event#ac:exec-pipes-event-to-stdin, cli/event#ac:exec-timeout-kills-hung-process

Add `pkg/event/exec.go` implementing `Exec`. Build `os/exec.Cmd` from the configured argv (first element executable, rest positional args); append the configured `env` mapping to the child's environment (additive, not replacement); pipe serialized envelope JSON to stdin then close stdin (signalling EOF); inherit stderr, discard stdout. Enforce wall-clock timeout (default 2000 ms; range `[100, 30000]` ms — the bounds check itself lives in task 6's config loader, but the Exec subscriber respects the configured value). On timeout send SIGTERM, wait 100 ms grace, then SIGKILL. Distinguish timeout errors from exit-code errors in the returned error type so task 7's dispatcher stderr log can name the failure mode.

### Task 5: Implement envelope validator

**Status:** complete
**Depends-On:** 1
**Verifies:** cli/event#ac:envelope-validation-rejects-bad-name, cli/event#ac:envelope-validation-rejects-bad-actor-kind, cli/event#ac:payload-is-opaque-passthrough

Add `pkg/event/envelope.go` with a pure `Validate(Event) error` function covering all rules from REQ:envelope-validation: event-name regex `^[a-z][a-z0-9-]*\.[a-z][a-z0-9-]*$`; positive `Version`; lowercase-hyphenated UUID v4 (full regex check, not just length); RFC 3339 UTC timestamp with `Z` or explicit `+00:00`; closed enums for `actor.kind` (`skill|user|external`) and `artifact.type` (`idea|feature|plan|task|lesson|idea-seed|consilium-review`); non-empty string fields for `actor.id`, `artifact.id`, `artifact.path`, `artifact.revision` (literal `uncommitted` is permitted); `payload` parses as a JSON object. Payload field-level inspection is explicitly out — the function MUST NOT touch payload fields beyond confirming the bytes parse as `{...}`. Validation errors carry the offending field name and the rule violated.

### Task 6: Implement `events:` config schema loader

**Status:** complete
**Depends-On:** 2, 3, 4
**Verifies:** cli/event#ac:events-config-default-when-absent, cli/event#ac:events-config-explicit-empty-is-noop, cli/event#ac:events-config-unknown-type-rejected

Add `pkg/event/config.go` parsing the `events:` block from `specscore.yaml`. Extend the existing project-config loader (or wire a sibling decoder using `gopkg.in/yaml.v3`) to produce a typed subscriber list of `Subscriber` instances. Enforce REQ:events-config-schema: unknown `type` value yields a load error citing the file (`specscore.yaml`), the key path (`events.subscribers[N].type`), the offending value, and the accepted enum (`jsonl`, `noop`, `exec`); missing required fields per type (`path` for jsonl, `command` for exec — non-empty argv list) yield the same form; `timeout_ms`, when present, must be an integer in `[100, 30000]`. Apply REQ:default-and-empty-config: an absent `events:` block synthesizes one `JsonlWriter` at `.specscore/events.jsonl`; an explicit `subscribers: []` is honored as a zero-subscriber list (no synthesis) and is the project-level explicit-disable path.

### Task 7: Implement fan-out dispatcher with stderr failure log + exit-code mapping

**Status:** complete
**Depends-On:** 5, 6
**Verifies:** cli/event#ac:fan-out-continues-after-failure, cli/event#ac:per-subscriber-failure-stderr-format, cli/event#ac:dispatch-exit-code-when-all-fail

Add `pkg/event/dispatcher.go` exposing `Dispatch(ctx, Event, []Subscriber) DispatchResult`. Validate envelope first via task 5's validator; iterate subscribers sequentially in declared order; on per-subscriber non-nil error, write exactly one stderr line in the contracted key=value form `event-dispatch failure: subscriber=<Name> event=<n> error="<err>"` (double-quote-escape embedded double quotes in the error message) and continue to the next subscriber. Map dispatch outcomes to the standard exit-code contract (REQ:dispatch-exit-codes — 0 / 2 / 3 / 10): exit 0 when at least one subscriber returned nil OR the configured list was empty; exit 2 on envelope-validation failure (delegated to task 5) or config-validation failure (delegated to task 6); exit 3 on missing project root (delegated to the existing `pkg/projectdef` autodetect); exit 10 only when every subscriber in a non-empty list returned a non-nil error or timed out. Successful deliveries MUST NOT produce any stderr output.

### Task 8: Author `docs/events.md` skeleton

**Status:** complete
**Depends-On:** 7
**Verifies:** cli/event#ac:docs-events-md-skeleton-present

Add `docs/events.md` with the seven required second-level sections in order: `## Overview`, `## The events: config block`, `## Built-in subscribers`, `## Writing an Exec subscriber`, `## Envelope shape`, `## Default behavior`, `## Disabling events`. Under `## Built-in subscribers`, third-level subsections `### jsonl`, `### noop`, `### exec`. Under `## Envelope shape`, mirror REQ:envelope-validation's rule set as user-facing prose. Land last so any flag-name or schema details that shifted during tasks 1–7 are captured in the user-facing doc rather than drift from it. The user-facing tone differs from the spec's MUST/MUST-NOT phrasing — write the doc in declarative prose.

### Task 9: Add durable per-subscriber outbox and explicit recovery

**Status:** complete
**Depends-On:** 5, 6, 7
**Verifies:** cli/event#ac:subscriber-success-never-masks-another-pending-sink, cli/event#ac:interrupted-prepared-event-is-explicit, cli/event#ac:corrupt-ledger-never-reaches-a-subscriber

Publish one immutable prepared ledger record with the complete sorted subscriber set before an artifact mutation; commit or abort it with per-event synchronization; reconstruct every pending marker from the committed ledger; and acknowledge each subscriber independently. Route `event emit` through the same generic outbox. Add explicit replay and prepared-record reconciliation, strict envelope/record validation before delivery, no-clobber publication with directory durability, crash-boundary tests, and an at-least-once UUID-idempotency contract. Keep cryptographic detection of a schema-valid local ledger rewrite honestly Planned.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
