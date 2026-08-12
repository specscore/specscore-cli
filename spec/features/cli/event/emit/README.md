---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: Event Emit Verb

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/event/emit?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/event/emit?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/event/emit?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/event/emit?op=request-change) |
**Status:** Stable
**Date:** 2026-05-22
**Owner:** alexandertrakhimenok
**Source Ideas:** event-emit-dispatcher
**Supersedes:** —

## Summary

The user-facing emission verb: `specscore event emit`. It constructs a validated envelope from flags and one payload input mode, durably enqueues the envelope for the complete subscriber set, and attempts every pending delivery. A successful subscriber never masks a failed subscriber.

## Problem

The parent Feature ([`cli/event`](../README.md)) owns the subscriber registry, envelope validator, dispatcher primitive, and durable outbox — none of which is reachable from a shell. Skills need a CLI entry point that assembles the envelope, chooses one payload source, supplies bookkeeping fields, and does not lose one sink's work when another sink succeeds.

This Feature is that entry point. It translates command-line input into a fully populated `event.Event`, then uses the parent's durable outbox so a stopped process or one failed sink does not lose delivery work.

## Behavior

### Verb registration

The verb is the only command this Feature owns.

#### REQ: verb-registration

The CLI MUST register `specscore event emit` as a cobra subcommand under the `event` parent command. `specscore event` with no subcommand MUST print help and exit `0` (matching the parent `cli` Feature's REQ:verb-subcommands). `specscore event emit --help` MUST exit `0` and print the verb's full usage including every flag defined by REQ:envelope-flags and REQ:payload-input-modes.

### Envelope construction from flags

The CLI supplies the envelope's stable fields and auto-fills the bookkeeping fields.

#### REQ: envelope-flags

`specscore event emit` MUST accept the following flags, all required unless marked optional:

| Flag | Argument | Maps to envelope field |
|---|---|---|
| `--name` | event name (e.g. `idea.drafted`) | `name` |
| `--actor-kind` | one of `skill`, `user`, `external` | `actor.kind` |
| `--actor-id` | string | `actor.id` |
| `--artifact-type` | one of `idea`, `feature`, `plan`, `task`, `lesson`, `idea-seed`, `consilium-review` | `artifact.type` |
| `--artifact-id` | string | `artifact.id` |
| `--artifact-path` | repo-relative path string | `artifact.path` |
| `--artifact-revision` (optional) | git SHA or the literal `uncommitted` | `artifact.revision` (overrides auto-fill — see REQ:envelope-auto-fill) |

A missing required flag MUST exit `2` with a stderr message naming the flag and the envelope field it supplies. The CLI MUST NOT accept extra positional arguments — only flag-form input — to keep the call shape stable across shells.

#### REQ: envelope-auto-fill

The CLI MUST auto-fill the following envelope fields when the user does not supply them. Each rule below is a default; if the user passes the corresponding flag (where one exists), the user-supplied value wins.

| Field | Auto-fill rule |
|---|---|
| `version` | Always set to the integer `1`. There is no override flag in v1; future envelope-schema versions will add `--envelope-version`. |
| `uuid` | A freshly-generated UUID v4 (lowercase, hyphenated). There is no override flag in v1 (regeneration is the point — duplicate uuids defeat consumer dedup). |
| `timestamp` | The current wall-clock time in UTC, formatted RFC 3339 with the `Z` suffix (e.g. `2026-05-22T13:46:57Z`). There is no override flag in v1. |
| `artifact.revision` | The output of `git rev-parse HEAD` run in the project root. If the project root is not a git repository OR `git rev-parse HEAD` fails (e.g. no commits yet), the field MUST be filled with the literal string `uncommitted`. The user MAY override via `--artifact-revision` (REQ:envelope-flags) — for example, to emit an event for a specific historical revision during replay. |

### Payload input

The payload is opaque to the CLI (per the parent's REQ:payload-opaque clause); the CLI's only job is to read its bytes from one of three sources.

#### REQ: payload-input-modes

`specscore event emit` MUST accept exactly one of the following payload input modes per invocation:

| Mode | Flag / source | Behavior |
|---|---|---|
| Inline JSON | `--payload-json '<json>'` | The flag value is the payload bytes. |
| File | `--payload-file <path>` | The CLI reads the file's bytes. The path MAY be absolute or project-root-relative. |
| Stdin (default) | (no payload flag) | The CLI reads payload bytes from stdin until EOF. |

Exactly one input mode is permitted per invocation. Passing both `--payload-json` and `--payload-file`, or either flag together with a non-TTY stdin that contains data, MUST exit `2` with a stderr message naming the conflict. When no payload flag is supplied AND stdin is a TTY, the CLI MUST exit `2` with a stderr message naming the three modes — the verb MUST NOT block waiting for keyboard input that the caller almost certainly did not intend.

The CLI MUST verify the payload bytes parse as a JSON object (per the parent's REQ:envelope-validation) before enqueueing them. Bytes that don't parse MUST exit `2` with a stderr message naming the input mode and the JSON parse error.

### Durable enqueue and delivery

The verb hands off to the parent's durable outbox and adopts its outcome.

#### REQ: dispatch-handoff

After constructing the envelope and validating the payload, the verb MUST exclusively enqueue the event with the complete canonical subscriber-name set, commit it, and replay every currently pending subscriber. An enqueue/ledger error or any remaining failed delivery MUST exit `10`; successful delivery to all subscribers or an explicit empty subscriber set MUST exit `0`. Each failure MUST remain inspectable for `event replay`, and success at another subscriber MUST NOT acknowledge it.

#### REQ: help-output

`specscore event emit --help` MUST include a one-line link to `https://specscore.md/event-emit` (the canonical docs URL for this verb) so that users discovering the flag set can find the full prose documentation. The help text MUST enumerate every flag defined by REQ:envelope-flags and REQ:payload-input-modes with their argument shape and a one-line description.

## Architecture & Components

| Unit | Responsibility | Used by | Depends on |
|---|---|---|---|
| `internal/cli/event.go` | Registers the event command tree and implements emit: resolve payload, construct and validate the envelope, enqueue, and replay pending delivery. | cobra root. | `pkg/event`, `os` (stdin), Git revision lookup. |

The verb owns no in-memory retry worker. Durable state lives under the parent Feature's project-local outbox.

## Data Flow

```
$ specscore event emit \
    --name idea.drafted \
    --actor-kind skill --actor-id skill:specstudio:ideate \
    --artifact-type idea \
    --artifact-id event-emit-dispatcher \
    --artifact-path spec/ideas/event-emit-dispatcher.md \
    --payload-json '{"slug":"event-emit-dispatcher","approved":false}'
  │
  ├─→ Parse flags (REQ:envelope-flags); validate required set
  │     missing flag → exit 2 with stderr
  │
  ├─→ Resolve payload input mode (REQ:payload-input-modes)
  │     conflict or TTY stdin → exit 2 with stderr
  │     bytes don't parse JSON → exit 2 with stderr
  │
  ├─→ Auto-fill envelope fields (REQ:envelope-auto-fill)
  │     version=1, uuid=<v4>, timestamp=<now UTC>, revision=<git HEAD or "uncommitted">
  │
  └─→ pkg/event.Outbox.Enqueue(event, subscribers) (REQ:dispatch-handoff)
        └─→ Replay every pending subscriber independently
              all acknowledged → exit 0
              ledger/delivery pending → exit 10 and retain inspectable work
```

## Error Handling & Failure Modes

| Failure | Exit | Behavior |
|---|---|---|
| Required flag missing | `2` | Stderr names the missing flag and the envelope field it supplies. |
| Both `--payload-json` and `--payload-file` supplied | `2` | Stderr names the conflict and the three accepted input modes. |
| Either payload flag supplied AND non-empty stdin piped | `2` | Same. |
| No payload flag supplied AND stdin is a TTY | `2` | Stderr names the three accepted input modes and recommends piping or `--payload-json`. |
| Payload bytes don't parse as a JSON object | `2` | Stderr names the input mode (`stdin` / `--payload-file <path>` / `--payload-json`) and the JSON parse error. |
| `--artifact-revision` not supplied AND `git rev-parse HEAD` fails (no commits, not a git repo) | `0` (success path) | Auto-fills `artifact.revision = "uncommitted"`. NOT a failure. |
| Envelope-validation failure (e.g. bad event-name pattern) | `2` | Delegated to the parent's REQ:envelope-validation. Stderr per the parent's contract. |
| Config-load failure (`events:` block malformed) | `2` | Delegated to the parent's REQ:events-config-schema. |
| `specscore.yaml` not found | `3` | Delegated to the parent `cli` Feature's REQ:project-autodetect. |
| Any subscriber remains pending | `10` | Stderr names each pending subscriber; another subscriber's success does not mask it. |
| Every subscriber succeeds, or subscriber list is explicitly empty | `0` | Every required acknowledgement is durable. |

## Testing Strategy

Tests cover flag validation, payload-mode arbitration, auto-fill values, durable enqueue, per-subscriber acknowledgement, replay, and exit-code mapping. The parent Feature tests ledger and dispatcher primitives independently.

## Rehearse Integration

| AC | Stub? | Rationale |
|---|---|---|
| `verb-registers-and-helps` | yes | `specscore event emit --help` exit `0`, contains the docs link |
| `required-flag-missing-fails-2` | yes | Omit `--name`; assert exit 2 + stderr names the flag |
| `payload-json-flag-shape` | yes | `--payload-json '{"k":"v"}'`; assert recording subscriber sees `{"k":"v"}` |
| `payload-file-flag-shape` | yes | `--payload-file p.json`; assert recording subscriber sees file content |
| `payload-stdin-shape` | yes | Pipe JSON to stdin; assert recording subscriber sees it |
| `payload-mode-conflict-fails-2` | yes | Both `--payload-json` and `--payload-file`; assert exit 2 |
| `payload-tty-stdin-fails-2` | yes | No flags, TTY stdin (PTY in test harness); assert exit 2 |
| `payload-bad-json-fails-2` | yes | `--payload-json 'not json'`; assert exit 2 |
| `envelope-auto-fill-fields` | yes | Pass minimum flags; assert recording subscriber sees envelope with `version=1`, a UUID v4 `uuid`, an RFC 3339 `timestamp`, an `artifact.revision` matching `git rev-parse HEAD` |
| `envelope-auto-fill-revision-no-git` | yes | Project root with no `.git`; assert `artifact.revision = "uncommitted"` |
| `envelope-artifact-revision-override` | yes | Pass `--artifact-revision deadbeef`; assert recording subscriber sees that exact value |
| `dispatch-exit-code-handoff` | yes | Configure one successful and one failing subscriber; assert verb exits `10` and only the failed sink remains replayable |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [`cli/event`](../README.md) (parent) | Owns envelope validation, subscribers, the immutable ledger, per-subscriber pending markers, replay, and reconciliation. |
| [`cli`](../../README.md) (root) | The `event` cobra subcommand attaches under root. Inherits the shared exit-code contract; the verb adds no new exit-code semantics beyond delegation. Inherits `--project` autodetect and the global `--help` / `-h` flag. |
| Event-emitting skills in the SDD skills repo | Out of scope of this Feature. The `verify` skill is the proof-point port — its bash block, in a follow-up change to that repo, drops the inline JSONL append and calls this verb unconditionally. Migration of the other five event-emitting skills is tracked in the SDD skills repo, not gated on this Feature. |

## Not Doing / Out of Scope

- **The dispatcher, envelope validator, subscriber implementations, and `events:` config schema.** Owned by the parent Feature [`cli/event`](../README.md).
- **The `verify` skill port itself.** Lands in a follow-up change to the SDD skills repo. This Feature's contract (the flag set + the auto-fill semantics + exit codes) is what the port consumes.
- **Migration of the five remaining event-emitting skills.** Out of scope; tracked in the SDD skills repo.
- **Per-event-name flag sets** (e.g. `--payload-slug`, `--payload-hmw`). Deliberately rejected during ideation: per-event flag sets couple CLI releases to skill releases, which the source Idea explicitly avoids. The payload-input-modes contract keeps the CLI envelope-only.
- **Override flags for `--version`, `--uuid`, `--timestamp`.** Auto-fill is the contract in v1; override flags can be added later if a replay/import use case requires them.
- **Reading events back** (a `specscore event log` or `event tail` verb). Deliberately omitted per the source Idea's revision; `jq . .specscore/events.jsonl` covers the read path.

## Assumption Carryover

| Idea assumption | Status after this spec |
|---|---|
| The six event-emitting skills can be ported with no behavioral regression. | **Tested** by the `envelope-auto-fill-fields` AC plus the proof-point port of `verify` (which lives in a follow-up). Byte-level equality of the JSONL line written via this verb vs. the legacy inline bash block is the regression test. |
| Incremental skill migration without flag-day. | **Inherited.** This Feature ships the verb; the SDD skills repo ports skills one by one against the verb's stable flag set. No coordination required beyond the verb's release. |

## Acceptance Criteria

### AC: verb-registers-and-helps

**Requirements:** cli/event/emit#req:verb-registration, cli/event/emit#req:help-output

**Given** a `specscore` binary built from this Feature's implementation
**When** the following commands run: `specscore event`, `specscore event --help`, `specscore event emit --help`
**Then** `specscore event` MUST exit `0` and print help that lists `emit` as an available subcommand; `specscore event --help` MUST exit `0`; `specscore event emit --help` MUST exit `0`, print every flag defined by REQ:envelope-flags and REQ:payload-input-modes, and include the literal string `https://specscore.md/event-emit` somewhere in its output.

### AC: required-flag-missing-fails-2

**Requirements:** cli/event/emit#req:envelope-flags

**Given** a `specscore` binary and a valid project root with default `events:` config
**When** `specscore event emit --actor-kind skill --actor-id skill:t --artifact-type idea --artifact-id x --artifact-path spec/ideas/x.md --payload-json '{}'` runs (missing `--name`)
**Then** exit code MUST be `2`; stderr MUST contain a message naming the missing flag (`--name`) and the envelope field it supplies; no event MUST be written to `.specscore/events.jsonl`.

### AC: payload-json-flag-shape

**Requirements:** cli/event/emit#req:payload-input-modes

**Given** a `specscore` binary, a project root with `events:` config pointing at a recording test subscriber, and a valid envelope flag set
**When** the verb runs with `--payload-json '{"slug":"x","approved":false}'`
**Then** exit `0`; the recording subscriber MUST have received an envelope whose `payload` field decodes to `{"slug":"x","approved":false}` (field-by-field equality; the dispatcher MAY canonicalize JSON formatting).

### AC: payload-file-flag-shape

**Requirements:** cli/event/emit#req:payload-input-modes

**Given** a payload file at `/tmp/p.json` containing `{"slug":"x","approved":false}`, a `specscore` binary, and a recording subscriber as above
**When** the verb runs with `--payload-file /tmp/p.json`
**Then** exit `0`; the recording subscriber MUST have received the same payload as the `--payload-json` case (byte-identical after canonicalization).

### AC: payload-stdin-shape

**Requirements:** cli/event/emit#req:payload-input-modes

**Given** the same setup as the previous two ACs
**When** `printf '%s' '{"slug":"x","approved":false}' | specscore event emit <envelope flags>` runs (no `--payload-json`, no `--payload-file`, stdin piped)
**Then** exit `0`; the recording subscriber MUST have received the same payload as the previous two ACs.

### AC: payload-mode-conflict-fails-2

**Requirements:** cli/event/emit#req:payload-input-modes

**Given** a `specscore` binary
**When** the verb runs with both `--payload-json '{}'` and `--payload-file /tmp/p.json`
**Then** exit code MUST be `2`; stderr MUST name the conflict (both flags) and enumerate the three accepted input modes.

### AC: payload-tty-stdin-fails-2

**Requirements:** cli/event/emit#req:payload-input-modes

**Given** a `specscore` binary invoked from a PTY (TTY stdin) with no payload flag set
**When** `specscore event emit <envelope flags>` runs
**Then** exit code MUST be `2` within 1 second (the verb MUST NOT block waiting for keyboard input); stderr MUST name the three accepted input modes.

### AC: payload-bad-json-fails-2

**Requirements:** cli/event/emit#req:payload-input-modes

**Given** a `specscore` binary
**When** the verb runs with `--payload-json 'not json'`
**Then** exit code MUST be `2`; stderr MUST name the input mode (`--payload-json`) and a JSON parse error description; no subscriber MUST be called.

### AC: envelope-auto-fill-fields

**Requirements:** cli/event/emit#req:envelope-auto-fill

**Given** a `specscore` binary, a project root that is a git repository with at least one commit, and a recording subscriber
**When** the verb runs with the minimum required flag set (no `--artifact-revision`, no envelope-override flags) and `--payload-json '{}'`
**Then** exit `0`; the recording subscriber MUST have received an envelope where `version == 1`; `uuid` matches the v4 pattern `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`; `timestamp` matches RFC 3339 UTC with the `Z` suffix and is within ±5 seconds of the call time; `artifact.revision` equals the output of `git rev-parse HEAD` run from the project root immediately before the call.

### AC: envelope-auto-fill-revision-no-git

**Requirements:** cli/event/emit#req:envelope-auto-fill

**Given** a project root that is NOT a git repository (no `.git/` directory) AND has a valid `specscore.yaml`, a `specscore` binary, and a recording subscriber
**When** the verb runs with the minimum required flag set
**Then** exit `0`; the recording subscriber MUST have received an envelope where `artifact.revision == "uncommitted"` (the literal string).

### AC: envelope-artifact-revision-override

**Requirements:** cli/event/emit#req:envelope-flags, cli/event/emit#req:envelope-auto-fill

**Given** a project root that IS a git repository, a `specscore` binary, and a recording subscriber
**When** the verb runs with `--artifact-revision deadbeef00000000000000000000000000000000` (an arbitrary 40-char hex string) and the minimum other flags
**Then** exit `0`; the recording subscriber MUST have received an envelope where `artifact.revision == "deadbeef00000000000000000000000000000000"`; the value MUST NOT be replaced by the auto-fill from `git rev-parse HEAD`.

### AC: dispatch-exit-code-handoff

**Requirements:** cli/event/emit#req:dispatch-handoff

**Given** a `specscore.yaml` configured with one successful subscriber and one Exec subscriber pointing at `/bin/false`, a `specscore` binary
**When** the verb runs with a valid envelope and any payload mode
**Then** exit code MUST be `10`; stderr MUST name the failed subscriber; the successful subscriber MUST be acknowledged; the failed subscriber MUST remain independently replayable with the same event UUID.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
