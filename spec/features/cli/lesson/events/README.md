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

Successful mutations automatically create versioned events after artifact mutation and lint/index synchronization commit: `lesson.lifecycle-changed`, `lesson.occurrence-recorded`, `lesson.relation-recorded`, and `lesson.imported`. Payloads contain canonical Lesson identity/path/revision, event ID/time/actor, and minimal mutation facts; occurrence context remains opaque JSON. Failed/rolled-back mutations emit nothing. Callers never need a separate `event emit`.

### Durable per-subscriber outbox and replay

For each named durable subscriber, the event layer appends an accepted envelope to an immutable project ledger and atomically records a pending delivery in that subscriber's independent outbox before attempting delivery. An acknowledgement advances only its own cursor. Failure leaves the item replayable and never suppresses another subscriber; `(subscriber, event UUID)` is the idempotency key.

```
specscore event replay --subscriber <name> [--from <event-id>] [--limit N]
```

Replay is ledger ordered and safe to repeat; it records durable acknowledgements. The sender never promises exactly-once remote delivery; consumers deduplicate UUIDs. The outbox is for SpecScore event subscribers only, never a live-agent message queue. Any Synchestra integration sends only to its configured authoritative public CLI/server endpoint/outbox, never a replica/mirror or Git/SQLite/DALgo/inGitDB internal; Synchestra owns backend selection and asynchronously mirrored Git lag.

## Acceptance Criteria

### AC: lifecycle-event-is-committed-or-absent

**Given** a Lesson transition whose post-mutation lint fails
**When** `lesson change-status` rolls back
**Then** no lifecycle event exists in the ledger or any outbox.

### AC: failed-subscriber-replays-independently

**Given** two durable subscribers and an occurrence event where A fails and B acknowledges
**When** `event replay --subscriber A` later succeeds
**Then** B is never redelivered, A receives original UUID/context on each retry, and both cursors acknowledge the same ledger event.

## Open Questions

- Ledger/outbox storage and locking are owned with the event subsystem owner; the above durability and Synchestra-boundary requirements are fixed.

---
*This document follows the https://specscore.md/feature-specification*
