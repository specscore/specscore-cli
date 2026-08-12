---
format: https://specscore.md/plan-specification
status: Approved
---

# Plan: CLI Telemetry — Usage Stats Channel

**Status:** Approved
**Source Feature:** cli/telemetry/usage-telemetry
**Date:** 2026-05-21
**Owner:** alexandertrakhimenok
**Supersedes:** —

## Summary

Decomposes the `cli/telemetry/usage-telemetry` child Feature into 5 ordered tasks that wire the PostHog product-analytics channel onto the parent's `cli/telemetry` plumbing. All 8 source-Feature ACs are covered; no deferrals.

## Approach

Tasks follow a strict bottom-up dependency order on top of an already-built parent: PostHog client + registration first (Task 1), then the caller-identification surface that downstream events need (Task 2), then the event-emission transmit function that consumes both (Task 3), then the operational provisioning that turns dev builds into real-emitting release builds (Task 4), then the docs population that closes the loop on the parent's `--help`-points-to-docs/telemetry.md contract (Task 5). Caller resolution + enum coercion is bundled into one task because they form a single conceptual unit and Task 3 depends on the resolved-and-coerced caller value being available at event-build time.

This Plan assumes the parent Plan (`cli-telemetry`) is implemented through at least Task 5 (channel registry + transmission-timeout wrapper) before Task 1 here can land — the parent provides `RegisterChannel(name, transmit)` and the 500 ms wrapper that this child registers against.

## Tasks

### Task 1: PostHog client wiring + channel registration

**Verifies:** cli/telemetry/usage-telemetry#ac:posthog-client-eu-region, cli/telemetry/usage-telemetry#ac:posthog-write-key-empty-no-op, cli/telemetry/usage-telemetry#ac:usage-stats-channel-registered

Create `internal/telemetry/usage.go` as the sole file importing `github.com/posthog/posthog-go`. Declare the package-scoped `posthogWriteKey` variable (target for `-ldflags "-X internal/telemetry.posthogWriteKey=..."` injection). Instantiate the PostHog client with EU endpoint (`https://eu.i.posthog.com`) as a Go constant. Add a Go `init()` function that calls `telemetry.RegisterChannel("usage-stats", transmit)`. When `posthogWriteKey` is the empty string (dev build), the registered transmit-fn MUST silently no-op — no HTTPS request issued, but the channel still appears in `specscore telemetry status`.

### Task 2: `--caller` flag + env var + enum coercion

**Verifies:** cli/telemetry/usage-telemetry#ac:caller-resolution-precedence, cli/telemetry/usage-telemetry#ac:caller-enum-coercion

Add a global `--caller <value>` flag to the root cobra command in `internal/cli/root.go` (visible in `--help`, NOT hidden). Read `SPECSCORE_CALLER` env var as the fallback. Implement the resolution precedence in `internal/telemetry/usage.go`: `--caller` flag > `SPECSCORE_CALLER` env var > default literal `cli`. Define the 20-value closed-enum allowlist (`cli`, `claude`, `codex`, `aider`, `opencode`, `goose`, `cursor`, `gemini`, `copilot`, `devin`, `cline`, `roo`, `continue`, `windsurf`, `zed`, `amazon-q`, `tabnine`, `pi.dev`, `antigravity.google`, `other`) as Go constants. Coerce any non-allowlist resolved value to `other` BEFORE handing it to PostHog — coercion happens in `usage.go`, NOT at the cobra flag-parsing layer, so a script setting `--caller my-custom-tag` does not fail the user's command.

### Task 3: Event emission — `cli.command.completed` with full property set

**Verifies:** cli/telemetry/usage-telemetry#ac:event-name-and-properties

Implement the `transmit` function in `internal/telemetry/usage.go` that converts a parent-emitted typed property struct into a PostHog event. Event name: literal `cli.command.completed` for every invocation (first-run or otherwise — `is_first_run` distinguishes). PostHog `distinct_id` parameter MUST be the `install_id` value (so PostHog groups events per machine and the north-star funnel can group by it). Populate all 10 properties per the value-source contract in REQ:usage-stats-event-properties: `command` (dot-separated cobra path), `success` (bool from exit code 0), `duration_ms` (PreRun→PostRun integer), `exit_code`, `cli_version` (build-time from goreleaser), `os`/`arch` (runtime), `caller` (from Task 2's resolved+coerced value), `install_id` (from parent's helper), `is_first_run`. Wire the transmit-fn into the registration from Task 1.

### Task 4: PostHog project provisioning + north-star funnel (operational)

**Verifies:** cli/telemetry/usage-telemetry#ac:posthog-funnel-defined

Operational task, not code. (a) Create a PostHog project named `specscore-cli` in the EU region. (b) Capture the project's write key and store as a GitHub Actions secret (`POSTHOG_WRITE_KEY`). (c) Update `.goreleaser.yml` to inject the secret via `-ldflags "-X internal/telemetry.posthogWriteKey=${POSTHOG_WRITE_KEY}"` for release builds; dev builds get the empty string. (d) Configure the funnel named exactly `North-Star: First Real Spec Use within 7 Days` with three steps per REQ:posthog-funnel-defined (first_run=true → first feature.create → second feature.create within 7 days), grouped by `distinct_id`. (e) Record the operator's confirmation of the funnel's presence in the v0.2.0 release notes.

### Task 5: `docs/telemetry.md` `### usage-stats` sections

**Verifies:** cli/telemetry/usage-telemetry#ac:docs-usage-stats-sections-present

Populate the parent's already-built `docs/telemetry.md` skeleton with this child's content. Inside `## Channels`: add `### usage-stats` subsection with a one-paragraph description, the literal event name `cli.command.completed`, a link to PostHog's privacy/data-handling page, and an EU-region callout (literal `eu.i.posthog.com`). Inside `## Event Schema`: add `### usage-stats events` subsection enumerating all 10 property keys (matching REQ:usage-stats-event-properties) AND enumerating the closed `caller` enum end-to-end (all 20 values including `cli` and `other`) — this closes the loop on REQ:caller-flag's `--help` pointer at `docs/telemetry.md`. Inside `## Data Retention`: add `### usage-stats` subsection naming PostHog's retention policy (7 years per free-tier defaults) and how to request deletion via PostHog's GDPR endpoint, parameterised by `install_id`.

## Open Questions

- **Exact prose of the `--caller` `--help` text (Task 2).** REQ:caller-flag fixes the audience ("AI coding agents driving the CLI") and the pointer ("see `docs/telemetry.md` for the full list") but the connective wording gets a copy-review pass during Task 2 implementation. Lean draft from the source Feature's user-review notes: `--caller string   The AI coding agent driving the CLI (e.g. claude, codex, aider). See docs/telemetry.md for the full list. Default: cli (human at terminal).`
- **`pi.dev` and `antigravity.google` description prose (Task 5).** REQ:caller-enum-known-values pins the value strings; the one-line product descriptions in `docs/telemetry.md` should reference the canonical URLs (`https://pi.dev/`, `https://antigravity.google/`).

---
*This document follows the https://specscore.md/plan-specification*
