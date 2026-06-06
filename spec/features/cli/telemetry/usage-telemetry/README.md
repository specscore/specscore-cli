---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: Usage Telemetry (CLI)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/telemetry/usage-telemetry?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/telemetry/usage-telemetry?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/telemetry/usage-telemetry?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/telemetry/usage-telemetry?op=request-change) |
**Status:** Stable
**Date:** 2026-05-21
**Owner:** alexandertrakhimenok
**Source Ideas:** cli-telemetry
**Supersedes:** —

## Summary

Implements the `usage-stats` channel registered against the parent [`cli/telemetry`](../README.md) Feature. Sends one anonymous product-analytics event per `specscore` CLI invocation to PostHog (cloud, EU region), tagged with the parent-defined property schema plus a caller-identification field (`--caller` flag, `SPECSCORE_CALLER` env var) that segments plugin-driven usage from raw-CLI usage and AI-agent-driven usage. The single goaled output of this Feature is the PostHog funnel that operationalises the 2026-Q2 GTM north-star metric: `first_run → first feature.create → second feature.create within 7 days of the same install_id`.

## Problem

The parent Feature locked the shared plumbing (package boundary, opt-out, first-run notice, install_id, transmission timeout, channel registry). What remained was the product-analytics channel itself: how invocations turn into events, what those events carry, how plugin-driven and AI-agent-driven invocations are distinguished from human-typed CLI use, and how the resulting event stream answers the north-star question "did 100 developers install AND use SpecScore on a real spec twice within 7 days?"

This Feature is small on purpose. Every architectural decision that could have lived here was deliberately hoisted into the parent so a privacy reviewer auditing the data path has one place to read. The child encodes only what is unique to product analytics: PostHog as the destination, the caller-tagging mechanism, the event name and shape, and the north-star funnel definition.

## Behavior

### PostHog client

The `usage-stats` channel transmits to PostHog cloud, EU region, via the PostHog Go SDK confined to `internal/telemetry/usage.go` per the parent's vendor-SDK import confinement REQ.

#### REQ: posthog-go-sdk

`internal/telemetry/usage.go` MUST import `github.com/posthog/posthog-go` and instantiate the client at package init time inside the boundary the parent established. The SDK's batched-async sender MUST be used (not the synchronous capture path); flush is bounded by the parent's REQ:transmission-hard-timeout (500 ms).

#### REQ: posthog-eu-region

