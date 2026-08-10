---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: Events (CLI)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/event?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/event?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/event?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/event?op=request-change) |
**Status:** Stable
**Date:** 2026-05-22
**Owner:** alexandertrakhimenok
**Source Ideas:** event-emit-dispatcher
**Supersedes:** —

## Summary

The shared reliable event-delivery plumbing for the `specscore` CLI. It owns the `pkg/event` envelope, subscribers, direct dispatcher, and durable per-subscriber outbox. `specscore event emit` and artifact mutations prepare or enqueue one immutable event for the complete subscriber set; `event replay` retries only unacknowledged deliveries; `event reconcile` resolves an interrupted prepared record against artifact evidence.

## Contents

| Child | Description |
|---|---|
| [emit](emit/README.md) | The `specscore event emit` verb — cobra wiring, envelope flags, payload input modes, dispatch invocation, exit-code mapping. |

## Problem

A cross-repo event contract (declared in the SDD skills repository) requires every event-emitting skill to route its events through a single CLI entry point. The verb does not exist today. Six event-emitting skills each carry an inline JSONL-append bash block that hard-codes `.specscore/events.jsonl` and the envelope shape, so every new event-aware skill pays the same duplication tax. There is no extension point: a downstream automation that wants to react to events (e.g. trigger a workflow when `idea.approved` fires) must tail the JSONL file out-of-band, and a user who wants to silence events must edit each skill.

A shared dispatch package solves all three. It owns the envelope shape, the file format, the subscriber registry, and the YAML config surface — so skill authors construct events through one typed entry point, downstream consumers register through one config block, and the CLI's `events:` configuration is the single source of truth for "where do events go on this project."

## Behavior

### Subscriber interface

The package exposes one extension point — anything that can receive an event implements `event.Subscriber`.

#### REQ: pkg-event-package-location

The dispatch package MUST live at `pkg/event/` in the repo, importable as `github.com/specscore/specscore-cli/pkg/event`. Subscriber implementations MAY be subfiles within this package (e.g. `pkg/event/jsonl.go`, `pkg/event/exec.go`, `pkg/event/noop.go`); they MUST NOT live in sibling or parent packages. Per the `pkg/` layout used by `pkg/feature`, `pkg/idea`, `pkg/lifecycle`, `pkg/plan`, `pkg/lint`, `pkg/task`, the package name is the singular `event`, not `events`.

#### REQ: subscriber-interface

`pkg/event` MUST export an interface `Subscriber` with two methods:

| Method | Signature | Purpose |
|---|---|---|
| `Deliver` | `Deliver(ctx context.Context, e Event) error` | Invoked by the dispatcher with the validated envelope. Returns nil on successful delivery; non-nil on any failure (timeout, exec exit non-zero, filesystem error). |
| `Name` | `Name() string` | Stable identifier used in stderr failure logs. The dispatcher does not interpret the string. |

The `Event` type is a Go struct mirroring the envelope shape with fields `Name`, `Version`, `UUID`, `Timestamp`, `Actor`, `Artifact`, and `Payload`. `Payload` is `json.RawMessage` — the dispatcher does not parse it (envelope-only validation; see REQ:envelope-validation). The interface MUST be safe to call repeatedly within a single CLI invocation.

### First-class subscribers

The package ships three subscriber implementations.

#### REQ: jsonl-writer-subscriber

`pkg/event` MUST export a `JsonlWriter` type implementing `Subscriber` whose `Deliver` method:

1. Serializes the `Event` to single-line JSON (no embedded newlines; multi-line payload content is JSON-escaped per the standard).
2. Appends the serialized line plus a single trailing `\n` to a configured file path, opened with `O_APPEND|O_CREATE|O_WRONLY`, mode `0644`. Parent directories MUST be created with mode `0755` if absent.
3. Returns nil on successful write; on filesystem error returns the underlying error wrapped with the file path.

`Name()` MUST return `jsonl:<path>` (e.g. `jsonl:.specscore/events.jsonl`).

The configured path MAY be absolute or relative. Relative paths MUST be resolved against the project root (the directory containing the `specscore.yaml` discovered for the invocation), NOT against cwd. This makes the default `.specscore/events.jsonl` semantics deterministic for callers invoked from subdirectories of a monorepo.

#### REQ: noop-subscriber

`pkg/event` MUST export a `NoOp` type implementing `Subscriber` whose `Deliver` method does nothing and returns nil. `Name()` MUST return the literal string `noop`. This is the explicit-disable subscriber: configuring `events: { subscribers: [{ type: noop }] }` disables all event emission for the project.

#### REQ: exec-subscriber

`pkg/event` MUST export an `Exec` type implementing `Subscriber` whose `Deliver` method:

