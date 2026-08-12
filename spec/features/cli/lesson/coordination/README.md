---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Lesson Coordination

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/coordination?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/coordination?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/coordination?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/coordination?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Summary

Expose adapter-produced agent context without brokering live messages or
teaching Synchestra about SpecScore Lessons.

## Problem

Several Codex and Claude sessions can work on the same process gap. The durable record must identify their ownership and resume links, while live presence and messages change too quickly and belong to Synchestra. Copying live state into SpecScore would create a second, divergent broker.

## Behavior

```
specscore lesson agents <lesson> [--refresh] [--open <agent-id>] [--message <agent-id> --text <text>] [--resume <agent-id>] [--format text|yaml|json]
```

SpecScore resolves the selected Lesson before crossing the adapter boundary. The
SpecScore lesson-agents request schema version 2 sends the configured hook a
selected-project context and an `external_resource` object containing the
canonical project-relative artifact reference plus its revision digest. The
reference is stable within that project and opaque to Synchestra: it is not a
Synchestra Lesson identifier, and Synchestra never resolves its path or infers
a Doc-Kind from it.

SpecScore owns only the read-side contract for a generic, adapter-produced
durable projection. Normal reads render `agents.json` beside the canonical
Lesson and never contact a server; each projected record contains only the
external effort/session URL, agent ID, declared role, last observed state/time,
and source event ID. The Lesson association comes from SpecScore's local
resolution and the projection location, not from Lesson fields understood by
Synchestra. The core never creates or rewrites that operational projection, so
a repository chooses whether its adapter versions it or keeps it local.
`--refresh` explicitly invokes a configured neutral executable hook; a native
orchestration plugin owns the atomic projection refresh.

`--open`, `--message`, and `--resume` are explicit one-shot hook invocations. The core sends the generic external-resource request on stdin, runs the hook from the selected `--project` root, and streams the executable's result; it persists no message body, acknowledgement, retry queue, presence, or synthetic state. If the hook fails, the command fails and directs the caller to that native plugin. A Synchestra implementation binds a concrete **authoritative** public CLI/server interface/outbox (including its real receipt/retry path), never another unspecified executable: SpecScore itself never reads/writes a mirror/backup, Git state, SQLite, DALgo, inGitDB, or backend-specific API. Synchestra associates efforts, runs, and sessions with generic external-resource references, stores/compares/returns each reference unchanged, and owns topology, authorization, stable idempotent message IDs, delivery receipts/retry, resume audit, replay, and visible mirror lag. The adapter maps project context plus the opaque reference into that generic contract and atomically renders `agents.json`.

Neither the adapter request nor the required Synchestra public surface carries
structured Lesson slugs, statuses, occurrences, or relations. A native adapter
MUST NOT depend on a Lesson-specific endpoint such as
`GET /v1/lesson-agents`; such an endpoint would put a SpecScore concept in the
wrong product.

## Acceptance Criteria

### AC: durable-context-is-readable-offline

**Given** a Lesson projection with a Codex implementation session and Claude reviewer session
**When** `lesson agents <slug> --format json` runs offline
**Then** durable URLs, roles, observed states, and observation times return without contacting Synchestra or reading a message.

### AC: message-is-delegated-not-stored

**Given** a configured Synchestra adapter
**When** a user runs `lesson agents <slug> --message codex-1 --text "please review occurrence 01"`
**Then** the adapter receives one generic external-resource request, SpecScore writes no message/outbox/history, and delivery/retry/audit remain Synchestra's responsibility.

### AC: synchestra-remains-resource-generic

**Given** a canonical Lesson with lifecycle, occurrence, and relation data
**When** SpecScore invokes the configured native adapter
**Then** the request contains project context, one canonical external-resource reference, and its revision, but no structured Lesson slug, status, occurrence, or relation fields; the adapter uses only Synchestra's generic effort/run/session association and no Lesson-specific endpoint.

## Open Questions

- Which configured adapter transport maps the selected local project context to
  Synchestra project identity and authentication remains to be specified with
  the native integration. That choice cannot change the fixed product boundary:
  the external-resource reference stays generic and opaque to Synchestra.

---
*This document follows the https://specscore.md/feature-specification*