The PostHog client MUST be configured with the EU region endpoint (`https://eu.i.posthog.com`, or the SDK's `Endpoint` config field set to the EU value). The US endpoint MUST NOT be used. The region is encoded as a Go constant in `internal/telemetry/usage.go`; changing region requires a code change.

#### REQ: posthog-write-key-embedded-at-build-time

The PostHog write key (project API key, public-but-not-secret per PostHog's docs but treated as build-time data) MUST be compiled into the binary via Go's `-ldflags "-X internal/telemetry.posthogWriteKey=..."` during release builds. The variable is package-scoped (which is what Go ldflags require) but its *declaration* MUST live in `internal/telemetry/usage.go` so the vendor-SDK import-confinement audit surface (parent's REQ:vendor-sdk-import-confinement) stays intact. The key MUST NOT be read from an environment variable at runtime — users MUST NOT need to set anything for telemetry to work. When the embedded value is the empty string (i.e. a developer build with no key), the channel MUST register with the parent but `RegisterChannel`'s transmit callback MUST silently no-op; no PostHog HTTP requests MUST be issued.

### Channel registration

The `usage-stats` channel registers with the parent's registry exactly once.

#### REQ: usage-stats-channel-registration

`internal/telemetry/usage.go` MUST contain a Go `init()` function that calls `telemetry.RegisterChannel("usage-stats", transmit)` where `transmit` is the function that sends one event to PostHog. The registered name MUST be the exact string `usage-stats` — it MUST match the enumerated channel-registry name in the parent's REQ:channel-registry. The registration MUST be the only call site for `RegisterChannel("usage-stats", ...)` in the entire repo.

### Event shape and name

One event per CLI invocation, emitted in PersistentPostRun by the parent's plumbing, carrying the parent's fixed-property-key set plus the channel-specific event name.

#### REQ: usage-stats-event-name

Every event emitted by this channel MUST use the PostHog event name `cli.command.completed`. No other event name MUST be emitted by this channel. First-run invocations MUST also use this name — the `is_first_run` property (from the parent's REQ:fixed-event-property-keys) distinguishes them.

#### REQ: usage-stats-event-properties

Each `cli.command.completed` event MUST populate all ten properties from the parent's REQ:fixed-event-property-keys: `command`, `success`, `duration_ms`, `exit_code`, `cli_version`, `os`, `arch`, `caller`, `install_id`, `is_first_run`. Properties MUST be set per this contract:

| Property | Value source |
|---|---|
| `command` | The cobra command path of the executed command, e.g. `feature.create`, `spec.lint`, `telemetry.status`. Use dot-separated form, NOT space-separated. |
| `success` | `true` if the command's exit code is `0`, `false` otherwise. |
| `duration_ms` | Wall-clock duration of the command, measured from PersistentPreRun start to PersistentPostRun emission, in integer milliseconds. |
| `exit_code` | The integer exit code as returned by the command. |
| `cli_version` | The version string from `specscore --version`, embedded at build time via goreleaser. |
| `os` | `runtime.GOOS` (e.g. `darwin`, `linux`, `windows`). |
| `arch` | `runtime.GOARCH` (e.g. `amd64`, `arm64`). |
| `caller` | The resolved caller value per REQ:caller-resolution. |
| `install_id` | Read from the parent's install-id helper (REQ:install-id-file-path). |
| `is_first_run` | `true` if and only if the install_id file was created during *this* invocation; `false` otherwise. |

The PostHog `distinct_id` parameter on each event MUST be the `install_id` value, so PostHog groups events per machine.

### Caller identification

The `caller` property segments invocation sources without identifying users.

#### REQ: caller-flag

`specscore` MUST accept a global `--caller <value>` flag on the root command. The flag MUST be visible in `specscore --help` (NOT hidden) and described as the integration point for AI coding agents that drive the CLI (Claude Code, Codex, Aider, etc. — see REQ:caller-enum-known-values for the full list). The `--help` text MUST point at `docs/telemetry.md` for the enumerated values; it MUST NOT list the full enum inline (the help text stays terse).

#### REQ: caller-env-var

`specscore` MUST accept the env var `SPECSCORE_CALLER` as an alternative to the `--caller` flag for callers that cannot inject flags (e.g. shell aliases that wrap `specscore`).

#### REQ: caller-resolution

The resolved `caller` value sent to PostHog MUST be computed via this precedence, taking the first non-empty source:

1. `--caller <value>` flag on the current invocation.
2. `SPECSCORE_CALLER` env var.
3. Default literal string `cli`.

The resolved value MUST then be passed through the enum guard (REQ:caller-enum-known-values).

#### REQ: caller-enum-known-values

The `caller` field identifies which AI coding agent (if any) is driving the CLI. CI is not represented in this enum — CI environments are already handled by the parent's auto-disable opt-out (REQ:opt-out-signal-precedence step 3) so no event transmits at all. Specstudio-skills, our own Claude Code plugin, sets `SPECSCORE_CALLER=claude` because it IS Claude Code; "specstudio-skills user vs raw Claude user" segmentation is **approximated by patterns in** the `command` property (the plugin's skills invoke a narrow set of commands in a stereotyped order), NOT recovered deterministically. If post-launch the approximation proves inadequate, a follow-up Idea may add a `caller_subtype` or analogous property — tracked as an Outstanding Question below.

The set of recognised caller values is a closed enum:

| Value | Agent |
|---|---|
| `cli` | Default. A human typed `specscore` at a terminal; no agent set. |
| `claude` | Claude Code (Anthropic). Includes invocations via the `specstudio-skills` plugin — that plugin MUST set this value. |
| `codex` | Codex CLI (OpenAI). |
| `aider` | Aider — open-source AI pair programmer. |
| `opencode` | OpenCode — open-source AI coding agent. |
| `goose` | Goose (Block / Square). |
| `cursor` | Cursor (Anysphere). |
| `gemini` | Gemini CLI (Google). |
| `copilot` | GitHub Copilot CLI. |
| `devin` | Devin (Cognition Labs). |
| `cline` | Cline — open-source VS Code AI agent (formerly Claude Dev). |
| `roo` | Roo Code — Cline fork. |
| `continue` | Continue — open-source VS Code / JetBrains AI assistant. |
| `windsurf` | Windsurf — Codeium's AI-native IDE. |
| `zed` | Zed — collaborative code editor with AI mode. |
| `amazon-q` | Amazon Q Developer (AWS). |
| `tabnine` | Tabnine. |
| `pi.dev` | Pi (https://pi.dev/). Domain-form value chosen to disambiguate from other products named "Pi". |
| `antigravity.google` | Antigravity (https://antigravity.google/) — Google's AI-driven coding environment. Domain-form value chosen to make the project unambiguous. |

Any value not in this enum MUST be coerced to the string `other` before transmission. The coercion MUST happen inside `internal/telemetry/usage.go`, not at the cobra-flag-parsing layer — the flag itself accepts arbitrary strings (so an agent setting `caller=brand-new-agent-2026` doesn't fail the user's command); only the *transmitted* value is constrained. The full set of values PostHog ever sees from this channel is the twenty in this table, including `cli` (default) and `other` (coercion target). The enum is intentionally extended only by amending this REQ — new agents require a code change.

### North-star funnel

PostHog must be configured with a single named funnel that operationalises the GTM north-star metric. Code does not create the funnel (PostHog provides no API for ad-hoc funnel creation that's worth scripting at our scale) — this Feature documents the contract the operator implements once in the PostHog UI.

#### REQ: posthog-funnel-defined

The PostHog project for `usage-stats` MUST contain a funnel named exactly `North-Star: First Real Spec Use within 7 Days` with these three steps:

1. **Step 1 — First run.** `cli.command.completed` event with property `is_first_run = true`.
2. **Step 2 — First spec authoring.** `cli.command.completed` event with property `command = feature.create`, occurring after Step 1.
3. **Step 3 — Second spec authoring.** `cli.command.completed` event with property `command = feature.create`, occurring after Step 2 AND within 7 days of Step 1.

The funnel MUST be grouped by `distinct_id` (which equals `install_id` per REQ:usage-stats-event-properties), so progression is per-machine. The funnel definition is part of the deliverable; this Feature's `## Acceptance Criteria` includes a manual operator check (`docs/telemetry.md` lists the funnel name and the operator confirms it exists in the PostHog UI before declaring v0.2.0 shipped).

### Documentation contribution

This Feature populates the parent-owned `docs/telemetry.md` skeleton with its channel-specific content.

#### REQ: docs-usage-stats-section

`docs/telemetry.md` MUST contain, inside the parent's `## Channels` section, a `### usage-stats` subsection authored by this Feature with at least: a one-paragraph description of what the channel is for; the literal PostHog event name (`cli.command.completed`); a link to PostHog's privacy / data-handling page; and a callout that the EU region is used. The `## Event Schema` section MUST contain a `### usage-stats events` subsection authored by this Feature enumerating the ten properties (matching REQ:usage-stats-event-properties exactly) AND enumerating the full closed-enum value set for the `caller` property (matching REQ:caller-enum-known-values exactly: all twenty values including `cli` and the `other` coercion target). This closes the loop on REQ:caller-flag's contract that `--help` points at `docs/telemetry.md` for the enumerated caller values. The `## Data Retention` section MUST contain a `### usage-stats` subsection stating PostHog's data-retention policy that applies to this project (currently 7 years per PostHog's free-tier defaults) and how to request deletion (via PostHog's GDPR endpoint, parameterised by `install_id`).

## Architecture & Components

| Unit | Responsibility | Used by | Depends on |
|---|---|---|---|
| `internal/telemetry/usage.go` | The only file in the repo that imports `github.com/posthog/posthog-go`. Contains the `init()` registration, the PostHog client instantiation, the transmit callback that converts the parent's typed event property struct into a PostHog event, and the caller-enum-guard coercion. | The parent's PersistentPostRun, which calls the registered transmit-fn. | `internal/telemetry` (the parent); `github.com/posthog/posthog-go`. |
| `internal/cli/root.go` (modified) | Adds the global `--caller <value>` flag definition to the root cobra command. Reads `SPECSCORE_CALLER` env var into the same variable when the flag is absent. The resolved value is passed to `internal/telemetry`'s event property builder. | Every `specscore` invocation. | cobra; `internal/telemetry`. |
| Build pipeline (`.goreleaser.yml`, modified) | Injects the PostHog write key via `-ldflags "-X internal/telemetry.posthogWriteKey=<key>"` for release builds. Development builds get the empty string and silently no-op. | Release process. | The PostHog project key (stored in the GitHub Actions secrets for the release workflow). |
| PostHog project (operational) | The destination. Configured once in the PostHog UI: project named `specscore-cli` in the EU region, single funnel `North-Star: First Real Spec Use within 7 Days` per REQ:posthog-funnel-defined. | n/a (operational artifact). | n/a. |
| `docs/telemetry.md` (`### usage-stats` subsections) | Per-channel content authored by this Feature into the parent's skeleton. | Users; the first-run notice. | The parent's REQ:docs-telemetry-md-skeleton. |

## Data Flow

```
specscore <cmd> [args] [--caller <value>]
  │
  ├─→ parent PersistentPreRun
  │     (opt-out resolution, install_id, first-run notice — unchanged from parent spec)
  │
  ├─→ <cmd> Run()
  │     duration timer is running; --caller flag value (or SPECSCORE_CALLER env) captured
  │
  └─→ parent PersistentPostRun
        1. If usage-stats channel disabled by opt-out: skip
        2. Build the typed event property struct (all 10 keys populated per REQ:usage-stats-event-properties)
        3. Pass to internal/telemetry/usage.go's registered transmit-fn
        4. transmit-fn:
             a. Coerce `caller` to known-enum-or-`other`
             b. Call posthog.Enqueue(event{
                  EventName: "cli.command.completed",
                  DistinctID: install_id,
                  Properties: {...10 keys...},
                })
             c. Return immediately (PostHog SDK is async; flush bounded by parent's 500ms timeout)
        5. Parent flushes (bounded)
```

## Error Handling & Failure Modes

| Failure | Behavior |
|---|---|
| PostHog endpoint unreachable / 5xx / DNS | Caught by parent's REQ:transmission-hard-timeout. Event dropped silently. |
| PostHog Go SDK initialisation error (e.g. malformed write key) | Logged to stderr at `--verbose` only. The channel registers but the transmit-fn no-ops. User's command proceeds unaffected. |
| `posthogWriteKey` is the empty string (development build) | Per REQ:posthog-write-key-embedded-at-build-time: register the channel, no-op the transmit. No HTTP request issued. |
| `--caller` flag value contains characters PostHog rejects (e.g. embedded newline) | Coerced to `other` by the enum guard. PostHog never sees the original value. |
| `--caller` AND `SPECSCORE_CALLER` both set | The flag wins per REQ:caller-resolution. The env var is silently ignored. |
| User passes `--caller ""` (empty string) | Treated as "flag not supplied"; falls through to env var → default `cli`. |
| Multiple PostHog flushes in rapid succession (e.g. `specscore feature list` immediately followed by `specscore feature show <id>`) | Each invocation is independent (separate process, separate `init()`, separate flush). The 500 ms timeout applies per invocation. No batching across invocations. |
| User has very fast commands (e.g. `specscore --version` returns in <50 ms) | The PostHog SDK's async batched sender may not have flushed by the time the process exits. The parent's flush-with-timeout MUST be invoked from PersistentPostRun. Whether it succeeds is incidental; the 500 ms upper bound holds. |

## Testing Strategy

Per-AC Rehearse stubs MAY be scaffolded for the testable ACs. The PostHog-funnel-defined AC is an operational check, not a runtime test — recorded as a skip with rationale.

## Rehearse Integration

| AC | Stub? | Rationale |
|---|---|---|
| `posthog-client-eu-region` | yes | Network-mocked: capture the request URL, assert host is `eu.i.posthog.com` |
| `posthog-write-key-empty-no-op` | yes | Build with empty `-ldflags`, run a command, assert no HTTP requests issued |
| `usage-stats-channel-registered` | yes | After process start, query the parent's channel registry and assert `usage-stats` is present |
| `event-name-cli-command-completed` | yes | Network-mocked: capture the payload, assert `event: "cli.command.completed"` |
| `event-properties-all-ten` | yes | Network-mocked: capture the payload, assert all 10 keys present with the documented value sources |
| `caller-resolution-precedence` | yes | Matrix: flag vs env vs default; flag wins, env beats default, default `cli` when neither set |
| `caller-enum-coercion` | yes | Pass `--caller my-script`, capture the payload, assert `caller: "other"` |
| `posthog-funnel-defined` | no | Operational check: confirm the funnel exists in the PostHog UI before declaring v0.2.0 shipped. Documented in the release checklist. |
| `docs-usage-stats-sections-present` | yes | Static-doc structure check; markdown-lint over `docs/telemetry.md` |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| `cli/telemetry` (parent) | Provides every piece of shared plumbing this child uses: install_id, opt-out evaluation, first-run notice, PersistentPostRun hook, channel registry, transmission-timeout wrapper, fixed property-key enum, `docs/telemetry.md` skeleton. This child adds nothing to the parent's user-facing CLI surface except the global `--caller` flag. |
| `cli/telemetry/errors-telemetry` | Sibling. Coexists with this child as a second registered channel. Shares the install_id and opt-out plumbing. Does NOT share the PostHog client, the `--caller` flag, or any event. |
| `cli` (root) | Adds the `--caller` global flag to the root command. No other CLI command's behavior is affected. |

## Not Doing / Out of Scope

- **Custom PostHog backend / proxy.** Direct to PostHog cloud EU. Self-host is the parent's deferred decision; not relevant here.
- **Multiple PostHog projects** (e.g. one for staging, one for prod). Single project named `specscore-cli` in the PostHog EU region. Dev builds no-op via empty write key.
- **Event batching across invocations.** Each `specscore` invocation is a separate process; the PostHog SDK's intra-process batching is enough.
- **Sampling.** All events transmit (subject to opt-out and timeout). At expected volume (260k events/Q2), sampling adds complexity without saving cost.
- **A/B testing infrastructure** (PostHog feature flags). The product-analytics channel is read-only from PostHog's perspective; this Feature does not consume feature flags.
- **User-identification or pseudonymisation beyond `install_id`.** No email, no GitHub handle, no git config user.name.
- **Cohort definitions beyond the north-star funnel.** Additional funnels MAY be created operationally in the PostHog UI but are not part of this Feature's deliverable.
- **Real-time dashboards.** PostHog's funnel + insight UI is the reporting surface. No Grafana, no custom dashboard.
- **The `crash-reports` channel.** Owned by the `errors-telemetry` sibling Feature.

## Assumption Carryover

| Idea assumption | Status |
|---|---|
| The dev-tool audience tolerates opt-out telemetry when disclosure is loud and the schema is publicly auditable. | Inherited from parent. This Feature contributes to the audit surface via REQ:docs-usage-stats-section. |
| CLI invocations are a faithful proxy for "used it on a real spec." | **Encoded in this spec** via the `command = feature.create` filter in REQ:posthog-funnel-defined Step 2 and Step 3. Validated post-launch by spot-check per the Idea. |
| PostHog free tier (1M events/mo) covers Q2 volume. | **Encoded** as the implicit cap on event volume. Monitoring deferred to operational dashboards. |
| 500 ms hard timeout is invisible to users in the worst case. | Encoded in the parent (REQ:transmission-hard-timeout). This Feature inherits. |
| The `SPECSCORE_CALLER` env-var convention is sufficient to separate plugin-driven from raw-CLI usage. | **Encoded** in REQ:caller-env-var and REQ:caller-resolution. The `--caller` flag was added during Idea review as a higher-precedence channel. The Idea's enum (`cli`, `specstudio-skills`, `ci`, `script`, `agent`) was refactored during Feature review into a twenty-value agent-product-name enum (see REQ:caller-enum-known-values for the full list) for sharper PostHog segmentation. Validated by inspecting the share of `caller=other` events post-launch — a high `other` rate signals the enum is missing common agents. |
| EU-region PostHog is the right default. | **Encoded** as REQ:posthog-eu-region. |

## Acceptance Criteria

### AC: posthog-client-eu-region

**Requirements:** cli/telemetry/usage-telemetry#req:posthog-go-sdk, cli/telemetry/usage-telemetry#req:posthog-eu-region

**Given** a release build of `specscore` with a valid PostHog write key embedded AND telemetry enabled (no opt-out signals)
**When** `specscore --version` runs against a network where the PostHog SDK's outbound HTTPS request is captured by a proxy
**Then** the captured request's URL MUST have host `eu.i.posthog.com` (or the equivalent EU regional endpoint per the PostHog SDK's configuration); no request MUST be issued to `app.posthog.com`, `us.i.posthog.com`, or any non-EU endpoint.

### AC: posthog-write-key-empty-no-op

**Requirements:** cli/telemetry/usage-telemetry#req:posthog-write-key-embedded-at-build-time

**Given** a `specscore` binary built with empty `-ldflags` (no write key injected) AND telemetry enabled
**When** `specscore --version` runs against a proxy that captures all outbound HTTPS
**Then** no HTTPS request to any PostHog endpoint MUST be issued; `specscore telemetry status` MUST nonetheless report the `usage-stats` channel as `registered` (channel registration happens regardless of key presence).

### AC: usage-stats-channel-registered

**Requirements:** cli/telemetry/usage-telemetry#req:usage-stats-channel-registration

**Given** any build of `specscore` (release or dev)
**When** `specscore telemetry status` runs
**Then** stdout MUST contain a row for the channel `usage-stats` with its current enabled/disabled state and source. (The parent's AC:telemetry-subcommand-status already exercises the registry's unknown-channel rejection path; this AC only asserts that this child's registration is observable.)

### AC: event-name-and-properties

**Requirements:** cli/telemetry/usage-telemetry#req:usage-stats-event-name, cli/telemetry/usage-telemetry#req:usage-stats-event-properties

**Given** a release build with a valid PostHog write key, telemetry enabled, and a proxy capturing the outbound HTTPS payload
**When** `specscore feature list` runs (any successful command that exits `0`)
**Then** the captured PostHog `/capture/` request body MUST be a JSON object with `event: "cli.command.completed"`, `distinct_id: <a UUID v4 matching install_id>`, and `properties` containing exactly these ten keys (no more, no fewer): `command` (value `feature.list`), `success` (value `true`), `duration_ms` (positive integer), `exit_code` (value `0`), `cli_version` (matching `specscore --version` output), `os` (one of `darwin`/`linux`/`windows`), `arch` (one of `amd64`/`arm64`/etc.), `caller` (value `cli` in absence of flag/env), `install_id` (matching `distinct_id`), `is_first_run` (value `false` on a second invocation, `true` only when the install_id file was created on this invocation).

### AC: caller-resolution-precedence

**Requirements:** cli/telemetry/usage-telemetry#req:caller-flag, cli/telemetry/usage-telemetry#req:caller-env-var, cli/telemetry/usage-telemetry#req:caller-resolution

**Given** a release build, telemetry enabled, and a proxy capturing the payload
**When** the following invocations run:
- `specscore --caller claude --version`
- `SPECSCORE_CALLER=codex specscore --version`
- `SPECSCORE_CALLER=codex specscore --caller claude --version` (both set; flag wins)
- `specscore --version` (neither set; default)

**Then** the captured `caller` property MUST be:
- First invocation: `claude`
- Second invocation: `codex`
- Third invocation: `claude` (flag wins over env)
- Fourth invocation: `cli` (default)

### AC: caller-enum-coercion

**Requirements:** cli/telemetry/usage-telemetry#req:caller-enum-known-values

**Given** a release build, telemetry enabled, and a proxy capturing the payload
**When** `specscore --caller my-custom-tag --version` runs
**Then** the captured `caller` property MUST be the literal string `other`; the original value `my-custom-tag` MUST NOT appear anywhere in the request body. The command itself MUST exit `0` (the flag accepts arbitrary strings; only the transmitted value is constrained).

### AC: posthog-funnel-defined

**Requirements:** cli/telemetry/usage-telemetry#req:posthog-funnel-defined

**Given** v0.2.0 is being released
**When** the release checklist's "Telemetry verification" step is executed
**Then** the PostHog project named `specscore-cli` in the EU region MUST contain a funnel named exactly `North-Star: First Real Spec Use within 7 Days` with three steps as specified in REQ:posthog-funnel-defined, grouped by `distinct_id`. The check is manual and recorded by the release operator in the release notes.

### AC: docs-usage-stats-sections-present

**Requirements:** cli/telemetry/usage-telemetry#req:docs-usage-stats-section

**Given** a fresh checkout after this Feature is implemented
**When** the file `docs/telemetry.md` is opened
**Then** the file MUST contain a `### usage-stats` subsection inside `## Channels` with at least the literal strings `cli.command.completed` and `eu.i.posthog.com`; a `### usage-stats events` subsection inside `## Event Schema` enumerating all ten property keys AND enumerating the closed `caller` enum (the file MUST contain at least the literal strings `cli`, `claude`, `codex`, and `other` to demonstrate the enum is documented end-to-end); and a `### usage-stats` subsection inside `## Data Retention` referencing PostHog's retention defaults and the GDPR deletion endpoint.

## Open Questions

- **Exact wording of the `--caller` `--help` text.** REQ:caller-flag fixes the *content* (audience: AI coding agents; pointer to `docs/telemetry.md` for the enumerated values) but the prose is settled at Plan time during a copy-review pass.
- **Specstudio-skills sub-segmentation.** REQ:caller-enum-known-values folds the plugin's invocations into `caller=claude` and relies on command-pattern approximation to tell plugin users from raw-Claude users. If that approximation proves too lossy (e.g. raw-Claude users start running the same skill-shaped commands by hand), file a follow-up Idea adding a `caller_subtype` property or similar. Defer until post-W4 signal available.

> **Resolved during user review (2026-05-21):**
> - `pi` disambiguated to `pi.dev` (https://pi.dev/).
> - `antigravity` disambiguated to `antigravity.google` (https://antigravity.google/).
> - Enum expansion (was an OQ): all 8 previously-suggested agents — `devin`, `cline`, `roo`, `continue`, `windsurf`, `zed`, `amazon-q`, `tabnine` — are now in the enum. Total: 20 values including `cli` default and `other` coercion target.

---
*This document follows the https://specscore.md/feature-specification*
