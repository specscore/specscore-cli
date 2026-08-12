---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Lesson Events

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/events?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/events?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/events?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/events?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Summary

Emit reliable lifecycle and occurrence events for process improvement.

## Problem

The existing event dispatcher is best-effort fan-out: a failure may be ignored when another subscriber succeeds. That is acceptable for telemetry, but not for durable process-improvement facts or coordination projections; a lost lifecycle/occurrence event makes subscribers disagree about active work.

## Behavior

### Automatic Lesson events

Mutating commands prepare the complete event and recipient set before artifact publication, then commit it only after artifact and bounded index validation: `lesson.created`, `lesson.lifecycle-changed`, `lesson.occurrence-recorded`, `lesson.relation-recorded`, `lesson.legacy-import-applied`, and `lesson.flat-migrated`. Payloads contain Lesson identity/path, event UUID/time/actor, and minimal mutation facts; occurrence context remains in the validated child rather than being copied into the envelope. A failed mutation aborts its prepared record only when it is proven pre-publication or its exact rollback plus durability fence completed; any publication, removal, or fsync uncertainty retains the prepared record with its UUID for explicit reconciliation. Callers never need a separate `event emit`.

### Durable per-subscriber outbox and replay

For each named durable subscriber, the event layer publishes an immutable prepared ledger record containing the complete canonical subscriber set. Committing makes every missing acknowledgement reconstructibly pending before delivery. An acknowledgement advances only its own state. Failure leaves the item replayable and never suppresses another subscriber; `(subscriber, event UUID)` is the idempotency key. An explicitly empty configured subscriber list opts out without creating an invalid recipientless ledger.

```
specscore event replay --subscriber <name> [--from <event-id>] [--limit N]
```

Replay is deterministically ordered by immutable event timestamp then UUID and safe to repeat; it records durable acknowledgements. `--from` is an inclusive committed-event cursor, remains valid after that event is acknowledged, and fails closed for an unknown, prepared, aborted, or subscriber-unaddressed event. The sender never promises exactly-once remote delivery; consumers deduplicate UUIDs. The outbox is for SpecScore event subscribers only, never a live-agent message queue. Any Synchestra integration sends only to its configured authoritative public CLI/server endpoint/outbox, never a replica/mirror or Git/SQLite/DALgo/inGitDB internal; Synchestra owns backend selection and asynchronously mirrored Git lag.

## Acceptance Criteria

### AC: lifecycle-event-is-committed-or-absent

**Given** a Lesson transition whose post-mutation lint or durability fence fails after publication
**When** `lesson change-status` returns recovery-required
**Then** no lifecycle event is committed or deliverable, and its prepared ledger record remains inspectable for explicit reconciliation.

### AC: failed-subscriber-replays-independently

**Given** two durable subscribers and an occurrence event where A fails and B acknowledges
**When** `event replay --subscriber A` later succeeds
**Then** B is never redelivered, A receives original UUID/context on each retry, and both cursors acknowledge the same ledger event.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
