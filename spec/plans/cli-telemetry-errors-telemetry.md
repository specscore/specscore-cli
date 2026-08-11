---
format: https://specscore.md/plan-specification
status: Approved
---

# Plan: CLI Telemetry — Crash Reports Channel

**Status:** Approved
**Source Feature:** cli/telemetry/errors-telemetry
**Date:** 2026-05-21
**Owner:** alexandertrakhimenok
**Supersedes:** —

## Summary

Decomposes the `cli/telemetry/errors-telemetry` child Feature into 7 ordered tasks that wire the Sentry crash-reporting channel onto the parent's `cli/telemetry` plumbing. All 16 source-Feature ACs are covered; no deferrals.

## Approach

Tasks follow a strict bottom-up order on top of the already-built parent: scrubber first (Task 1) because every downstream task either invokes it or relies on the SafePanic allowlist mechanism it ships; then the Sentry client + channel registration (Task 2) which gives us the file structure and the no-op-when-DSN-empty behavior; then the transmit-callback safety properties (Task 3 — defensive recover + release tag) which are inherent to the transmit-fn independent of trigger conditions; then the trigger logic + exit-code audit + production-allowlist population (Task 4) which is the largest single chunk of work and the only task that modifies multiple files in one pass; then the `specscore debug error` verification subcommand (Task 5); then the Sentry project provisioning and alert configuration (Task 6, operational); then the documentation population (Task 7).

This Plan assumes the parent Plan (`cli-telemetry`) is implemented through at least its Task 5 (channel registry + transmission-timeout wrapper) and Task 6 (root command wiring with PersistentPostRun loop). One cross-Plan coordination point is flagged as an Outstanding Question below: the transmit-fn signature needs to carry an Event context (panic info + exit code) so per-channel triggering inside the transmit body works without forcing the parent's PostRun loop to know about channel-specific dispatch rules.

## Tasks

### Task 1: Scrubber package — paths, locals, SafePanic allowlist, fuzz tests

**Verifies:** cli/telemetry/errors-telemetry#ac:scrubber-strips-paths, cli/telemetry/errors-telemetry#ac:scrubber-strips-local-variables, cli/telemetry/errors-telemetry#ac:safe-panic-allowlisted-message-transmitted-verbatim

Build `internal/telemetry/scrubber.go`: the stack-frame scrubber (file paths → basenames, line numbers + function names preserved verbatim, local-variable values stripped from frame metadata via explicit Sentry-SDK option disable), the `SafePanic(messageID, err)` constructor + payload type, and the closed-enum allowlist as Go constants. The allowlist ships with one mandatory test-only entry `test-known-id` whose declaration MUST be guarded by a `_test.go` file (Go test-only compilation) so it never appears in release binaries; production messageIDs are added by Task 4 as the panic-site audit produces them. Build `internal/telemetry/scrubber_fuzz_test.go` with adversarial corpus (paths like `/Users/...`, `/home/...`, sentinel user-content strings, multi-byte UTF-8); fuzz tests MUST run in CI with at least a 30 s smoke budget per AC:scrubber-strips-paths, and CI MUST fail on any fuzz failure.

### Task 2: Sentry client wiring + channel registration

**Verifies:** cli/telemetry/errors-telemetry#ac:sentry-client-eu-region, cli/telemetry/errors-telemetry#ac:sentry-dsn-empty-no-op, cli/telemetry/errors-telemetry#ac:crash-reports-channel-registered

Create `internal/telemetry/errors.go` as the sole file importing `github.com/getsentry/sentry-go`. Declare the package-scoped `sentryDSN` variable (target for `-ldflags "-X internal/telemetry.sentryDSN=..."` injection); the declaration MUST live in `errors.go` per the parent's vendor-SDK-import-confinement audit surface. Initialize the Sentry client at `init()` time configured for the EU region (DSN form `https://<key>@<org>.ingest.de.sentry.io/<project-id>`). Add an `init()` function that calls `telemetry.RegisterChannel("crash-reports", transmit)`. When `sentryDSN` is the empty string (dev build), the channel MUST still register but the transmit callback MUST silently no-op — no Sentry HTTP requests issued.

