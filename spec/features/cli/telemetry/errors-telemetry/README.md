---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: Errors Telemetry (CLI)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/telemetry/errors-telemetry?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/telemetry/errors-telemetry?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/telemetry/errors-telemetry?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/telemetry/errors-telemetry?op=request-change) |
**Status:** Stable
**Date:** 2026-05-21
**Owner:** alexandertrakhimenok
**Source Ideas:** cli-error-telemetry
**Supersedes:** —

## Summary

Implements the `crash-reports` channel registered against the parent [`cli/telemetry`](../README.md) Feature. Sends one anonymous error event to Sentry (cloud, EU region) when the CLI panics or exits with code ≥10 (unexpected error path), so the founder can fix-and-ship within hours of a user hitting a crash. Documented errors with exit codes 1–9 are never reported. Includes a `specscore debug error --text "<msg>" [--force]` verification subcommand that synthesises a test event through the live pipeline.

## Problem

Crash signal from the first soft-post cohort (PLAN-90D W4–W5) is the highest-leverage debugging input of Q2. At early-adopter scale (10–30 users), losing a single user to a silent crash is a catastrophic retention hit — and without crash telemetry we don't even know to act. The parent Feature locked the shared plumbing (package boundary, opt-out, first-run notice, install_id, channel registry, 500ms transmission timeout). What remained was the crash-reporting channel itself: how panics and unexpected errors become Sentry events without exfiltrating user-authored content, how the verification path works without flipping persistent opt-out, and how the audit surface for stack-frame scrubbing stays unified with the product-analytics scrubber.

This Feature is small on purpose. Like its sibling [`usage-telemetry`](../usage-telemetry/README.md), every architectural decision that could live in the parent already does. The child encodes only what is unique to crash reporting: Sentry as the destination, the panic-recovery hook and exit-code-≥10 trigger, the allowlist-based panic-message gating, and the debug subcommand.

## Behavior

### Sentry client

The `crash-reports` channel transmits to Sentry cloud, EU region, via the Sentry Go SDK confined to `internal/telemetry/errors.go` per the parent's vendor-SDK import confinement REQ.

#### REQ: sentry-go-sdk

`internal/telemetry/errors.go` MUST import `github.com/getsentry/sentry-go` and instantiate the client at package init time inside the boundary the parent established. The SDK's async transport MUST be used; flush is bounded by the parent's REQ:transmission-hard-timeout (500 ms).

#### REQ: sentry-eu-region

The Sentry client MUST be configured to use a project in Sentry's EU region (`https://sentry.io/welcome/eu/` data residency, DSN form `https://<key>@<org>.ingest.de.sentry.io/<project-id>`). The US region MUST NOT be used. Region is encoded by the DSN value compiled in at build time (REQ:sentry-dsn-embedded-at-build-time).

#### REQ: sentry-dsn-embedded-at-build-time

The Sentry DSN MUST be compiled into the binary via Go's `-ldflags "-X internal/telemetry.sentryDSN=..."` during release builds. The variable is package-scoped but its declaration MUST live in `internal/telemetry/errors.go` so the vendor-SDK import-confinement audit surface stays intact. The DSN MUST NOT be read from an environment variable at runtime. When the embedded value is the empty string (i.e. a developer build with no DSN), the channel MUST register with the parent but the transmit callback MUST silently no-op; no Sentry HTTP requests MUST be issued.

#### REQ: sentry-release-tag

Every event MUST include a `release` tag equal to the CLI's `cli_version` (matching the parent's REQ:fixed-event-property-keys `cli_version` source — embedded via goreleaser). This lets Sentry correlate crash signatures to specific releases and the "regression in 0.2.1" common case is observable.

### Channel registration

The `crash-reports` channel registers with the parent's registry exactly once.

#### REQ: crash-reports-channel-registration

`internal/telemetry/errors.go` MUST contain a Go `init()` function that calls `telemetry.RegisterChannel("crash-reports", transmit)` where `transmit` is the function that sends one error event to Sentry. The registered name MUST be the exact string `crash-reports` — matching the parent's REQ:channel-registry enumeration. The registration MUST be the only call site for `RegisterChannel("crash-reports", ...)` in the entire repo.

### Trigger conditions

Not every error is a crash report. The channel transmits only on truly unexpected paths.

#### REQ: trigger-on-panic-recovery

The parent's PersistentPostRun MUST invoke this channel's transmit callback when a panic was recovered during the command's execution. Recovery happens at the cobra-root level (a `defer recover()` in `internal/cli/root.go`'s PersistentPreRun-installed handler); the panic value plus the stack trace are captured and handed to `internal/telemetry/errors.go` for scrubbing and transmission.

