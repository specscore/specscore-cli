# Events

SpecScore emits structured events when artifacts change state, when skills
complete, or when a verb explicitly asks the CLI to publish one. This document
describes how events flow and how to configure consumers for your project.

## Overview

Skills and verbs produce events; the `specscore` CLI dispatches each one to a
list of registered subscribers configured in `specscore.yaml`. Before delivery,
the CLI publishes one immutable outbox record containing the complete canonical
subscriber set. Built-in
subscriber types cover the common cases (append-to-file, drop-on-the-floor,
and pipe-to-a-child-process), and the dispatcher delivers each envelope to
every configured subscriber in order. Subscriber failures do not stop other
subscribers from running and remain independently pending; a success at one
sink never acknowledges another sink. The user-facing entrypoint for
publishing an event is the `specscore event emit` verb (see
[`https://specscore.md/event-emit`](https://specscore.md/event-emit)); the
source-of-truth Feature is
[`spec/features/cli/event/README.md`](../spec/features/cli/event/README.md).

Delivery is at least once. Subscribers must deduplicate using the envelope
`uuid` if an interruption can occur after their side effect but before the CLI
records the acknowledgement. Inspect and resume pending work with
`specscore event replay`; inspect an interrupted prepared record and resolve it
deliberately with `specscore event reconcile`. Malformed, trailing, invalid, or
noncanonical ledger records are rejected before a subscriber is invoked.
Cryptographic detection of an otherwise schema-valid local ledger rewrite is a
Planned capability, not a current guarantee.

## The events: config block

Subscribers live under a top-level `events:` key in `specscore.yaml`:

```yaml
events:
  subscribers:
    - type: jsonl
      path: .specscore/events.jsonl
    - type: noop
    - type: exec
      name: webhook
      command: ["./bin/forward-to-webhook.sh"]
      timeout_ms: 5000
      env:
        WEBHOOK_URL: https://example.com/hooks/specscore
```

Each entry under `subscribers:` is a mapping. The required `type:` field
selects which built-in subscriber to instantiate; the remaining fields are
type-specific.

| Field         | Applies to       | Description                                                                                                |
| ------------- | ---------------- | ---------------------------------------------------------------------------------------------------------- |
| `type`        | all              | One of `jsonl`, `noop`, `exec`. Unknown values are rejected at load time.                                  |
| `path`        | `jsonl`          | File path for appended JSONL output. Relative paths resolve from the project root. Required.               |
| `command`     | `exec`           | Argv list (sequence of strings) for the child process. Must be non-empty.                                  |
| `name`        | `exec`           | Optional human-readable identifier used in stderr failure logs. Defaults to the first argv element.        |
| `timeout_ms`  | `exec`           | Per-event wall-clock budget for the child process, in milliseconds. Defaults to `2000`. Range: `[100, 30000]`. |
| `env`         | `exec`           | Optional mapping of environment variables (string to string) to pass to the child process.                 |

## Built-in subscribers

### jsonl

Appends one event per line to a file in JSONL format. The `path:` field is
resolved relative to the project root, so `path: .specscore/events.jsonl`
writes to `<project>/.specscore/events.jsonl`. Parent directories are created
on first write. This is the default sink synthesized when no `events:` block
is configured.

### noop

Silently discards every event delivered to it. Useful for explicitly turning
off event delivery at the project level without removing the `events:` block.

### exec

Pipes the JSON-encoded envelope to a child process via stdin and waits for it
to exit. The child is considered successful if it exits with status `0`. A
non-zero exit, a crash, or exhausting `timeout_ms` is reported on stderr. The
budget starts before process-tree setup, so an expiry during setup or process
start is reported as a timeout too. Timeout cleanup includes descendants
started by the command. On Unix the
dispatcher sends `SIGTERM` to the command's process group, then `SIGKILL` after
100 ms if the group is still running. On Windows it assigns the command to an
owned Job Object and terminates that job because Windows has no `SIGTERM`
equivalent. If Job Object termination is unavailable, it immediately kills the
retained direct child and bounds cleanup waiting rather than hanging.
Successful commands release this timeout ownership without terminating any
background descendants they deliberately started. The default timeout is
2000 ms.

## Writing an Exec subscriber

An `exec` subscriber can be any executable on disk. The contract is small:

1. The process receives one line of JSON on stdin: the event envelope.
2. Exit `0` on successful handling; any non-zero exit is treated as failure.
3. Finish within `timeout_ms` (default 2000 ms).

A tiny shell-based webhook forwarder:

```bash
#!/usr/bin/env bash
# bin/forward-to-webhook.sh
set -euo pipefail
payload="$(cat)"
curl -fsS -X POST \
  -H 'content-type: application/json' \
  --data "$payload" \
  "$WEBHOOK_URL" >/dev/null
```

Wire it in:

```yaml
events:
  subscribers:
    - type: exec
      name: webhook
      command: ["./bin/forward-to-webhook.sh"]
      timeout_ms: 5000
      env:
        WEBHOOK_URL: https://example.com/hooks/specscore
```

The `specscore event emit` verb exits with code `0` on success, `2` on
envelope validation failure, `3` on configuration error, and `10` when one or
more subscribers fail to deliver.

## Envelope shape

Every event delivered to a subscriber is a JSON object with the following
fields. All fields are required.

| Field               | Description                                                                                                          |
| ------------------- | -------------------------------------------------------------------------------------------------------------------- |
| `name`              | Dotted event name, e.g. `feature.approved`. Must match `^[a-z][a-z0-9-]*\.[a-z][a-z0-9-]*$`.                         |
| `version`           | Positive integer schema version of the event (`>= 1`).                                                               |
| `uuid`              | Lowercase UUID v4 uniquely identifying this emission.                                                                |
| `timestamp`         | RFC 3339 timestamp in UTC (must end with `Z` or `+00:00`).                                                           |
| `actor.kind`        | One of `skill`, `user`, `external`.                                                                                  |
| `actor.id`          | Non-empty string identifying the actor (e.g. skill name, user handle, integration slug).                             |
| `artifact.type`     | One of `idea`, `feature`, `plan`, `task`, `idea-seed`, `consilium-review`.                                           |
| `artifact.id`       | Non-empty string identifying the artifact (typically its slug or ID).                                                |
| `artifact.path`     | Non-empty repo-relative path to the artifact file.                                                                   |
| `artifact.revision` | Non-empty string; the git revision the artifact was read at. The literal value `uncommitted` is permitted.           |
| `payload`           | Free-form JSON object carrying event-specific data. Must parse as a JSON object; the dispatcher does not inspect it. |

Example envelope:

```json
{
  "name": "feature.approved",
  "version": 1,
  "uuid": "7c2d3f5a-9b4e-4a1f-8c2d-1e3a4b5c6d7e",
  "timestamp": "2026-05-22T18:42:11Z",
  "actor": {
    "kind": "user",
    "id": "alexander"
  },
  "artifact": {
    "type": "feature",
    "id": "cli/event",
    "path": "spec/features/cli/event/README.md",
    "revision": "uncommitted"
  },
  "payload": {
    "previous_status": "Under Review",
    "new_status": "Approved"
  }
}
```

## Default behavior

If `specscore.yaml` does not contain an `events:` block (or the file does not
exist), the CLI synthesizes a single `jsonl` subscriber writing to
`.specscore/events.jsonl` relative to the project root. By convention this
file is git-ignored — it is a local audit log, not a checked-in artifact.

## Disabling events

To turn off event delivery entirely, declare an explicit empty subscriber
list:

```yaml
events:
  subscribers: []
```

This differs from omitting the block: an empty list means "the operator opted
in and chose zero subscribers," so the CLI creates no recipientless outbox
record; an absent block falls back to the default JSONL sink. A single `noop`
entry keeps a durable delivery/acknowledgement record while producing no
external effect:

```yaml
events:
  subscribers:
    - type: noop
```