1. Builds an `os/exec.Cmd` from the configured `command` array (the first element is the executable, subsequent elements are positional arguments). The configured `env` mapping MUST be appended to the child process's environment (in addition to, not in place of, the parent process's environment).
2. Pipes the serialized event JSON (same single-line form as `JsonlWriter`'s output, followed by a trailing `\n`) to the child's stdin, then closes stdin (signalling EOF).
3. Inherits the parent process's stderr so the subscriber's diagnostic output reaches the user. Discards stdout (subscribers MUST NOT return data to the dispatcher in v1).
4. Enforces a hard wall-clock timeout per REQ:exec-subscriber-timeout. The budget starts before process-tree setup; expiry at any setup or start boundary MUST return the same distinguishable timeout error as expiry while the command runs. Timeout cleanup MUST include descendants started by the command, while successful delivery MUST NOT terminate background descendants. On Unix, the dispatcher MUST send SIGTERM to an owned command process group, wait 100 ms, then send SIGKILL to that same group even if SIGTERM reports an error; the group identity MUST remain owned through that grace interval. On Windows, where SIGTERM has no equivalent, the dispatcher MUST create the command suspended, assign its retained process handle to an owned Job Object, then resume it; on timeout it MUST use `TerminateJobObject` to forcefully terminate the command and its descendants. If that Job Object operation fails, it MUST immediately attempt to kill the retained direct child and MUST bound cleanup waiting so delivery cannot hang. Closing the Job Object after a successful command MUST NOT terminate its remaining descendants.
5. Returns nil when the child exits with code 0; returns an error wrapping the child's exit code on non-zero exit; returns a distinguishable timeout error on timeout.

`Name()` MUST return `exec:<argv[0]>` (e.g. `exec:my-event-consumer`).

#### REQ: exec-subscriber-timeout

The `Exec` subscriber MUST enforce a hard wall-clock timeout on each `Deliver` invocation, beginning before any platform process-tree setup. If the deadline expires during setup, `cmd.Start`, or `afterStart` ownership establishment, it MUST return the same distinguishable timeout error used for a running command; it MUST NOT expose a platform-specific setup error as a non-timeout failure. The default timeout is **2000 ms**. The default MAY be overridden per-subscriber via the `timeout_ms` field in the subscriber's config entry (REQ:events-config-schema). Values outside the range `[100, 30000]` MUST be rejected at config-load time with an error naming the offending entry.

### Configuration

Subscribers are declared in `specscore.yaml` under a top-level `events:` block.

#### REQ: events-config-schema

The CLI MUST accept an optional top-level `events:` mapping in `specscore.yaml` with the following shape:

```yaml
events:
  subscribers:
    - type: jsonl                    # required; one of: jsonl, noop, exec
      name: <string>                 # optional; used only in stderr failure logs
      path: <string>                 # required when type=jsonl
    - type: noop
      name: <string>                 # optional
    - type: exec
      name: <string>                 # optional
      command: [<argv0>, <arg1>, …]  # required when type=exec; non-empty argv list
      env:                           # optional; appended to child process environment
        KEY: VALUE
      timeout_ms: <int>              # optional; default 2000; must be in [100, 30000]
```

Validation rules:

- Unknown `type` values MUST cause a config-load error naming the offending entry, its key path (`events.subscribers[N].type`), and the closed enum of accepted values.
- Unknown keys within a subscriber entry MUST cause the same form of error.
- Missing required keys for a given `type` (`path` for jsonl; `command` for exec) MUST cause the same.
- `command` MUST be a non-empty list of non-empty strings.
- `timeout_ms`, when present, MUST be an integer in `[100, 30000]`.

#### REQ: default-and-empty-config

When `specscore.yaml` does not contain an `events:` mapping at all, the dispatcher MUST behave as if exactly one subscriber were declared:

```yaml
events:
  subscribers:
    - type: jsonl
      path: .specscore/events.jsonl
```

This preserves the pre-dispatcher convention as the zero-config default. An **explicitly empty** subscriber list (`events: { subscribers: [] }`) MUST be treated as the explicit no-subscribers configuration: no event reaches a sink, dispatch reports success, and no recipientless outbox record is created. A configured `noop` also has no external sink effect but does retain a durable delivery acknowledgement. Both forms MUST be accepted; the dispatcher MUST NOT synthesize the default when the user has written an empty list.

### Envelope validation

The dispatcher validates the common envelope before invoking any subscriber.

#### REQ: envelope-validation

Before any subscriber's `Deliver` is called, the dispatcher MUST validate that the event envelope satisfies all of:

- `name` is a non-empty string matching `^[a-z][a-z0-9-]*\.[a-z][a-z0-9-]*$` (e.g. `idea.drafted`, `feature.approved`, `sidekick-idea.captured`).
- `version` is a positive integer.
- `uuid` is a lowercase, hyphenated UUID v4.
- `timestamp` parses as RFC 3339 with a UTC offset (the `Z` suffix is canonical; explicit `+00:00` is accepted).
- `actor.kind` is one of `skill`, `user`, `external`. (`external` is the value used by automations that drive the CLI without being a skill or a human at a terminal.)
- `actor.id` is a non-empty string.
- `artifact.type` is one of `idea`, `feature`, `plan`, `task`, `idea-seed`, `consilium-review`. Future event-emitting callers MAY extend this enum by amending this REQ.
- `artifact.id` is a non-empty string.
- `artifact.path` is a non-empty string.
- `artifact.revision` is a non-empty string. The literal `uncommitted` is accepted for pre-commit emissions.
- `payload` is present and parses as a JSON object (`{}` is acceptable). The dispatcher MUST NOT inspect, validate, or transform payload fields — payload schemas are out of scope (REQ:payload-opaque clause below).

**REQ:payload-opaque clause.** Payload is passed through to subscribers as opaque JSON. New event names MAY ship in event-emitting callers without modifying or releasing the CLI, provided the envelope satisfies the rules above. Per-event payload schemas are tracked as an Open Question for a future additive Feature.

Validation failure MUST exit non-zero per REQ:dispatch-exit-codes with a stderr message naming the offending field and the rule violated. No subscriber's `Deliver` MUST be called when validation fails.

### Dispatch semantics

The dispatcher fans events out to every configured subscriber and tolerates per-subscriber failure.

#### REQ: fan-out-dispatch

On a valid envelope, the dispatcher MUST iterate the configured subscriber list in declared order and call `Deliver` on each. A non-nil error or timeout from one subscriber MUST NOT prevent subsequent subscribers from being invoked. Subscribers MUST be invoked sequentially (no concurrent invocation in v1); future asynchronous dispatch is explicitly out of scope per the source Idea.

When a subscriber's `Deliver` returns a non-nil error, the dispatcher MUST write one line of diagnostic output to stderr in the following key=value form:

```
event-dispatch failure: subscriber=<Name()> event=<envelope.name> error="<error string>"
```

The `error` value MUST be wrapped in double quotes; embedded double quotes MUST be escaped with a backslash. A successful `Deliver` MUST NOT produce stderr output.

#### REQ: dispatch-exit-codes

The dispatcher MUST map outcomes to exit codes consistent with the parent `cli` Feature's shared exit-code contract:

| Code | Condition | Maps to standard |
|---|---|---|
| `0` | The configured subscriber list is empty OR at least one subscriber's `Deliver` returned nil. | Success. |
| `2` | Envelope validation failed (REQ:envelope-validation) OR the loaded `events:` block failed schema validation (REQ:events-config-schema). | Invalid arguments / malformed input. |
| `3` | `specscore.yaml` not found via the standard `--project` autodetect (the parent `cli` Feature's REQ:project-autodetect). | Resource not found. |
| `10` | The configured subscriber list is non-empty AND every subscriber's `Deliver` returned a non-nil error or timed out. | Unexpected runtime error. |

The "single subscriber failed, others succeeded" case is success (exit 0) with stderr diagnostics per REQ:fan-out-dispatch. The "explicitly empty subscriber list" case is also success (REQ:default-and-empty-config).

### Durable per-subscriber delivery

The dispatcher remains the synchronous delivery primitive. Public emission and artifact-mutation commands place a validated envelope in the durable outbox before delivery so success at one sink cannot erase another sink's failure.

#### REQ: immutable-prepared-ledger

`Prepare` MUST exclusively publish one immutable ledger record containing the validated event and the complete sorted, unique, nonempty subscriber-name set before the corresponding artifact mutation. An explicitly empty configured subscriber list is the project-level opt-out and MUST create no recipientless ledger. The event UUID is the delivery idempotency key. Ledger publication MUST be no-clobber and parent-directory durable. Commit and abort MUST use per-event synchronization and refuse invalid state transitions.

Before replay or recovery, the reader MUST reject unknown or trailing JSON, an invalid event envelope, and an empty, duplicate, or unsorted subscriber set. A schema-valid malicious local rewrite cannot currently be authenticated and MUST remain an explicitly Planned capability rather than a claimed guarantee.

#### REQ: reconstructible-pending-delivery

Commit MUST make the ledger record authoritative before delivery and reconstruct one pending marker for every subscriber from that ledger. One successful delivery acknowledges only its own subscriber. Failure or interruption leaves that subscriber inspectable and replayable even when another subscriber succeeds. Delivery is at least once around a subscriber-success/ack interruption, so subscribers MUST deduplicate by event UUID.

#### REQ: prepared-reconciliation

A prepared record left by a stopped process MUST remain visible. `specscore event reconcile` MUST require an explicit commit-or-abort decision and confirmation token after the operator compares the record with its artifact evidence. It MUST NOT silently ignore or guess the outcome. Artifact writers MUST keep an inspectable prepared record if artifact mutation succeeds but event commit cannot complete.

### Documentation surface

The repo carries a single human-readable events document at a stable location.

#### REQ: docs-events-md-skeleton

The repository MUST contain `docs/events.md` with the following second-level sections, in order: `Overview`, `The events: config block`, `Built-in subscribers`, `Writing an Exec subscriber`, `Envelope shape`, `Default behavior`, `Disabling events`. The `Built-in subscribers` section MUST contain one third-level subsection per shipped subscriber type (`### jsonl`, `### noop`, `### exec`). The `Envelope shape` section MUST mirror REQ:envelope-validation's rule set as user-facing prose.

## Architecture & Components

| Unit | Responsibility | Used by | Depends on |
|---|---|---|---|
| `pkg/event/` (Go package) | The only place subscriber implementations live. Owns the `Subscriber` interface, the three built-in subscribers, the envelope type, the dispatcher, and the config loader. | The emit verb in `internal/cli/event_emit.go` (defined by `cli/event/emit`). Future event-consuming verbs attach here. | `pkg/projectdef` for project-root resolution; YAML library for config parsing; `os/exec` stdlib for the Exec subscriber. |
| `pkg/event/subscriber.go` | The `Subscriber` interface and the `Event` envelope struct. | The dispatcher; every subscriber implementation. | None. |
| `pkg/event/dispatcher.go` | Iterates the configured subscriber list and calls `Deliver` on each. Implements REQ:fan-out-dispatch and the failure-logging contract. | The emit verb. | `pkg/event/subscriber.go`. |
| `pkg/event/outbox.go` | Publishes immutable prepared records, reconstructs per-subscriber pending markers, records acknowledgements, and exposes replay/reconciliation. | The emit verb and artifact-mutation commands. | `pkg/event/subscriber.go`; project-local `.specscore/event-outbox/`. |
| `pkg/event/envelope.go` | The envelope validator (REQ:envelope-validation). Pure function: `Validate(Event) error`. | The dispatcher. | Stdlib regex for `name` pattern. |
| `pkg/event/jsonl.go` | The `JsonlWriter` subscriber. | Dispatcher (via config loader). | Stdlib `os`. |
| `pkg/event/noop.go` | The `NoOp` subscriber. | Dispatcher (via config loader). | None. |
| `pkg/event/exec.go` | The `Exec` subscriber with per-call timeout and SIGTERM/SIGKILL handling. | Dispatcher (via config loader). | `os/exec`, `context`, `syscall` stdlib. |
| `pkg/event/config.go` | Parses the `events:` block from `specscore.yaml`, constructs subscriber instances, enforces REQ:events-config-schema validation. Applies the default-when-absent and explicit-empty-list rules of REQ:default-and-empty-config. | Dispatcher initialization. | YAML library; `pkg/projectdef`. |
| `docs/events.md` | Human-readable, single page covering the verb, the config block, and how to write an Exec subscriber. Stable URL referenced from `specscore event --help` and `specscore event emit --help`. | Users; the emit verb's help text. | This Feature. |

## Data Flow

```
specscore event emit --name=<n> --actor-kind=<k> ... [--payload-json|--payload-file|stdin]
   │
   ├─→ Load specscore.yaml (pkg/projectdef + pkg/event/config.go)
   │    - events: absent → synthesize [{type: jsonl, path: .specscore/events.jsonl}]
   │    - events: {subscribers: []} → empty list (dispatch is a no-op, exit 0)
   │    - events: {subscribers: [...]} → parse and validate each entry
   │    - on schema violation → exit 2 with stderr message naming key path
   │
   ├─→ Construct Event envelope (verb owns this; see child Feature)
   │    - flags supply name, actor.*, artifact.{type,id,path}
   │    - CLI auto-fills version=1, uuid (UUID v4), timestamp (RFC 3339 UTC), artifact.revision (git HEAD or "uncommitted")
   │    - payload bytes read from --payload-json | --payload-file | stdin
   │
   ├─→ pkg/event.Validate(Event)
   │    - on validation failure → exit 2 with stderr message naming offending field
   │    - on success → continue
   │
   └─→ pkg/event.Dispatch(ctx, Event, subscribers)
        for each subscriber in declared order:
          err := subscriber.Deliver(ctx, Event)
          if err != nil:
            stderr <- "event-dispatch failure: subscriber=<Name()> event=<n> error=\"…\""
            continue
        if all_failed (and list was non-empty):
          exit 10
        else:
          exit 0
```

## Error Handling & Failure Modes

| Failure | Behavior |
|---|---|
| `specscore.yaml` not found by autodetect | Exit 3 (parent `cli` Feature's REQ:project-autodetect). |
| `events:` block has unknown `type` | Exit 2 with stderr naming the file, key path, and accepted enum (REQ:events-config-schema). No subscriber called. |
| `events.subscribers[N].command` missing for type=exec | Exit 2 with stderr naming the missing key. No subscriber called. |
| `timeout_ms` outside `[100, 30000]` | Exit 2 with stderr naming the offending value and bounds. No subscriber called. |
| Envelope validation fails (e.g. bad event-name pattern, missing actor.id) | Exit 2 with stderr naming the offending field and rule. No subscriber called. |
| `JsonlWriter` cannot create the parent directory | `Deliver` returns error; logged per REQ:fan-out-dispatch; dispatch continues to next subscriber. |
| `JsonlWriter` write fails (disk full, permission denied) | Same as above. |
| `Exec` subscriber binary not found in PATH | `Deliver` returns error from `cmd.Start`; logged; dispatch continues. |
| `Exec` subscriber exits non-zero | `Deliver` returns error wrapping the exit code; logged; dispatch continues. |
| `Exec` subscriber hangs past `timeout_ms` | Dispatcher terminates the owned command tree (SIGTERM then SIGKILL on Unix; Job Object termination on Windows); `Deliver` returns timeout error; logged; dispatch continues. |
| All non-empty subscribers fail | Exit 10 (REQ:dispatch-exit-codes). |
| Explicitly empty subscriber list (`events: { subscribers: [] }`) | Exit 0; no subscriber called; no stderr output. |
| Concurrent invocations writing the same JSONL file | POSIX `O_APPEND` guarantees per-write atomicity for writes within `PIPE_BUF` (4 KiB on Linux, 512 B on macOS); serialized envelope JSON lines fit well under that limit. No CLI-side locking required. |

## Testing Strategy

Per-AC Rehearse stubs are scaffolded for the testable ACs (envelope validation, config-schema parsing, subscriber dispatch with mock subscribers, timeout enforcement). The documentation-only AC (`docs-events-md-skeleton-present`) is verifiable by lint over the markdown file and does not get a runtime stub.

## Rehearse Integration

| AC | Stub? | Rationale |
|---|---|---|
| `subscriber-interface-shape` | yes | Pure Go API surface — `go vet` / build covers it; supplement with a constructor test |
| `jsonl-writer-appends-line` | yes | Filesystem + file content snapshot |
| `jsonl-writer-resolves-against-project-root` | yes | Run from a subdirectory; assert file lands at project-root-relative path |
| `noop-discards` | yes | Pure-function test |
| `exec-pipes-event-to-stdin` | yes | Spawn a test subscriber that records stdin to a file; verify content matches the envelope JSON |
| `exec-timeout-kills-hung-process` | yes | Spawn a `sleep` subscriber with `timeout_ms: 200`; assert SIGKILL by ~300 ms |
| `events-config-default-when-absent` | yes | Project without `events:` block; assert the synthesized JsonlWriter at `.specscore/events.jsonl` |
| `events-config-explicit-empty-is-noop` | yes | `events: { subscribers: [] }`; assert exit 0 with zero subscribers invoked |
| `events-config-unknown-type-rejected` | yes | Malformed config; assert exit 2 + stderr |
| `envelope-validation-rejects-bad-name` | yes | Bad event-name pattern; assert exit 2 + stderr |
| `envelope-validation-rejects-bad-actor-kind` | yes | Unknown `actor.kind`; assert exit 2 + stderr |
| `payload-is-opaque-passthrough` | yes | Arbitrary JSON payload; assert delivered verbatim to a recording subscriber |
| `fan-out-continues-after-failure` | yes | Two subscribers, first fails; assert second still called; exit 0 |
| `per-subscriber-failure-stderr-format` | yes | Assert key=value pattern against the exact string |
| `dispatch-exit-code-when-all-fail` | yes | Configure two failing subscribers; assert exit 10 |
| `docs-events-md-skeleton-present` | no | Static doc structure; covered by lint over the markdown file |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [`cli`](../README.md) (root) | The new `event` subcommand attaches to the root cobra command. Inherits the shared exit-code contract (REQ:dispatch-exit-codes aligns to codes 0/2/3/10 from the root). Inherits `--project` autodetect (REQ:project-autodetect). |
| [`cli/telemetry`](../telemetry/README.md) | Independent. Telemetry emits anonymous product-analytics signals to PostHog/Sentry from `PersistentPreRun`/`PostRun`. This Feature emits structured lifecycle events to user-configured subscribers from an explicit verb invocation. The two paths share no envelope, no transport, and no config. |

## Not Doing / Out of Scope

- **The `specscore event emit` verb itself.** Owned by the child Feature `cli/event/emit`.
- **HTTP webhook subscribers.** Deferred per the source Idea's Not Doing list. Exec subscribers fronting `curl` cover the case at one-tenth the v1 design cost.
- **Go plugin / wasm subscribers.** Deferred per the source Idea.
- **Background daemon dispatch.** Replay is command-driven; no resident SpecScore delivery daemon is introduced.
- **Per-subscriber event filtering / topic routing.** Every subscriber receives every event in v1. Subscribers filter in-process if they care.
- **Automated timed backoff.** Failed subscriber delivery remains pending for explicit replay; no scheduler is introduced.
- **Per-event payload schema validation.** Envelope-only validation in v1. Per-event schemas couple CLI releases to skill releases; tracked as an Open Question for a future additive Feature.
- **Replay / seek API over `.specscore/events.jsonl`.** Durable replay is based on the immutable outbox ledger and per-subscriber acknowledgements, not scanning the compatibility JSONL sink.
- **A `specscore event log` pretty-printer.** Deliberately omitted; `jq . .specscore/events.jsonl` covers the read path.
- **Migration of any event-emitting skill other than `verify`.** Tracked as follow-up work in the SDD skills repository.

## Assumption Carryover

| Idea assumption | Status after this spec |
|---|---|
| The six event-emitting skills can be ported with no behavioral regression. | Inherited by `cli/event/emit` (the `verify` proof-point port lives there). This parent owns the underlying dispatch machinery; "no regression" is verified by the byte-level equality of the JSONL line written via the new path. |
| A single `events:` block can express compiled-in and Exec subscribers in a schema small enough to fit on one page of docs. | **Encoded** as REQ:events-config-schema. Schema is locked here. |
| Synchronous fan-out keeps emission overhead bounded. | **Partially encoded.** REQ:fan-out-dispatch (synchronous) and REQ:exec-subscriber-timeout (2 s default cap) lock the worst-case behavior, verified by the `exec-timeout-kills-hung-process` AC. The source Idea's typical-case p50/p99 microbenchmark validation (100 back-to-back emissions per config, target p99 < 50 ms for JSONL-only / < 200 ms with one Exec subscriber) is deferred to a perf-followup outside this MVP. |
| The Exec contract (stdin JSON, non-zero = failure) is sufficient for arbitrary downstream consumers. | **Encoded** as REQ:exec-subscriber. |
| Resolving JSONL path against project root (not cwd) is correct for monorepos. | **Encoded** as REQ:jsonl-writer-subscriber's project-root resolution clause. Verified by the `jsonl-writer-resolves-against-project-root` AC. |
| Incremental skill migration without flag-day. | Inherited by `cli/event/emit`. |
| One Exec subscriber per consumer is the right granularity. | Carried forward unchanged; revisit only when a second Exec consumer surfaces. |
| The `events:` block belongs in `specscore.yaml` rather than a separate file. | **Encoded** as REQ:events-config-schema. |

## Acceptance Criteria

### AC: subscriber-interface-shape

**Requirements:** cli/event#req:pkg-event-package-location, cli/event#req:subscriber-interface

**Given** a fresh checkout of the repository after this Feature is implemented
**When** `go list ./pkg/event` and `go vet ./pkg/event/...` run from the repo root
**Then** both commands MUST exit `0`; the package path MUST be exactly `github.com/specscore/specscore-cli/pkg/event` (singular `event`); the exported `Subscriber` interface MUST declare exactly the two methods `Deliver(ctx context.Context, e Event) error` and `Name() string`; the exported `Event` struct MUST declare fields `Name`, `Version`, `UUID`, `Timestamp`, `Actor`, `Artifact`, `Payload` with `Payload` typed as `json.RawMessage`.

### AC: jsonl-writer-appends-line

**Requirements:** cli/event#req:jsonl-writer-subscriber

**Given** a `JsonlWriter` constructed with `path = .specscore/events.jsonl` in an otherwise-empty project root, and a valid `Event` value
**When** `Deliver(ctx, event)` is called twice in sequence
**Then** the file `.specscore/events.jsonl` MUST exist with mode `0644`; its content MUST be exactly two lines, each a valid single-line JSON document of the envelope, separated by `\n`; no other bytes MUST appear in the file. The parent directory `.specscore/` MUST exist with mode `0755` (created by the subscriber if absent).

### AC: jsonl-writer-resolves-against-project-root

**Requirements:** cli/event#req:jsonl-writer-subscriber

**Given** a project root containing `specscore.yaml` at `/tmp/proj/specscore.yaml` and cwd set to `/tmp/proj/sub/dir/`
**When** a `JsonlWriter` configured with the relative path `.specscore/events.jsonl` calls `Deliver`
**Then** the line MUST be appended to `/tmp/proj/.specscore/events.jsonl` (project-root-relative), NOT to `/tmp/proj/sub/dir/.specscore/events.jsonl` (cwd-relative).

### AC: noop-discards

**Requirements:** cli/event#req:noop-subscriber

**Given** a `NoOp` subscriber and any valid `Event`
**When** `Deliver(ctx, event)` is called
**Then** the call MUST return nil; `Name()` MUST return the literal string `noop`; no filesystem, network, stdout, or stderr side effect MUST be observable.

### AC: exec-pipes-event-to-stdin

**Requirements:** cli/event#req:exec-subscriber

**Given** an `Exec` subscriber configured with `command: [tee, /tmp/recorded-event.jsonl]` and a valid `Event`
**When** `Deliver(ctx, event)` is called
**Then** the call MUST return nil; `/tmp/recorded-event.jsonl` MUST contain exactly one line that is byte-identical to the JSON serialization of the event (modulo a trailing `\n`); the child process MUST have received EOF on stdin (`tee` exits cleanly only on EOF).

### AC: exec-timeout-kills-hung-process

**Requirements:** cli/event#req:exec-subscriber, cli/event#req:exec-subscriber-timeout

**Given** on Unix an `Exec` subscriber configured with a shell command that starts a `sleep 30` descendant which ignores SIGTERM and `timeout_ms: 200`; and on Windows an equivalent PowerShell subscriber that starts a 30-second descendant and records a retained native handle to it; and a valid `Event`
**When** each platform's `Deliver(ctx, event)` is called and the wall clock is measured
**Then** the call MUST return a timeout error (distinguishable from an exit-code error); on Unix the wall-clock duration MUST be in `[200, 400]` ms (200 ms timeout + ≤100 ms SIGTERM grace + scheduler slack) and neither the shell nor its TERM-ignoring `sleep` descendant MUST remain running, proving the SIGKILL escalation is effective; on Windows `TerminateJobObject` MUST complete within 2 seconds of the configured timeout and the retained descendant handle MUST report a non-active exit status.

### AC: events-config-default-when-absent

**Requirements:** cli/event#req:default-and-empty-config

**Given** a `specscore.yaml` containing only `project: { title: "Test" }` (no `events:` mapping)
**When** the config loader runs against that file from the project root
**Then** the returned subscriber list MUST contain exactly one entry, a `JsonlWriter` whose configured path resolves to `<project-root>/.specscore/events.jsonl`.

### AC: events-config-explicit-empty-is-noop

**Requirements:** cli/event#req:default-and-empty-config, cli/event#req:dispatch-exit-codes

**Given** a `specscore.yaml` containing `events: { subscribers: [] }` and a valid envelope passed through the dispatcher
**When** the dispatch runs end-to-end (config load + validate + dispatch)
**Then** the returned subscriber list MUST be empty (no default fallback); the dispatcher MUST NOT call any subscriber; exit code MUST be `0`; no stderr output MUST be produced.

### AC: events-config-unknown-type-rejected

**Requirements:** cli/event#req:events-config-schema, cli/event#req:dispatch-exit-codes

**Given** a `specscore.yaml` containing `events: { subscribers: [{ type: webhook, url: "https://example.com" }] }`
**When** the dispatch verb runs
**Then** exit code MUST be `2`; stderr MUST contain a single line naming the file (`specscore.yaml`), the key path (`events.subscribers[0].type`), the offending value (`webhook`), and the accepted enum (`jsonl`, `noop`, `exec`); no subscriber MUST be called.

### AC: envelope-validation-rejects-bad-name

**Requirements:** cli/event#req:envelope-validation, cli/event#req:dispatch-exit-codes

**Given** an envelope where every field is valid except `name = "Idea.Drafted"` (uppercase, period in the wrong place would also fail; uppercase alone is sufficient)
**When** the dispatcher validates the envelope
**Then** validation MUST fail; exit code MUST be `2`; stderr MUST name the field (`name`) and the pattern rule (`^[a-z][a-z0-9-]*\.[a-z][a-z0-9-]*$`); no subscriber MUST be called.

### AC: envelope-validation-rejects-bad-actor-kind

**Requirements:** cli/event#req:envelope-validation

**Given** an envelope where every field is valid except `actor.kind = "robot"`
**When** the dispatcher validates the envelope
**Then** validation MUST fail; exit code MUST be `2`; stderr MUST name the field (`actor.kind`), the offending value (`robot`), and the accepted enum (`skill`, `user`, `external`); no subscriber MUST be called.

### AC: payload-is-opaque-passthrough

**Requirements:** cli/event#req:envelope-validation

**Given** an envelope with a payload containing fields the CLI has never heard of: `{ "made_up_field_1": [1,2,3], "nested": { "anything": null }, "unicode": "héllo 🎉" }`
**When** the dispatcher dispatches the envelope to a recording subscriber
**Then** the subscriber MUST receive the payload byte-identical to the input (after JSON round-trip canonicalization — same field names, same values, same nesting); the dispatcher MUST NOT mutate, reject, or warn about any payload field; exit code MUST be `0`.

### AC: fan-out-continues-after-failure

**Requirements:** cli/event#req:fan-out-dispatch

**Given** a subscriber list `[failing_subscriber, recording_subscriber]` where `failing_subscriber.Deliver` always returns an error
**When** the dispatcher dispatches a valid envelope
**Then** `failing_subscriber.Deliver` MUST be called exactly once and `recording_subscriber.Deliver` MUST be called exactly once (in that order); `recording_subscriber` MUST have received the envelope; exit code MUST be `0` (one subscriber succeeded).

### AC: per-subscriber-failure-stderr-format

**Requirements:** cli/event#req:fan-out-dispatch

**Given** a subscriber whose `Name()` returns `exec:my-consumer` and whose `Deliver` returns the error `exit status 1`, dispatched against an envelope with `name = "idea.drafted"`
**When** the dispatcher invokes the subscriber
**Then** stderr MUST contain exactly one line matching the pattern: `event-dispatch failure: subscriber=exec:my-consumer event=idea.drafted error="exit status 1"`. A successful subscriber MUST NOT produce stderr.

### AC: dispatch-exit-code-when-all-fail

**Requirements:** cli/event#req:dispatch-exit-codes, cli/event#req:fan-out-dispatch

**Given** a non-empty subscriber list where every subscriber's `Deliver` returns a non-nil error
**When** the dispatch verb runs with a valid envelope
**Then** every subscriber MUST be invoked once; stderr MUST contain one failure line per subscriber per REQ:fan-out-dispatch; exit code MUST be `10`.

### AC: subscriber-success-never-masks-another-pending-sink

**Requirements:** cli/event#req:immutable-prepared-ledger, cli/event#req:reconstructible-pending-delivery

**Given** one subscriber succeeds and a second subscriber fails for the same committed event
**When** delivery finishes and `event replay --subscriber <failed>` runs
**Then** the first subscriber remains acknowledged, the second remains independently pending and is retried with the same event UUID, and no successful sink acknowledges another sink's work.

### AC: interrupted-prepared-event-is-explicit

**Requirements:** cli/event#req:prepared-reconciliation

**Given** a process stops after publishing a prepared record and artifact evidence but before committing the event
**When** `event replay` and `event reconcile` inspect the outbox
**Then** replay reports the unresolved prepared record without delivering it, and only a confirmed commit or abort decision transitions it.

### AC: corrupt-ledger-never-reaches-a-subscriber

**Requirements:** cli/event#req:immutable-prepared-ledger

**Given** a local ledger record with an invalid envelope, unknown/trailing JSON, or a noncanonical subscriber set
**When** replay or recovery reads it
**Then** validation fails before any subscriber is called; cryptographic detection of an otherwise schema-valid rewrite remains Planned.

### AC: docs-events-md-skeleton-present

**Requirements:** cli/event#req:docs-events-md-skeleton

**Given** a fresh checkout after this Feature is implemented
**When** the file `docs/events.md` is opened
**Then** the file MUST contain the following second-level headings in order: `## Overview`, `## The events: config block`, `## Built-in subscribers`, `## Writing an Exec subscriber`, `## Envelope shape`, `## Default behavior`, `## Disabling events`. The `## Built-in subscribers` section MUST contain third-level subsections `### jsonl`, `### noop`, and `### exec`. The `## Envelope shape` section MUST enumerate each field in REQ:envelope-validation's rule set.

## Open Questions

- **Per-event payload schema validation.** Payload is opaque in v1. A future additive Feature MAY add per-event-name schema validation; design and tradeoffs deferred until a concrete consumer asks for it.
- **Per-subscriber stderr format for non-TTY callers.** REQ:fan-out-dispatch fixes a key=value single-line pattern that works for human and grep consumers. Whether to switch to JSON when stderr is non-TTY can be revisited after a real consumer surfaces a preference.

---
*This document follows the https://specscore.md/feature-specification*