#### REQ: trigger-on-exit-code-ge-10

The channel's transmit callback MUST be invoked when a command exits with code ≥10 AND no panic was recovered (panics take priority — a panicking command also has a non-zero exit, but it is transmitted as a panic, not as an exit-code error). Exit codes 1–9 are documented errors (per the CLI's shared exit-code contract) and MUST NOT trigger transmission.

#### REQ: exit-code-audit-precondition

Before v0.2.0 ships, every existing CLI call site that returns an exit code ≥10 MUST be audited: if it represents an *expected* condition (e.g. a documented user-facing error), it MUST be renumbered into 1–9 first. The audit is a Plan-time deliverable and MUST be recorded in the Plan's task list with a concrete file/line list of audited sites.

### Stack-frame and message scrubbing

The privacy invariant from the parent ("no user-authored text leaves this binary") applies. The scrubber is the implementation.

#### REQ: stack-frame-scrubber

Before transmission, every stack frame MUST be passed through a scrubber that:

1. Replaces the file path with its basename only (e.g. `/Users/foo/projects/specscore/internal/feature/feature.go:142` → `feature.go:142`).
2. Preserves the function name and line number verbatim.
3. Strips local-variable values from frame metadata (the Sentry SDK MAY attach these by default — disable explicitly).

The scrubber MUST live at `internal/telemetry/scrubber.go` so the privacy audit surface is unified and future channels (or shared usage-telemetry scrubbing needs) attach to one file. Scrubber code MUST NOT be inlined in `errors.go` or `usage.go`.

#### REQ: panic-message-safe-allowlist

The panic message itself is risky — `panic(fmt.Errorf("failed to load spec %s: %w", userInput, err))` includes user-authored content. The channel MUST gate panic-message transmission via a `telemetry.SafePanic(messageID string, err error)` wrapper:

- Code wishing to panic with a transmittable message uses `panic(telemetry.SafePanic("spec-load-failed", err))`. The wrapper returns a value carrying a stable `messageID` (an opaque identifier — NOT formatted prose) plus the unwrapped `err` chain.
- The transmit callback inspects the recovered panic value. If it carries a `SafePanic` payload, the `messageID` MUST be sent verbatim as the Sentry event's `message` field; the original Go `err` chain MUST be discarded (the wrapped err may contain user content).
- If the recovered panic value is anything else (a plain string, an unwrapped error, a runtime panic, an unscrubbed `fmt.Errorf` result), the Sentry event's `message` field MUST be set to the literal string `"unscrubbed panic"` and the event MUST carry a tag `message: unscrubbed` so the operator can grep for these and audit.

The allowlist of valid `messageID` values is enumerated in `internal/telemetry/scrubber.go` (Go constants). New IDs require a code change. Initial allowlist for v0.2.0 is populated by the Plan's panic-site-audit task; the allowlist MUST also contain a single test-only entry `test-known-id` whose declaration MUST be guarded by a `_test.go` file (Go test-only compilation) so it never appears in release binaries. The test-only entry exists exclusively to satisfy AC:safe-panic-allowlisted-message-transmitted-verbatim without coupling that AC to whichever production IDs the audit happens to produce.

#### REQ: transmit-callback-must-not-mask-exit-code

The transmit callback in `internal/telemetry/errors.go` MUST install its own `defer recover()` around the scrubber + Sentry-emit work. A panic originating *inside* the transmit callback (e.g. a scrubber bug, a malformed event payload, an SDK panic) MUST be caught silently and MUST NOT propagate to the user's command. The user's original exit code MUST be preserved byte-identical whether or not the transmit callback panicked. At `--verbose`, a one-line stderr message MAY indicate the telemetry path failed; absent `--verbose`, the user MUST observe no difference.

#### REQ: scrubber-fuzz-tests

The scrubber MUST have fuzz tests in `internal/telemetry/scrubber_fuzz_test.go` that feed adversarial inputs — paths containing `/Users/.../`, project paths containing the literal string `secret`, frames whose function names contain `(my-spec.md)`, panic messages containing email-like strings, multi-byte UTF-8 noise — and assert the scrubber's output contains no characters that would indicate leakage. Fuzz tests MUST run in CI (not just locally); a fuzz failure MUST fail the build.

### Verification subcommand

A single, deliberate way to exercise the live pipeline end-to-end without crashing.