### Task 3: Transmit callback — defensive recover + release tag

**Verifies:** cli/telemetry/errors-telemetry#ac:transmit-panic-does-not-mask-user-exit-code, cli/telemetry/errors-telemetry#ac:sentry-release-tag-applied

Implement the transmit callback's body in `internal/telemetry/errors.go` independent of triggering logic. Install a `defer recover()` around the scrub-and-emit work — any panic inside the callback (scrubber bug, Sentry SDK panic, malformed event) MUST be caught silently; the user's original exit code MUST be preserved byte-identical; at `--verbose`, log a one-line stderr breadcrumb. Apply the `release` tag (= `cli_version` from build-time goreleaser injection) to every emitted event so Sentry correlates signatures to specific releases. These safety properties are inherent to the transmit-fn regardless of when/why it's invoked.

### Task 4: Trigger conditions — panic recovery, exit-code ≥10, exit-code audit, production allowlist

**Verifies:** cli/telemetry/errors-telemetry#ac:panic-triggers-event, cli/telemetry/errors-telemetry#ac:exit-code-thresholds, cli/telemetry/errors-telemetry#ac:panic-and-exit-ge-10-emits-single-event

The largest task — bundles five logically inseparable units. **Pre-step (0):** inspect the parent Plan's implemented `RegisterChannel` signature in `internal/telemetry/`. If it is context-free (no Event / panic / exit-code arguments passed to the transmit-fn), file an amendment PR to the parent Plan's Task 6 that adds an Event-carrying signature before continuing. (a) Install `defer recover()` at `internal/cli/root.go`'s PersistentPreRun-installed handler so panics are captured into an Event the parent's PostRun loop passes to channels. (b) Implement exit-code detection in PostRun so ExitCode is part of the Event. (c) Implement the conditional emit logic inside the crash-reports transmit-fn: emit when `Panic != nil` OR `ExitCode ≥ 10`; panic takes priority over exit-code so a panicking command emits exactly one event (the panic-signature event, NOT the exit-code-synthesised event). (d) Audit every existing call site that returns exit code ≥10 per REQ:exit-code-audit-precondition: each expected condition gets renumbered into 1–9 first; the audit's file/line list MUST be recorded in the commit message. As a follow-on within the same pass, wrap the audit-identified high-value panic sites with `telemetry.SafePanic(...)` and add their messageIDs to the production allowlist in `scrubber.go` — the long-tail unwrapped panics continue to emit `"unscrubbed panic"` + tag `message: unscrubbed` per REQ:panic-message-safe-allowlist.

**Suggested commit cadence within Task 4** (one commit per logical unit, not one mega-commit): (i) signature amendment if needed (pre-step 0); (ii) audit + exit-code renumbering of expected ≥10 sites; (iii) defer recover() in PreRun + ExitCode detection in PostRun; (iv) conditional emit + panic-priority logic in transmit-fn; (v) SafePanic retrofit + production allowlist population. Each commit MUST reference `cli/telemetry/errors-telemetry#ac:<ac-slug>` for any AC it advances.

### Task 5: `specscore debug error` subcommand

**Verifies:** cli/telemetry/errors-telemetry#ac:debug-error-honors-optout, cli/telemetry/errors-telemetry#ac:debug-error-force-bypasses-optout, cli/telemetry/errors-telemetry#ac:debug-error-ci-smoke-test

