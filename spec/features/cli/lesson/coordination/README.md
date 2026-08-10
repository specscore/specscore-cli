---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Lesson Coordination

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/coordination?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/coordination?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/coordination?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/coordination?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Summary

Expose durable Synchestra agent context without brokering live messages.

## Problem

Several Codex and Claude sessions can work on the same process gap. The durable record must identify their ownership and resume links, while live presence and messages change too quickly and belong to Synchestra. Copying live state into SpecScore would create a second, divergent broker.

## Behavior

```
specscore lesson agents <lesson> [--refresh] [--open <agent-id>] [--message <agent-id> --text <text>] [--resume <agent-id>] [--format text|yaml|json]
```

SpecScore owns only a generic, adapter-produced durable projection: Lesson slug/path/revision, external task/session URL, agent ID, declared role, last observed state/time, and source event ID. Normal reads render `agents.json` beside the canonical Lesson and never contact a server. The core never creates or rewrites that operational projection, so a repository chooses whether its adapter versions it or keeps it local. `--refresh` explicitly invokes a configured neutral executable hook; a native orchestration plugin owns the projection update.

`--open`, `--message`, and `--resume` are explicit one-shot hook invocations. The core sends a versioned neutral JSON request on stdin, runs the hook from the selected `--project` root, and streams the executable's result; it persists no message body, acknowledgement, retry queue, presence, or synthetic state. If the hook fails, the command fails and directs the caller to that native plugin. A Synchestra implementation binds a concrete **authoritative** public CLI/server endpoint/outbox (including its real receipt/retry path), never another unspecified executable: SpecScore itself never reads/writes a mirror/backup, Git state, SQLite, DALgo, inGitDB, or backend-specific API. The native plugin owns topology, authorization, delivery, replay, audit, and visible mirror lag.

## Acceptance Criteria

### AC: durable-context-is-readable-offline

**Given** a Lesson projection with a Codex implementation session and Claude reviewer session
**When** `lesson agents <slug> --format json` runs offline
**Then** durable URLs, roles, observed states, and observation times return without contacting Synchestra or reading a message.

### AC: message-is-delegated-not-stored

**Given** a configured Synchestra adapter
**When** a user runs `lesson agents <slug> --message codex-1 --text "please review occurrence 01"`
**Then** the adapter receives one request, SpecScore writes no message/outbox/history, and delivery/retry/audit remain Synchestra's responsibility.

## Open Questions

- The adapter config and authentication hand-off are jointly owned with Synchestra; this Feature fixes the ownership boundary, not its transport protocol.

---
*This document follows the https://specscore.md/feature-specification*