#### REQ: debug-error-subcommand

The CLI MUST provide a `specscore debug error --text "<msg>" [--force]` subcommand that synthesises a test error event with the supplied `<msg>` text and emits it through the live Sentry pipeline. Behavior:

- The synthesised event MUST be tagged `debug: true` so Sentry alert rules can suppress it from on-call noise.
- The `<msg>` value MUST be interpreted as a candidate `messageID` (i.e. tested against the closed-enum allowlist per REQ:panic-message-safe-allowlist), NOT as free-form prose. If `<msg>` matches an allowlisted `messageID`, it is sent verbatim. If it does not, the transmit MUST replace it with `"unscrubbed panic"` and apply the `message: unscrubbed` tag — the same coercion path as a real unscrubbed panic. This means `--text` is a diagnostic surface for exercising specific allowlist entries, not for injecting arbitrary text into Sentry; the privacy invariant holds even under operator-supplied input.
- Without `--force`: the command MUST honor the user's opt-out signal for `crash-reports`. If opt-out is in effect, the command MUST exit `0` after printing a no-op message naming `specscore telemetry enable crash-reports` (per the parent's REQ:telemetry-subcommand-surface).
- With `--force`: the command MUST bypass the opt-out for this single invocation, transmit the event regardless of persistent state, and print to stdout a precise description of what was sent (event message, tags, frames-or-not, target DSN). `--force` MUST NOT modify the persistent opt-out state.
- The subcommand MUST be hidden from `specscore --help` (cobra `Hidden: true`) but documented in `docs/telemetry.md` as the diagnostic entry point.

#### REQ: debug-error-ci-usage

The release pipeline (goreleaser + CI workflows) MAY invoke `specscore debug error --text "ci-smoke-test"` without `--force` as a post-deploy smoke test. Because no `--force` flag is passed AND the CI environment auto-disables telemetry per the parent's REQ:opt-out-signal-precedence step 3, this exercises the **opt-out path** end-to-end — confirming the channel registers, the subcommand parses, and the no-op message renders correctly. The smoke test MUST exit `0` and MUST NOT issue any Sentry HTTP requests.

### Operator alerting

Crash signal is useless if the founder doesn't see it.

#### REQ: sentry-alert-on-new-signature