Create `internal/cli/debug.go` with the `specscore debug` cobra command group plus the `error` subcommand. Flags: `--text "<msg>"` (required) and `--force` (optional bool). Without `--force`: consult the crash-reports opt-out signal via the parent's evaluator; if disabled, no-op and print to stdout a message naming `specscore telemetry enable crash-reports`; exit 0. With `--force`: bypass the opt-out for a single invocation, print to stdout exactly what's about to be sent (coerced message, tags including `debug: true`, target DSN); `~/.specscore/telemetry.yaml` MUST be byte-identical before/after. The `--text` value is interpreted as a candidate `messageID` (tested against the SafePanic allowlist per REQ:debug-error-subcommand) — non-allowlist values coerce to `"unscrubbed panic"` + tag `message: unscrubbed`. The subcommand MUST be hidden from `specscore --help` (cobra `Hidden: true`) but documented in `docs/telemetry.md`. CI smoke-test path (no `--force` + `CI=true`) exercises the opt-out path end-to-end.

### Task 6: Sentry project provisioning + alert rule (operational)

**Verifies:** cli/telemetry/errors-telemetry#ac:sentry-alert-on-new-signature

Operational task, not code. (a) Create a Sentry project named `specscore-cli` in Sentry's EU region. (b) Capture the project's DSN as a GitHub Actions secret (`SENTRY_DSN`). (c) Update `.goreleaser.yml` to inject the secret via `-ldflags "-X internal/telemetry.sentryDSN=${SENTRY_DSN}"` for release builds; dev builds get the empty string. (d) Configure an alert rule that fires on any new issue (Sentry's term for a new crash signature) appearing in the latest release tag AND that filters events tagged `debug: true` so `specscore debug error` invocations don't page the founder. Notification channel: Sentry's built-in email-to-founder for v0.2.0; Slack DM or webhook MAY be substituted later. (e) Record the operator's confirmation of the alert rule's presence in the v0.2.0 release notes; for an auditable trail, the release-notes entry MUST link to the Sentry alert-rule URL (the Sentry UI exposes a permalink per alert) AND quote the alert's filter expression including the `debug: true` exclusion clause, so a future reviewer can reconstruct what was configured without UI access.

### Task 7: `docs/telemetry.md` `### crash-reports` sections

**Verifies:** cli/telemetry/errors-telemetry#ac:docs-crash-reports-sections-present

Populate the parent-built `docs/telemetry.md` skeleton with this channel's content. Inside `## Channels`: add `### crash-reports` subsection with a one-paragraph description, an explicit statement that only panics and exit codes ≥10 trigger transmission (literal strings `panic` and `exit code`), a link to Sentry's privacy / data-handling page, and an EU callout (literal `de.sentry.io`). Inside `## Event Schema`: add `### crash-reports events` subsection enumerating the Sentry event shape (event message handling per SafePanic allowlist or `"unscrubbed panic"` coercion, scrubbed frame format, the `release` and `debug` tags). Inside `## What We Don't Collect`: add a reference from the channel back to the parent-owned invariant list. Inside `## Data Retention`: add `### crash-reports` subsection naming Sentry's free-tier retention (30 days for events, 90 days for issues) and how to request deletion.

## Open Questions

- **Cross-Plan coordination: transmit-fn signature carries Event context.** Parent Plan `cli-telemetry`'s Task 6 wires PersistentPostRun to "iterate the channel registry and invoke each transmit-fn." For this Plan's Task 4 to work (per-channel triggering based on panic info and exit code), the transmit-fn signature MUST accept an Event context containing the recovered panic value (or nil) and the exit code. If the parent Plan's Task 6 was implemented with a context-free transmit signature, this Plan's Task 4 amends the registration signature first. Coordinate with the parent Plan's implementer before starting Task 4.
- **Initial production allowlist contents (Task 4 output).** The set of messageIDs added to the production SafePanic allowlist depends on which existing panic sites the Task 4 audit identifies as "high-value." Per the source-Feature OQ resolution, scope is limited to high-value sites — not full retrofit. The exact list is the audit's deliverable, not pre-specifiable here.
- **Sentry alert notification channel (Task 6).** Sentry's built-in email is the v0.2.0 default; Slack DM or webhook is a one-line substitution if/when a Slack workspace exists. Decision is in Task 6's operational scope, not blocking the Plan.

---
*This document follows the https://specscore.md/plan-specification*
