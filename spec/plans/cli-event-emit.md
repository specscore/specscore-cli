---
format: https://specscore.md/plan-specification
status: Implemented
---

# Plan: Event Emit Verb

**Status:** Implemented
**Mode:** full
**Source Feature:** cli/event/emit
**Date:** 2026-05-22
**Owner:** alexandertrakhimenok
**Supersedes:** —

## Summary

Implementation plan for the child `cli/event/emit` Feature — the `specscore event emit` cobra verb. Lands cobra wiring, the seven envelope flags with required-flag validation, three payload input modes (`--payload-json` / `--payload-file` / stdin) with arbitration, envelope auto-fill (version, uuid, timestamp, artifact.revision), and dispatch handoff to `pkg/event` (built by the sibling Plan `cli-event.md`). Six tasks covering all 12 ACs in the source Feature, zero deferred.

## Approach

Verb-surface decomposition, ordered foundation → flags → payload → handoff. The parent's `pkg/event` library is a hard prerequisite for every task here — task 6's dispatch handoff in particular is unreachable until `pkg/event.Dispatch` exists. This Plan assumes the sibling Plan `cli-event.md` has been or is being implemented in parallel; the verb cannot ship before its library does. Within this Plan, the linear order is: cobra plumbing first (task 1) so the verb is registered before any flag is wired; envelope flags before envelope auto-fill (tasks 2 and 3) because auto-fill operates on the already-parsed flag state; payload input modes before payload arbitration (tasks 4 and 5) because the happy-path reader is a precondition for the conflict-detection logic that wraps it; dispatch handoff last (task 6) because it composes everything above.

Task 5 carries a notable test-scaffolding cost: the `payload-tty-stdin-fails-2` AC requires a PTY harness (e.g. `creack/pty`) to simulate a TTY stdin in tests. This is sized in the task description so implementation effort isn't surprised by it.

No ACs are deferred. All 12 ACs in `cli/event/emit` are covered.

## Tasks

### Task 1: Register `event` cobra parent and `event emit` subcommand with `--help`

**Status:** complete
**Depends-On:** —
**Verifies:** cli/event/emit#ac:verb-registers-and-helps

Add `internal/cli/event.go` registering the `event` cobra parent (prints help on bare invocation, exits `0`) and the `emit` subcommand stub. Add the canonical docs link `https://specscore.md/event-emit` to the `emit --help` output. The verb's `RunE` is a stub returning success in this task — actual flag wiring lands in later tasks. Isolating the cobra plumbing from the verb's logic lets the registration AC verify in isolation and lets a regression in flag wiring not break the basic discoverability check.

### Task 2: Implement envelope flag set with required-flag validation

**Status:** complete
**Depends-On:** 1
**Verifies:** cli/event/emit#ac:required-flag-missing-fails-2

Add the seven envelope flags to the `emit` subcommand: `--name`, `--actor-kind`, `--actor-id`, `--artifact-type`, `--artifact-id`, `--artifact-path` (required); `--artifact-revision` (optional). Wire cobra's required-flag enforcement so a missing required flag exits `2` with a stderr line naming the flag and the envelope field it supplies. Reject extra positional arguments — the verb is flag-form only to keep the call shape stable across shells. No payload reading yet; no envelope construction yet (auto-fill is task 3).

### Task 3: Implement envelope auto-fill (version, uuid, timestamp, artifact.revision)

**Status:** complete
**Depends-On:** 2
**Verifies:** cli/event/emit#ac:envelope-auto-fill-fields, cli/event/emit#ac:envelope-auto-fill-revision-no-git, cli/event/emit#ac:envelope-artifact-revision-override

In the verb's `RunE`, after flag parsing, populate the envelope bookkeeping fields: `version = 1`; `uuid` = fresh v4 (lowercase, hyphenated, no surrounding whitespace); `timestamp` = `time.Now().UTC()` formatted RFC 3339 with the `Z` suffix; `artifact.revision` = output of `git rev-parse HEAD` invoked in the project root. Resolve the project root via the existing `pkg/projectdef`; invoke git via `pkg/gitremote` (or extend it with a thin `HeadSHA(dir)` helper if it doesn't already expose one — the source Feature's Architecture table calls this out as "`pkg/gitremote` or equivalent"). On `git rev-parse HEAD` failure (no `.git/`, no commits yet, other git error), fill the literal string `"uncommitted"` rather than failing the verb. The optional `--artifact-revision` flag, when present, wins over the auto-fill — the user-supplied value is not overridden by the git call.

### Task 4: Implement payload input modes — read bytes from `--payload-json` / `--payload-file` / stdin

**Status:** complete
**Depends-On:** 2
**Verifies:** cli/event/emit#ac:payload-json-flag-shape, cli/event/emit#ac:payload-file-flag-shape, cli/event/emit#ac:payload-stdin-shape

Add `--payload-json '<json>'` and `--payload-file <path>` flags. Implement payload-byte resolution: when `--payload-json` is set, the flag value IS the payload bytes; when `--payload-file` is set, read the file's bytes (resolve relative paths against the project root); otherwise read all bytes from stdin until EOF. Pass the bytes through to the envelope's `Payload` field as `json.RawMessage` — no payload mutation, no field-level inspection. This task covers the three happy-path ACs only; mode arbitration and parse-error paths land in task 5.

### Task 5: Implement payload mode arbitration + JSON parse pre-check (exit-2 paths)

**Status:** complete
**Depends-On:** 4
**Verifies:** cli/event/emit#ac:payload-mode-conflict-fails-2, cli/event/emit#ac:payload-tty-stdin-fails-2, cli/event/emit#ac:payload-bad-json-fails-2

Layer the exit-`2` paths over task 4's reader: reject when both `--payload-json` and `--payload-file` are set (stderr names the conflict and the three accepted modes); reject when either flag is set AND stdin is a non-TTY pipe with bytes — detect via `os.Stat(os.Stdin)` for a `ModeNamedPipe` or character-device check rather than byte-peeking stdin (peeking consumes); reject when no payload flag is set AND stdin is a TTY (exit `2` within 1 second; the verb MUST NOT block on keyboard input that the caller almost certainly did not intend); validate the resolved payload bytes parse as a JSON object before handoff, and on parse failure exit `2` with stderr naming the input mode (`--payload-json` / `--payload-file <path>` / `stdin`) and the JSON parse error. **Test-scaffolding note:** the `payload-tty-stdin-fails-2` AC needs a PTY harness in tests (consider `creack/pty` or equivalent); call this out at implementation time so the test-fixture cost is sized correctly.

### Task 6: Implement dispatch handoff + exit code adoption

**Status:** complete
**Depends-On:** 3, 4, 5
**Verifies:** cli/event/emit#ac:dispatch-exit-code-handoff

Wire the verb's `RunE` to invoke `pkg/event.Dispatch(ctx, event, subscribers)` with the constructed envelope and the config-loaded subscriber list (config loading is provided by the parent Plan's `events:` config-loader work — the verb consumes it, doesn't reimplement it). Adopt the parent Feature's exit-code contract verbatim — 0, 2, 3, 10 only; the verb MUST NOT introduce new exit codes. Cover the all-subscribers-fail end-to-end path so exit `10` is verified through the full verb → dispatcher path, not only at the dispatcher unit-test level.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