The Sentry project for `crash-reports` MUST be configured with one alert rule: any new crash signature (Sentry's term: "issue") in the latest release tag MUST notify the founder. Notification channel is Sentry's built-in email-to-founder for v0.2.0; Slack DM or webhook MAY be substituted later. Like the usage-telemetry funnel, this is an operational artifact — the spec documents the contract; the configuration happens once in the Sentry UI. The alert MUST filter on `debug: true` to suppress `specscore debug error` invocations.

### Documentation contribution

This Feature populates the parent-owned `docs/telemetry.md` skeleton with its channel-specific content.

#### REQ: docs-crash-reports-section

`docs/telemetry.md` MUST contain, inside the parent's `## Channels` section, a `### crash-reports` subsection authored by this Feature with at least: a one-paragraph description of what the channel is for; an explicit statement that only panics and exit codes ≥10 trigger transmission; a link to Sentry's privacy / data-handling page; and a callout that the EU region is used. The `## Event Schema` section MUST contain a `### crash-reports events` subsection enumerating the Sentry event shape (event name, message handling, frame format after scrubbing, the `release` and `debug` tags). The `## What We Don't Collect` section (parent-owned but shared) MUST already cover the invariants — this Feature's contribution is an explicit reference from the `### crash-reports` subsection back to that section. The `## Data Retention` section MUST contain a `### crash-reports` subsection stating Sentry's retention policy for the free tier (currently 30 days for events, 90 days for issues) and how to request deletion.

## Architecture & Components

| Unit | Responsibility | Used by | Depends on |
|---|---|---|---|
| `internal/telemetry/errors.go` | The only file in the repo importing `github.com/getsentry/sentry-go`. Contains the `init()` registration, Sentry client instantiation, the transmit callback that converts panic-recovery context or exit-code-≥10 context into a Sentry event, the DSN-empty no-op path, and the release-tag application. | Parent's PersistentPostRun (panic recovery + exit-code dispatch); the `specscore debug error` subcommand. | `internal/telemetry` (parent); `github.com/getsentry/sentry-go`. |
| `internal/telemetry/scrubber.go` | Stack-frame scrubber (basename, no locals), panic-message gate (SafePanic allowlist), and the `SafePanic(messageID, err)` constructor + payload type. Owns the closed-enum allowlist of `messageID` values. Must be a separate file (per REQ:stack-frame-scrubber) — not inlined in `errors.go`. | The transmit callback in `errors.go`; every panic-site that wants its message transmitted. | None — pure Go. |
| `internal/telemetry/scrubber_fuzz_test.go` | Fuzz tests asserting the scrubber's leak-free invariant. | CI. | The scrubber. |
| `internal/cli/root.go` (modified) | `defer recover()` installed in PersistentPreRun captures panics; on recovery, the panic value plus stack are handed to the transmit callback in PostRun. PostRun also dispatches exit-code-≥10 events. | Every `specscore` invocation. | cobra; `internal/telemetry`. |
| `internal/cli/debug.go` (new) | The `specscore debug` cobra command group plus `specscore debug error` subcommand. Wires the `--text` and `--force` flags; honors opt-out by default; calls `internal/telemetry/errors.go`'s transmit callback directly with synthesised input. | Operators (manually); CI (smoke test, no `--force`). | `internal/telemetry`. |
| Build pipeline (`.goreleaser.yml`, modified further) | Injects the Sentry DSN via `-ldflags "-X internal/telemetry.sentryDSN=<dsn>"` for release builds. Dev builds get the empty string and silently no-op. | Release process. | The Sentry project DSN (stored in the GitHub Actions secrets for the release workflow). |
| Sentry project (operational) | The destination. Configured once in the Sentry UI: project named `specscore-cli` in the EU region, single alert rule on new crash signatures in the latest release tag (filtered to exclude `debug: true`). | n/a (operational artifact). | n/a. |
| `docs/telemetry.md` (`### crash-reports` subsections) | Per-channel content authored by this Feature into the parent's skeleton. | Users. | Parent's REQ:docs-telemetry-md-skeleton. |

## Data Flow

```
specscore <cmd> [args]
  │
  ├─→ parent PersistentPreRun
  │     (opt-out resolution, install_id, first-run notice)
  │     + INSTALLS `defer recover()` for the rest of the invocation
  │
  ├─→ <cmd> Run()
  │     either:
  │       (a) exits normally with code 0 ⇒ no crash-reports event
  │       (b) exits with code 1–9 (documented error) ⇒ no crash-reports event
  │       (c) exits with code ≥10 (unexpected error) ⇒ crash-reports trigger
  │       (d) panics ⇒ recovered by the deferred handler ⇒ crash-reports trigger
  │
  └─→ parent PersistentPostRun
        1. If crash-reports channel disabled by opt-out: skip
        2. Determine trigger: panic recovered (priority) OR exit ≥10
        3. Build the Sentry event:
             - If panic: extract recovered value, scrub stack frames, gate message
               (SafePanic allowlist OR "unscrubbed panic")
             - If exit ≥10: synthesise a message (e.g. "unexpected exit code 12 from cmd X"),
               scrub the call-site frame
        4. Apply tags: release=<cli_version>, debug=false
        5. Pass to internal/telemetry/errors.go's registered transmit-fn
        6. transmit-fn calls sentry.CaptureEvent (async, batched)
        7. Parent flushes (bounded by 500ms)

specscore debug error --text "<msg>" [--force]
  │
  └─→ debug.go Run()
        1. If --force NOT set AND crash-reports opt-out in effect:
             - Print no-op message naming `specscore telemetry enable crash-reports`
             - Exit 0
        2. Otherwise:
             - Build a synthesised Sentry event with the supplied <msg>
             - Apply tags: release=<cli_version>, debug=TRUE
             - --force prints to stdout exactly what is about to be sent
             - Call transmit-fn directly (NOT through PostRun)
             - Parent flush (500ms timeout)
             - Exit 0
```

## Error Handling & Failure Modes

| Failure | Behavior |
|---|---|
| Sentry endpoint unreachable / 5xx / DNS | Caught by parent's REQ:transmission-hard-timeout. Event dropped silently. |
| Sentry SDK initialisation error (e.g. malformed DSN) | Logged to stderr at `--verbose` only. The channel registers but the transmit-fn no-ops. User's command proceeds unaffected. |
| `sentryDSN` is the empty string (dev build) | Per REQ:sentry-dsn-embedded-at-build-time: register the channel, no-op the transmit. No HTTP request issued. |
| Panic during PersistentPostRun's transmit step itself (e.g. scrubber bug) | Recovered by an inner deferred recover() inside the transmit-fn. Event dropped; logged to stderr at `--verbose`. The user's original command's exit code MUST NOT be masked by a telemetry panic. |
| `SafePanic` allowlist contains a stale `messageID` (the panic site was renamed/deleted but the constant was not removed) | The constant is dead code; allowlist membership check still passes for any new use, no behavioral effect. Cleanup is a Plan-time hygiene item. |
| `specscore debug error` called without `--text` flag | cobra rejects at parse time; exit `2`. |
| `specscore debug error --force --text ""` (empty text with force) | Treated as a synthesised event with the literal message `"unscrubbed panic"` (per REQ:debug-error-subcommand, anything outside the SafePanic allowlist coerces). Useful as a regression-test for the unscrubbed path itself. |
| Exit code ≥10 occurs in a command that ALSO panicked | Panic takes priority (REQ:trigger-on-exit-code-ge-10). Single Sentry event sent representing the panic. The exit-code-≥10 path is not separately triggered. |

## Testing Strategy

Per-AC Rehearse stubs MAY be scaffolded for the testable ACs. The Sentry alert-rule REQ is an operational check, not a runtime test — recorded as a skip with rationale. The fuzz tests for the scrubber live as Go fuzz targets, not Rehearse scenarios.

## Rehearse Integration

| AC | Stub? | Rationale |
|---|---|---|
| `sentry-client-eu-region` | yes | Network-mocked: capture the DSN, assert host matches the EU regional ingest endpoint |
| `sentry-dsn-empty-no-op` | yes | Build with empty `-ldflags`, force-panic, assert no HTTP requests issued |
| `crash-reports-channel-registered` | yes | Query the parent's channel registry, assert `crash-reports` is present |
| `panic-triggers-event` | yes | Network-mocked: force a panic via a test-only command, assert exactly one Sentry event captured |
| `exit-code-ge-10-triggers-event` | yes | Network-mocked: invoke a test-only command that exits 11, assert one Sentry event |
| `exit-code-1-to-9-does-not-trigger` | yes | Same as above with exit codes 1, 5, 9; assert zero Sentry events |
| `safe-panic-message-allowlisted` | yes | Use SafePanic with a registered messageID, assert Sentry event's `message` matches verbatim |
| `unscrubbed-panic-tagged` | yes | Plain `panic("user-typed content")`, assert Sentry event's `message: "unscrubbed panic"` and tag `message: unscrubbed` |
| `scrubber-strips-paths` | yes | Fuzz tests in `internal/telemetry/scrubber_fuzz_test.go` — covered by Go's fuzzing infrastructure, runs in CI |
| `debug-error-honors-optout` | yes | Set opt-out, run `specscore debug error --text foo`, assert no HTTP request + stdout no-op message |
| `debug-error-force-bypasses-optout` | yes | Set opt-out, run `specscore debug error --text foo --force`, assert one HTTP request issued, exit 0 |
| `debug-error-ci-smoke-test` | yes | `CI=true specscore debug error --text "ci-smoke-test"`, assert exit 0 + no HTTP request (opt-out path exercised) |
| `sentry-release-tag-applied` | yes | Network-mocked: capture the event, assert `release` tag equals `cli --version` output |
| `sentry-alert-on-new-signature` | no | Operational check (Sentry UI configuration). Verified at v0.2.0 release-checklist time, recorded in release notes. |
| `docs-crash-reports-sections-present` | yes | Static-doc structure check; markdown-lint over `docs/telemetry.md` |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| `cli/telemetry` (parent) | Provides every piece of shared plumbing: install_id, opt-out evaluation, first-run notice, PersistentPostRun hook with panic recovery, channel registry, transmission-timeout wrapper, fixed property-key enum, `docs/telemetry.md` skeleton, the privacy invariant. This child adds no parent-level CLI surface; the `specscore debug error` subcommand is namespaced under the new `specscore debug` parent. |
| [`cli/telemetry/usage-telemetry`](../usage-telemetry/README.md) | Sibling. Coexists as a second registered channel. Shares install_id, opt-out plumbing, and the `docs/telemetry.md` skeleton. Does NOT share the Sentry client, the scrubber, the panic-recovery hook, or any event. The scrubber MAY end up in a shared package file (`scrubber.go`) if that file is later used by usage-telemetry too — Plan-time decision. |
| `cli` (root) | Adds the `defer recover()` panic handler in PersistentPreRun. Adds the `specscore debug` command group. No other CLI command's behavior is affected. |
| `cli/init` | Out of scope for any modification. `init`'s exit codes (already documented) MUST be audited per REQ:exit-code-audit-precondition — if any return ≥10 for expected conditions, they're renumbered before v0.2.0. |

## Not Doing / Out of Scope

- **PostHog for crash reports.** Owned by the sibling usage-telemetry channel as a product-analytics destination, not a crash-reporting destination.
- **The `caller` flag.** Owned by usage-telemetry (the agent-identifying property is irrelevant for crash reports — we want to know what *broke*, not who was driving).
- **Self-hosted GlitchTip.** Q3+ decision per the parent Idea (the Sentry-vs-GlitchTip OQ was resolved in favor of Sentry-for-v1). Same wire protocol means later migration is a config change.
- **Source-map / symbolication infrastructure** beyond what Sentry's Go SDK does natively. Go binaries are already symbolicated by the runtime; advanced symbolication is unnecessary at this scale.
- **User-feedback dialog on crash.** Sentry has a feature for this; it's IDE/web-app-shaped, not CLI-shaped. Out.
- **Breadcrumbs** that would require tracing every CLI command leading up to the crash. Too noisy at this scale; overlaps the usage-telemetry channel.
- **Source-map upload to Sentry from goreleaser.** Optional Sentry workflow; not required for Go binaries.
- **Cross-channel correlation.** No automatic linking between a usage-telemetry event and a crash-reports event from the same invocation. Both carry `install_id` so PostHog and Sentry can be joined manually if needed.
- **The `usage-stats` channel.** Owned by the sibling usage-telemetry Feature.

## Assumption Carryover

| Idea assumption | Status |
|---|---|
| Stack-frame scrubbing reliably strips file paths and any user-authored content before transmission. | **Encoded** in REQ:stack-frame-scrubber, REQ:panic-message-safe-allowlist, REQ:scrubber-fuzz-tests. Validated by fuzz tests in CI. |
| Sentry's free tier (5k events/mo) covers Q2 panic volume. | Inherited from the Idea. Monitored operationally (Sentry quota dashboard); not encoded as a code REQ. If we ever approach the cap, the CLI has a bigger problem than telemetry quota. |
| Users tolerate a separate opt-out for crash telemetry alongside product telemetry. | Inherited. The parent's per-channel opt-out (REQ:telemetry-subcommand-surface) is exactly the mechanism. Validated by absence of confusion in user reports. |
| Panic-recovery + exit-code ≥10 captures the right set of errors without false positives from expected paths. | **Encoded** in REQ:trigger-on-panic-recovery, REQ:trigger-on-exit-code-ge-10, REQ:exit-code-audit-precondition. The audit is the validation. |
| GlitchTip / self-host doesn't become necessary in Q2. | Inherited. Same wire protocol means future migration is a DSN config swap, not a rewrite. |

## Acceptance Criteria

### AC: sentry-client-eu-region

**Requirements:** cli/telemetry/errors-telemetry#req:sentry-go-sdk, cli/telemetry/errors-telemetry#req:sentry-eu-region

**Given** a release build of `specscore` with a valid Sentry DSN embedded (pointing at an EU project) AND telemetry enabled
**When** the test harness forces a panic via a test-only command (e.g. `specscore debug error --text "test" --force`) and a proxy captures the outbound HTTPS request
**Then** the captured request's URL MUST have a host matching the EU regional ingest endpoint (e.g. `*.ingest.de.sentry.io`); no request MUST be issued to `*.ingest.us.sentry.io` or any non-EU Sentry endpoint.

### AC: sentry-dsn-empty-no-op

**Requirements:** cli/telemetry/errors-telemetry#req:sentry-dsn-embedded-at-build-time

**Given** a `specscore` binary built with empty `-ldflags` (no DSN injected) AND telemetry enabled
**When** the test harness forces a panic via a test-only command AND a proxy captures all outbound HTTPS
**Then** no HTTPS request to any Sentry endpoint MUST be issued; `specscore telemetry status` MUST nonetheless report the `crash-reports` channel as `registered`.

### AC: crash-reports-channel-registered

**Requirements:** cli/telemetry/errors-telemetry#req:crash-reports-channel-registration

**Given** any build of `specscore`
**When** `specscore telemetry status` runs
**Then** stdout MUST contain a row for the channel `crash-reports` with its current enabled/disabled state and source.

### AC: panic-triggers-event

**Requirements:** cli/telemetry/errors-telemetry#req:trigger-on-panic-recovery

**Given** a release build with a valid Sentry DSN, telemetry enabled, and a proxy capturing outbound HTTPS
**When** a test-only command that calls `panic("test panic")` runs
**Then** exactly one Sentry event MUST be transmitted; the event's `release` tag MUST equal `cli_version`; the event's `message` field MUST be the literal string `"unscrubbed panic"` (per REQ:panic-message-safe-allowlist); the event MUST carry the tag `message: unscrubbed`.

### AC: panic-and-exit-ge-10-emits-single-event

**Requirements:** cli/telemetry/errors-telemetry#req:trigger-on-panic-recovery, cli/telemetry/errors-telemetry#req:trigger-on-exit-code-ge-10

**Given** a release build with a valid Sentry DSN, telemetry enabled, and a proxy capturing outbound HTTPS
**When** a test-only command runs that BOTH panics AND would have exited with code 12 (e.g. the panic occurs inside a code path that had already set the exit code)
**Then** exactly ONE Sentry event MUST be transmitted (not two); the event's `message` MUST match the panic signature (e.g. `"unscrubbed panic"` for a plain panic), NOT the literal string `"unexpected exit code 12 from cmd X"`. The panic-priority rule from REQ:trigger-on-exit-code-ge-10 is verified by absence of the exit-code event.

### AC: exit-code-thresholds

**Requirements:** cli/telemetry/errors-telemetry#req:trigger-on-exit-code-ge-10

**Given** a release build with a valid Sentry DSN, telemetry enabled, and a proxy capturing outbound HTTPS
**When** the following test-only commands run, each in a fresh process:
- A command that exits with code `0`
- A command that exits with code `1`
- A command that exits with code `5`
- A command that exits with code `9`
- A command that exits with code `10`
- A command that exits with code `12`

**Then** no Sentry event MUST be transmitted for the first four (exit 0–9); exactly one Sentry event MUST be transmitted for each of the last two (exit ≥10); the event's `message` MUST name the exit code (e.g. `"unexpected exit code 12 from cmd X"`).

### AC: safe-panic-allowlisted-message-transmitted-verbatim

**Requirements:** cli/telemetry/errors-telemetry#req:panic-message-safe-allowlist

**Given** a release build, telemetry enabled, a proxy capturing outbound HTTPS, AND a test-only command that calls `panic(telemetry.SafePanic("test-known-id", fmt.Errorf("user typed %s", "secret-content")))`
**When** the command runs
**Then** the Sentry event's `message` field MUST be the literal string `"test-known-id"`; the literal string `"secret-content"` MUST NOT appear anywhere in the captured request body; the `message: unscrubbed` tag MUST NOT be present.

### AC: scrubber-strips-paths

**Requirements:** cli/telemetry/errors-telemetry#req:stack-frame-scrubber, cli/telemetry/errors-telemetry#req:scrubber-fuzz-tests

**Given** the fuzz test corpus in `internal/telemetry/scrubber_fuzz_test.go` includes adversarial paths (`/Users/...`, `/home/...`, project paths with embedded `secret` strings, frames whose function names contain UTF-8 multi-byte chars)
**When** `go test -fuzz=Fuzz ./internal/telemetry/ -fuzztime=30s` runs in CI
**Then** the fuzzer MUST NOT find any input that causes the scrubber to emit a string containing a leading `/`, the substring `/Users/`, the substring `/home/`, or any literal value that appears in the input but should have been stripped; on any failure CI MUST exit non-zero.

### AC: scrubber-strips-local-variables

**Requirements:** cli/telemetry/errors-telemetry#req:stack-frame-scrubber

**Given** a release build, telemetry enabled, a proxy capturing outbound HTTPS, AND a test-only command that holds a local variable named `secretSentinel` with value `"VERY-SECRET-VALUE-SENTINEL-2026"` immediately before calling `panic(...)`
**When** the command runs
**Then** the captured Sentry event payload MUST NOT contain the literal string `"VERY-SECRET-VALUE-SENTINEL-2026"` anywhere — verifying that local-variable values attached by the Sentry SDK's default frame metadata are explicitly stripped before transmission (clause 3 of REQ:stack-frame-scrubber).

### AC: debug-error-honors-optout

**Requirements:** cli/telemetry/errors-telemetry#req:debug-error-subcommand

**Given** `crash-reports` opted out (e.g. `specscore telemetry disable crash-reports` was previously run) AND a proxy capturing outbound HTTPS
**When** `specscore debug error --text "hello"` runs (no `--force`)
**Then** no HTTPS request MUST be issued; stdout MUST contain a no-op message that names the literal string `specscore telemetry enable crash-reports`; exit code MUST be `0`.

### AC: debug-error-force-bypasses-optout

**Requirements:** cli/telemetry/errors-telemetry#req:debug-error-subcommand

**Given** `crash-reports` opted out AND a proxy capturing outbound HTTPS AND a valid Sentry DSN embedded
**When** `specscore debug error --text "hello" --force` runs (the value `hello` is NOT in the SafePanic allowlist per REQ:panic-message-safe-allowlist)
**Then** exactly one Sentry event MUST be transmitted; the event's `message` field MUST be the literal string `"unscrubbed panic"` (NOT `"hello"`) per REQ:debug-error-subcommand's coercion contract; the event MUST carry tag `debug: true` AND tag `message: unscrubbed`; stdout MUST contain a precise description of what was sent (the coerced message, tags). The persistent opt-out state (`~/.specscore/telemetry.yaml`) MUST be byte-identical to its pre-invocation state. Exit code MUST be `0`.

### AC: debug-error-ci-smoke-test

**Requirements:** cli/telemetry/errors-telemetry#req:debug-error-ci-usage

**Given** `CI=true` in the environment AND a proxy capturing outbound HTTPS
**When** `specscore debug error --text "ci-smoke-test"` runs (no `--force`)
**Then** no HTTPS request MUST be issued; exit code MUST be `0`; stdout MUST contain the no-op message confirming the opt-out path was taken.

### AC: transmit-panic-does-not-mask-user-exit-code

**Requirements:** cli/telemetry/errors-telemetry#req:transmit-callback-must-not-mask-exit-code

**Given** a release build with a Sentry DSN that points at a deliberately broken endpoint AND a test-only path that injects a panic into the scrubber (e.g. via a build-tagged hook in `internal/telemetry/scrubber_panic_inject_test.go`)
**When** a test-only command that itself exits with code `0` runs, triggering the scrubber-panic path during PostRun's transmit
**Then** the user's command's exit code MUST be `0` (not non-zero, not 10, not whatever Go's default-panic exit would produce); stdout MUST NOT contain any telemetry-related diagnostics; stderr MUST be silent absent `--verbose`. The user-observable behavior is identical whether the scrubber succeeded or panicked.

### AC: sentry-release-tag-applied

**Requirements:** cli/telemetry/errors-telemetry#req:sentry-release-tag

**Given** a release build with a valid DSN, telemetry enabled, and a proxy
**When** a test-only command forces a panic and the captured Sentry request body is inspected
**Then** the event payload MUST contain a `release` field whose value matches the string printed by `specscore --version` (e.g. `0.2.0`).

### AC: sentry-alert-on-new-signature

**Requirements:** cli/telemetry/errors-telemetry#req:sentry-alert-on-new-signature

**Given** v0.2.0 is being released
**When** the release checklist's "Telemetry verification" step is executed
**Then** the Sentry project named `specscore-cli` in the EU region MUST contain an alert rule that fires on any new issue (crash signature) in the latest release tag AND that excludes events tagged `debug: true`. Confirmed manually by the release operator; recorded in the release notes.

### AC: docs-crash-reports-sections-present

**Requirements:** cli/telemetry/errors-telemetry#req:docs-crash-reports-section

**Given** a fresh checkout after this Feature is implemented
**When** the file `docs/telemetry.md` is opened
**Then** the file MUST contain a `### crash-reports` subsection inside `## Channels` with at least the literal strings `panic`, `exit code`, and `de.sentry.io`; a `### crash-reports events` subsection inside `## Event Schema` enumerating the Sentry event shape (`message`, `release`, `debug` tag); and a `### crash-reports` subsection inside `## Data Retention` referencing Sentry's free-tier retention policy.

## Open Questions

- **Initial `messageID` allowlist contents.** REQ:panic-message-safe-allowlist starts the production allowlist populated by the Plan's panic-site-audit task. The list depends on the audit results, but the audit is scoped (per resolution below) to **high-value sites only**, not full retrofit.

> **Resolved during user review (2026-05-21):**
> - SafePanic retrofit scope → **only audit-identified high-value sites**. v0.2.0 ships with the high-value sites wrapped and the long tail emitting `"unscrubbed panic"` + `message: unscrubbed` tag. The unscrubbed tag itself is operationally observable, so coverage gaps surface as alerts and motivate follow-up retrofits.
> - Scrubber location → **`internal/telemetry/scrubber.go`** (separate file, not inlined in `errors.go`). Locked in REQ:stack-frame-scrubber.

---
*This document follows the https://specscore.md/feature-specification*
