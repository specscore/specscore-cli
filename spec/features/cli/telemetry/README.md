---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: Telemetry (CLI)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/telemetry?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/telemetry?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/telemetry?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/telemetry?op=request-change) |
**Status:** Stable
**Date:** 2026-05-21
**Owner:** alexandertrakhimenok
**Source Ideas:** cli-telemetry
**Supersedes:** —

## Summary

Shared plumbing for the `specscore` CLI's two telemetry channels — product analytics (usage) and crash reporting (errors). This Feature owns the parts that are identical across channels: the `internal/telemetry` package boundary, the privacy invariant ("no user-authored text leaves this binary"), the per-machine `install_id` lifecycle, the global opt-out subcommand surface, the first-run notice, and the `docs/telemetry.md` skeleton. Channel-specific wiring (PostHog event emission, Sentry crash transmission) is owned by child Features `cli/telemetry/usage-telemetry` and `cli/telemetry/errors-telemetry`, which are spec'd separately and depend on this parent existing.

## Contents

| Child | Description |
|---|---|
| [usage-telemetry](usage-telemetry/README.md) | The `usage-stats` channel — anonymous product-analytics events to PostHog (EU). Owns PostHog client wiring, event schema, `--caller` flag, and the north-star funnel. |
| [errors-telemetry](errors-telemetry/README.md) | The `crash-reports` channel — anonymous panic/exit-code-≥10 reports to Sentry (EU). Owns Sentry client wiring, stack-frame scrubber, `SafePanic` allowlist, and `specscore debug error` verification subcommand. |

## Problem

The 2026-Q2 GTM plan's north-star metric is unverifiable without our own instrumentation — Claude Code does not expose plugin install or invocation metrics to authors. Crash reporting from the first soft-post cohort is the highest-leverage debugging input of Q2. Both channels exist and ship in W1, but they share enough plumbing — scrubber, opt-out mechanics, first-run disclosure, `install_id`, the `specscore telemetry` command surface — that owning the shared pieces in a parent Feature is the only way to keep the children focused on their channel-specific deltas.

The parent's job is to encode the **invariants** ("no user-authored text," "opt-out always wins," "500 ms hard timeout") and the **surfaces** (file paths, subcommand layout, package boundary) once, so the children don't redefine them and so a reviewer auditing privacy posture has one place to read.

## Behavior

### Internal package boundary

Telemetry transmission is confined to a single Go package so the privacy invariant has exactly one audit surface.

#### REQ: internal-telemetry-package-location

The shared telemetry code MUST live at `internal/telemetry/` in the repo, importable as `github.com/specscore/specscore-cli/internal/telemetry`. Channel-specific files MAY be subfiles within this package (e.g. `internal/telemetry/usage.go`, `internal/telemetry/errors.go`); they MUST NOT be in sibling or parent packages. Go file naming inside `internal/telemetry/` is independent of the user-facing channel names — `usage.go` houses the `usage-stats` channel's transmission code, `errors.go` houses the `crash-reports` channel's. The user-facing name is whatever the channel registers via `RegisterChannel(name, ...)` (REQ:channel-registry).

#### REQ: vendor-sdk-import-confinement

Only files under `internal/telemetry/` MAY import the PostHog Go SDK, the Sentry Go SDK, or any other client whose purpose is to transmit data to an external telemetry endpoint. The repository MUST contain an automated check (a lint rule, a go-vet analyzer, or a test in `internal/telemetry/boundary_test.go`) that fails the build when a forbidden import appears outside `internal/telemetry/`.

#### REQ: fixed-event-property-keys

The package's public event-emission API MUST accept event properties only through a typed wrapper whose key set is a closed enum (Go constants or a typed string alias with a fixed set of values). The enum MUST include at least these ten keys: `command`, `success`, `duration_ms`, `exit_code`, `cli_version`, `os`, `arch`, `caller`, `install_id`, `is_first_run`. Child Features MAY extend the enum by amending this REQ and updating `docs/telemetry.md` accordingly. Arbitrary string-keyed maps MUST NOT be accepted from callers; adding a key requires a code change in this package.

#### REQ: transmission-hard-timeout

Every external transmission path (event emit, error capture, flush-on-exit) MUST be bounded by a 500 ms hard timeout. On timeout, the in-flight transmission MUST be dropped silently — no retry, no panic, no error returned to the caller. The CLI's user-visible behavior MUST be identical whether the telemetry endpoint is reachable, timing out, or returning 5xx.

### Install ID lifecycle

The CLI identifies an installation (not a user, not a project) via a per-machine UUID generated on first run.

#### REQ: install-id-file-path

The install ID MUST be stored at `~/.specscore/install_id` on Unix-like systems and the platform-appropriate user state directory equivalent on others (e.g. `%LOCALAPPDATA%\specscore\install_id` on Windows). The directory MUST be created with mode `0700` if it does not exist; the file MUST be created with mode `0600`.

#### REQ: install-id-creation

When `install_id` does not exist at the start of an invocation, the CLI MUST create it containing a freshly-generated UUID v4 (lowercase, hyphenated, no surrounding whitespace, no trailing newline beyond a single `\n`). Creation MUST be atomic: write to a temporary file in the same directory and rename, so a concurrent read never observes a partial file.

#### REQ: install-id-immutability

Once created, the `install_id` file MUST NOT be regenerated, rotated, or overwritten by any CLI command in normal operation. Users who want a fresh install identity MUST do so by deleting the file manually; the CLI MUST NOT expose a `regenerate` or `reset` subcommand.

#### REQ: install-id-scope

The install ID is per-machine and per-user, NOT per-project. The CLI MUST NOT read or write any project-local install identifier. A single developer working across N repositories MUST count as one install.

### Opt-out mechanics

Opt-out is the central privacy contract. The parent owns the precedence rules; the children inherit them.

#### REQ: opt-out-signal-precedence

Before any telemetry transmission (event or error), the CLI MUST evaluate opt-out signals in this order, stopping at the first positive signal:

1. The `--no-telemetry` global flag on the current invocation.
2. Environment variables: `SPECSCORE_TELEMETRY=0`, `DO_NOT_TRACK=1`.
3. CI auto-disable: any of `CI=true`, `GITHUB_ACTIONS=true`, `GITLAB_CI=true`, `BUILDKITE=true`, `CIRCLECI=true` (case-sensitive value match).
4. Persistent state from `~/.specscore/telemetry.yaml` (global `enabled: false` OR a per-channel override `usage-stats: false` / `crash-reports: false`).

Any positive signal MUST disable the relevant channel for the current invocation. No subsequent signal can re-enable it.

#### REQ: opt-out-always-wins

A local opt-out (any signal in REQ:opt-out-signal-precedence) MUST override any `--caller` value, `SPECSCORE_CALLER` env var, or other invocation context. A plugin, script, or AI agent invoking the CLI MUST NOT be able to bypass a user's opt-out by setting `--caller` or any other parameter.

#### REQ: telemetry-subcommand-surface

The CLI MUST provide a `specscore telemetry` root subcommand with three verbs, each taking an optional positional `[channel]` argument:

| Invocation | Effect |
|---|---|
| `specscore telemetry status` | Print, to stdout, the current opt-in/out state for every registered channel and the source of each setting (flag / env var / CI / persistent state / default). Exit `0`. |
| `specscore telemetry status <channel>` | Print state for the named channel only. Unknown channel name exits `2` with a message listing the known channels. |
| `specscore telemetry enable [channel]` | With no channel OR with the literal `all` channel argument: write `enabled: true` to `~/.specscore/telemetry.yaml` (applies to all channels). With a real channel name: write the per-channel `<channel>: true` override. Print confirmation. Exit `0`. |
| `specscore telemetry disable [channel]` | Symmetric to `enable`. With no channel OR `all`: global `enabled: false`. With a real channel name: per-channel `<channel>: false`. |

Known channel names are owned by the channel registry — see REQ:channel-registry. The MVP registry contains exactly two: `usage-stats` and `crash-reports`. The literal string `all` is reserved as the explicit "all channels" sentinel and MUST NOT be a registered channel name (chosen over `*` because `*` is shell-glob-expanded in interactive use and would require users to quote it, an avoidable fragility). The parent Feature owns all subcommand parsing, dispatch, and persistent-state writes; child Features do NOT add their own cobra subcommands — they register a channel name and a transmission callback with the parent.

#### REQ: channel-registry

The parent Feature MUST expose a function `internal/telemetry.RegisterChannel(name, transmit)` that child Features call once at package init time. `name` MUST be a stable user-facing identifier (the MVP set is `usage-stats` for product analytics and `crash-reports` for error reporting; future channels MAY be added by amending this REQ). The registry MUST be the single source of truth that `specscore telemetry status|enable|disable` consults to enumerate channels — REQ:telemetry-subcommand-surface MUST NOT hardcode channel names anywhere except in this REQ. Attempting to register an unknown channel name (i.e. one not enumerated in this REQ) MUST fail the build (Go `init()` panic is acceptable since it surfaces at startup, not at runtime).

Adding a new channel is intentionally a two-place edit: amending this REQ's enumerated list AND adding the Go constant. The duplication is the audit surface — a privacy reviewer reading only this REQ sees every channel that exists. Future maintainers MUST NOT collapse the two by generating the REQ list from the Go enum or vice versa.

#### REQ: persistent-state-file-shape

`~/.specscore/telemetry.yaml` MUST conform to this shape. Writes that would produce a non-conforming file (e.g. a future `--option` flag we don't yet support) MUST fail at write-time with a stderr message naming the file. Reads of a non-conforming file (e.g. a hand-edited file with unknown keys) MUST NOT abort the user's command; instead, the CLI MUST emit a one-line stderr warning naming the file and the offending key, disable telemetry for the invocation, and proceed. The shape:

```yaml
# SpecScore Telemetry Preferences: https://specscore.md/telemetry-preferences
enabled: true | false              # global; affects all channels
usage-stats: true | false          # optional per-channel override; absent ⇒ inherit `enabled`
crash-reports: true | false        # optional per-channel override; absent ⇒ inherit `enabled`
```

Per-channel keys MUST be drawn from the channel registry (REQ:channel-registry). The file MUST NOT contain any other keys. The CLI MUST NOT auto-create this file on first run (the absence of the file means "no persistent preference set" — the default-opt-in defaults apply).

### First-run notice

Disclosure happens once, on the very first invocation, before any meaningful command runs.

#### REQ: first-run-notice-trigger

When the `install_id` file does not exist at the start of an invocation AND none of the auto-disable signals from REQ:opt-out-signal-precedence steps 1–3 are present, the CLI MUST print the first-run notice (see REQ:first-run-notice-content) to stderr before executing the requested command. The notice MUST be printed exactly once per machine (subsequent invocations see the existing `install_id` and skip the notice). The notice MUST NOT block on input — execution proceeds immediately after printing.

#### REQ: first-run-notice-content

The notice MUST contain these three blocks, in this order:

1. **Intro line:** a single sentence stating that **SpecScore** (proper-noun capitalization) collects two anonymous telemetry streams and naming the data residency (EU-hosted).
2. **Channel list:** a bullet list with exactly two items, in this order:
   - the `usage-stats` channel with a brief parenthetical naming the vendor/role (e.g. "PostHog product analytics")
   - the `crash-reports` channel with a brief parenthetical naming the vendor (e.g. "Sentry")
3. **Disable instruction:** a single line naming the literal command form `specscore telemetry disable [channel-id]` and explaining that omitting the channel argument OR passing the literal `all` disables all telemetry. The notice MUST surface the channel-id placeholder; it MUST NOT enumerate the per-channel disable commands inline (the placeholder form is the canonical surface that scales as new channels are added).

The exact wording MAY be tuned at implementation time, but the following are normative:
- The intro line, the channel list, and the disable line in that order.
- The proper-noun "SpecScore" capitalization.
- The two channel names `usage-stats` and `crash-reports` appear by their exact identifiers.
- The literal string `specscore telemetry disable [channel-id]` appears.
- The literal string `all` appears as the explicit "all channels" sentinel.

Deliberately NOT in the notice (still findable via `--help`, README, and `docs/telemetry.md`):
- A link to `docs/telemetry.md` (REQ:docs-telemetry-md-skeleton still requires the doc to exist; users reach it from elsewhere).
- The `SPECSCORE_TELEMETRY=0` env-var quick-disable form (still supported per REQ:opt-out-signal-precedence; not surfaced on first contact to keep the notice focused on the per-command disable).

#### REQ: first-run-notice-ci-suppression

When any auto-disable signal from REQ:opt-out-signal-precedence steps 1–3 is present, the first-run notice MUST NOT be printed, even when `install_id` does not exist. The `install_id` file MUST still be created (so the same machine, run interactively later, also does not see the notice — CI-first developers should not be re-notified once they sit at their own terminal).

### Documentation surface

The repo carries a single human-readable telemetry document at a stable location.

#### REQ: docs-telemetry-md-skeleton

The repository MUST contain `docs/telemetry.md` with the following top-level sections, in this order: `Overview`, `Channels`, `Event Schema`, `What We Don't Collect`, `Opt-out`, `Data Retention`. The `Channels` section MUST contain one third-level subsection per active telemetry channel; for the MVP these are `### usage-stats` and `### crash-reports`, populated by the corresponding child Features. The parent Feature owns the skeleton (headings present, intro paragraphs, `What We Don't Collect` and `Opt-out` content); children populate their `Channels.<channel-name>` subsections and add per-channel entries to `Event Schema` and `Data Retention`.

#### REQ: docs-telemetry-md-no-collect-invariant

The `What We Don't Collect` section of `docs/telemetry.md` MUST list the invariant set: spec content, feature names, file paths, project paths, git remotes, hostnames, usernames, and any string the user typed as input to a CLI command. This list MUST match the package-boundary invariant enforced by REQ:vendor-sdk-import-confinement and REQ:fixed-event-property-keys.

## Architecture & Components

| Unit | Responsibility | Used by | Depends on |
|---|---|---|---|
| `internal/telemetry/` (Go package) | The only place external telemetry SDKs are imported. Owns the typed event surface, the install_id helpers, the opt-out evaluator, and the 500ms-timeout wrapper. | `internal/cli/root.go` for opt-out evaluation; child-feature files (`usage.go`, `errors.go`) for transmission. | PostHog / Sentry SDKs (used only from `usage.go` / `errors.go` respectively). |
| `internal/telemetry/optout.go` | Implements REQ:opt-out-signal-precedence. Pure function: signals in → channel enabled/disabled state out. | Both channel files; the `specscore telemetry status` command. | OS env, the persistent-state reader. |
| `internal/telemetry/installid.go` | Manages `~/.specscore/install_id` creation and read. Per-machine, atomic write. | All telemetry calls (to tag events); the first-run-notice trigger. | OS user-config-dir resolver. |
| `internal/telemetry/state.go` | Reads/writes `~/.specscore/telemetry.yaml`. Validates shape per REQ:persistent-state-file-shape. | The opt-out evaluator; the `specscore telemetry {enable,disable}` subcommands. | YAML library. |
| `internal/cli/telemetry.go` | Owns all `specscore telemetry <verb> [channel]` parsing and dispatch (positional channel arg). Consults the channel registry (REQ:channel-registry) for the known-channel set on every invocation. Children do NOT attach cobra subcommands here — they call `internal/telemetry.RegisterChannel(...)` at package init time. | cobra root. | `internal/telemetry`. |
| `internal/cli/root.go` (modified) | PersistentPreRun: evaluates opt-out, creates install_id on first run, prints first-run notice. PersistentPostRun: emits event via the `usage-stats` channel transmit-fn and flushes (timeout-bounded). | All commands. | `internal/telemetry`. |
| `docs/telemetry.md` | Human-readable, single page covering all channels. Stable URL referenced from the first-run notice. | Users; the first-run notice. | This Feature plus the two child Features. |
| Boundary check | A go-vet-style check OR a test in `internal/telemetry/boundary_test.go` that fails the build when forbidden imports appear outside `internal/telemetry/`. | CI. | None — runs from the repo's existing test pipeline. |

## Data Flow

```
specscore <cmd> [args]
  │
  ├─→ PersistentPreRun
  │     1. Resolve opt-out (optout.go) — produces per-channel enabled state
  │     2. Read or create install_id (installid.go); on creation, maybe print first-run notice
  │     3. If global opt-out: skip all subsequent telemetry hooks
  │
  ├─→ <cmd> Run() — the actual command logic (unaffected by telemetry)
  │
  └─→ PersistentPostRun
        1. If `usage-stats` channel enabled: emit event via internal/telemetry/usage.go (PostHog, 500ms timeout)
        2. If `crash-reports` channel enabled AND command exited ≥10 or panicked: emit error via internal/telemetry/errors.go (Sentry, 500ms timeout)
        3. Flush — also bounded by 500ms; remaining events are dropped silently
```

The `--caller` flag and `SPECSCORE_CALLER` env var (owned by the `usage-telemetry` child Feature, which implements the `usage-stats` channel) are read in step 1 of PostRun, not affected by the parent.

## Error Handling & Failure Modes

| Failure | Behavior |
|---|---|
| Telemetry endpoint unreachable / 5xx / DNS failure | Caught by REQ:transmission-hard-timeout. Event/error dropped. No user-visible side effect. |
| `~/.specscore/` cannot be created (permission denied, read-only filesystem) | Disable telemetry for this invocation silently; do NOT abort the user's command. Log to stderr at `--verbose` only. |
| `~/.specscore/install_id` contains malformed content (not a UUID) | Treat as missing: do NOT regenerate (REQ:install-id-immutability). Disable telemetry for this invocation and log to stderr at `--verbose`. The user has been editing the file intentionally; respect that. |
| `~/.specscore/telemetry.yaml` has unknown keys or wrong shape | Refuse to read. Disable telemetry for this invocation. Print a one-line stderr warning naming the file and suggesting `specscore telemetry status` to re-set preferences. Do NOT abort the user's command. |
| Conflicting opt-out signals (e.g. `--no-telemetry` flag with `enabled: true` persisted) | REQ:opt-out-signal-precedence resolves deterministically: first positive signal wins, opt-out always wins on tie. |
| First-run notice writes to a pipe / non-TTY stderr | The notice MUST still be printed (REQ:first-run-notice-trigger). Whether it's read is the user's problem; absence of TTY is not a suppression signal. |

## Testing Strategy

Per-AC Rehearse stubs MAY be scaffolded for the testable ACs (file-presence, exit-code, env-var-driven behavior). Stubs are not scaffolded for the documentation-only AC (`docs-telemetry-md-skeleton` is verifiable by lint over the markdown file but does not have an interesting Rehearse runtime surface). Recorded under `## Rehearse Integration` below.

## Rehearse Integration

| AC | Stub? | Rationale |
|---|---|---|
| `install-id-lifecycle` | yes | CLI surface, file system state — testable |
| `opt-out-precedence` | yes | Pure-function-ish; matrix of env-var + flag combinations |
| `telemetry-subcommand-status` | yes | CLI surface, stdout snapshot testable |
| `first-run-notice-shown-once` | yes | Stderr snapshot + `install_id` file presence |
| `first-run-notice-suppressed-in-ci` | yes | Env-var-driven |
| `vendor-sdk-import-confinement` | yes | Build-time check; runs as a Go test or go-vet step |
| `transmission-hard-timeout` | yes | Network-mocked; verify p99 < timeout under blackhole conditions |
| `docs-telemetry-md-skeleton-present` | no | Static doc structure; covered by lint over the markdown file |
| `persistent-state-file-shape-rejected` | yes | Write a malformed file, run `specscore telemetry status`, assert stderr warning + telemetry disabled for the invocation |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [`cli/init`](../init/README.md) | `specscore init` does NOT pre-create `~/.specscore/` — that's this Feature's first-run-notice trigger's responsibility. `init` MAY print a hint pointing at `docs/telemetry.md`; the canonical disclosure remains the first-run notice. |
| [`cli`](../README.md) (root) | Inherits the global `--no-telemetry` flag and the PersistentPreRun/PostRun wiring. The root command's exit-code contract is unaffected. |

## Not Doing / Out of Scope

- **PostHog or Sentry client wiring.** Lives in the child Features (`cli/telemetry/usage-telemetry/` implements the `usage-stats` channel; `cli/telemetry/errors-telemetry/` implements the `crash-reports` channel).
- **`--caller` flag, `SPECSCORE_CALLER` env var.** Owned by the `usage-telemetry` child Feature (implements the `usage-stats` channel).
- **`telemetry.SafePanic` wrapper and panic-site retrofit.** Owned by the `errors-telemetry` child Feature (implements the `crash-reports` channel).
- **`specscore debug error` subcommand.** Owned by the `errors-telemetry` child Feature.
- **Self-hosted telemetry backend.** Q3+ decision per the parent Idea.
- **Per-event sampling, quota management, rate limiting.** Out of MVP; the 500ms timeout + free-tier quotas suffice at expected Q2 volume.
- **Telemetry preferences UI in `specscore.studio`.** This Feature is CLI-only.

## Assumption Carryover

| Idea assumption | Status after this spec |
|---|---|
| The dev-tool audience tolerates opt-out telemetry when disclosure is loud and the schema is publicly auditable. | Carried forward. Validated post-launch per the Idea's threshold. |
| CLI invocations are a faithful proxy for "used it on a real spec." | Inherited by the `usage-telemetry` child Feature (the `usage-stats` channel); this parent doesn't carry product-analytics semantics. |
| PostHog free tier covers Q2 volume. | Inherited by the `usage-telemetry` child Feature (the `usage-stats` channel); not this Feature. |
| 500ms hard timeout is invisible to users in the worst case. | **Encoded in this spec** as REQ:transmission-hard-timeout. Validated by an AC. |
| `SPECSCORE_CALLER` env var is sufficient to separate plugin-driven from raw-CLI usage. | Inherited by the `usage-telemetry` child Feature (the `usage-stats` channel); this parent reserves the flag namespace via REQ:telemetry-subcommand-surface but does not implement it. |
| EU-region PostHog is the right default. | Inherited by the `usage-telemetry` child Feature (the `usage-stats` channel). |

## Acceptance Criteria

### AC: internal-telemetry-package-location-fixed

**Requirements:** cli/telemetry#req:internal-telemetry-package-location

**Given** a fresh checkout of the repository
**When** `find . -type d -name telemetry -path '*/internal/*'` runs from the repo root
**Then** the result contains exactly the path `./internal/telemetry`; no other `internal/.../telemetry` directory exists; and `go list ./internal/telemetry` succeeds (the package compiles).

### AC: vendor-sdk-import-confinement-enforced

**Requirements:** cli/telemetry#req:vendor-sdk-import-confinement

**Given** a candidate change that adds `import "github.com/posthog/posthog-go"` to a file in `internal/cli/feature.go`
**When** the repository's CI pipeline runs (lint + tests + build)
**Then** the build MUST fail with a clear error naming the offending file, the forbidden import, and the rule that forbids it (e.g. `vendor-sdk-import-confinement: internal/cli/feature.go imports github.com/posthog/posthog-go; only files under internal/telemetry/ may import telemetry vendor SDKs`).

### AC: fixed-event-property-keys-enforced-at-compile-time

**Requirements:** cli/telemetry#req:fixed-event-property-keys

**Given** caller code attempts to invoke `telemetry.Emit(ctx, map[string]any{"arbitrary": "value"})` from `internal/cli/`
**When** `go build` runs
**Then** the build MUST fail with a type error (the package's public API does not accept a `map[string]any`); the only way to attach properties MUST be via the closed-enum typed wrapper exposed by the package.

### AC: transmission-hard-timeout-bounded

**Requirements:** cli/telemetry#req:transmission-hard-timeout

**Given** the PostHog and Sentry endpoints are configured to point at a blackhole address (TCP connections accepted, no response)
**When** `specscore --version` runs 100 times in sequence with both telemetry channels enabled
**Then** the p99 wall-clock duration of `specscore --version` MUST be under 550 ms (500 ms timeout + 50 ms slack for non-telemetry CLI overhead); no event or error MUST be retried; no user-visible error MUST be printed about telemetry.

### AC: install-id-lifecycle

**Requirements:** cli/telemetry#req:install-id-file-path, cli/telemetry#req:install-id-creation, cli/telemetry#req:install-id-immutability, cli/telemetry#req:install-id-scope

**Given** a clean home directory with no `~/.specscore/` directory and no auto-disable env vars set
**When** `specscore --version` runs once, then `specscore feature list` runs from a different project root
**Then** after the first run: `~/.specscore/` exists with mode `0700`; `~/.specscore/install_id` exists with mode `0600` and contains exactly one UUID v4 followed by `\n`. After the second run from a different project: the same `install_id` file content is byte-identical to the first run (per-machine, not per-project; never regenerated). If the user runs `rm ~/.specscore/install_id` and re-runs `specscore --version`, a *new* UUID is generated — the CLI provides no in-band rotation, but file deletion is an out-of-band rotation users are entitled to.

### AC: opt-out-precedence

**Requirements:** cli/telemetry#req:opt-out-signal-precedence, cli/telemetry#req:opt-out-always-wins

**Given** a system where `~/.specscore/telemetry.yaml` contains `enabled: true` and `SPECSCORE_CALLER=claude`
**When** the following invocations run:
- `SPECSCORE_TELEMETRY=0 specscore --version` (env-var opt-out)
- `specscore --no-telemetry --version` (flag opt-out)
- `CI=true specscore --version` (CI auto-disable)
- `DO_NOT_TRACK=1 specscore --version`
- `specscore --version` (no opt-out signals; persistent `enabled: true`)
- `SPECSCORE_TELEMETRY=0 SPECSCORE_CALLER=claude specscore --version` (agent caller attempting to override opt-out)

**Then** for invocations 1–4 and 6: no telemetry transmission occurs (verifiable via a blackhole endpoint + transmission counter); the `--caller` value MUST NOT re-enable transmission in invocation 6. For invocation 5: transmission is attempted (subject to the 500 ms timeout).

### AC: telemetry-subcommand-status

**Requirements:** cli/telemetry#req:telemetry-subcommand-surface, cli/telemetry#req:channel-registry

**Given** a system with no `~/.specscore/telemetry.yaml` and no auto-disable env vars
**When** `specscore telemetry status` runs, then `specscore telemetry status usage-stats` runs, then `specscore telemetry status unknown-channel` runs
**Then** the first invocation's stdout MUST list both registered channels (`usage-stats`, `crash-reports`), each with current enabled/disabled state and the source of that state (e.g. `usage-stats: enabled (default; no persistent preference)`); exit code `0`. The second invocation MUST print only the `usage-stats` row; exit `0`. The third invocation MUST exit `2` with a stderr message listing the known channel names. The output format MUST be stable enough for `grep`-based scripting (one channel per line, the channel name as the first column).

### AC: telemetry-subcommand-enable-disable

**Requirements:** cli/telemetry#req:telemetry-subcommand-surface, cli/telemetry#req:persistent-state-file-shape

**Given** a system with no `~/.specscore/telemetry.yaml`
**When** `specscore telemetry disable` runs (no channel arg), then `specscore telemetry status` runs, then `specscore telemetry enable crash-reports` runs (channel positional), then `specscore telemetry status` runs again, then `specscore telemetry disable all` runs (the all-channels sentinel)
**Then** after the no-arg disable: `~/.specscore/telemetry.yaml` exists, contains the canonical schema-pointer comment on line 1, and `enabled: false`; the subsequent `status` reports both channels disabled with source `persistent state`. After the per-channel enable: the file now contains `enabled: false` AND `crash-reports: true`; the subsequent `status` reports `usage-stats: disabled` and `crash-reports: enabled` (per-channel override beats global). After `disable all`: the effect MUST be identical to a no-arg disable — `enabled: false` written, any prior per-channel `crash-reports` override preserved or cleared per the parent's write contract (implementation MAY clear per-channel overrides on an `all` operation; current behavior is to leave them). The file MUST contain no keys other than those documented in REQ:persistent-state-file-shape.

### AC: first-run-notice-shown-once

**Requirements:** cli/telemetry#req:first-run-notice-trigger, cli/telemetry#req:first-run-notice-content

**Given** a clean home directory with no `~/.specscore/install_id`, no auto-disable env vars, and stderr captured
**When** `specscore --version` runs once, then runs a second time
**Then** the first invocation's stderr MUST contain the first-run notice per REQ:first-run-notice-content, specifically: the literal proper noun `SpecScore` appears; the literal channel identifiers `usage-stats` and `crash-reports` both appear; the literal string `specscore telemetry disable [channel-id]` appears; the literal `all` appears as the all-channels sentinel. The second invocation's stderr MUST NOT contain any of those literal strings (the install_id now exists; notice is suppressed).

### AC: first-run-notice-suppressed-in-ci

**Requirements:** cli/telemetry#req:first-run-notice-ci-suppression

**Given** a clean home directory with no `~/.specscore/install_id` and `CI=true` in the environment
**When** `specscore --version` runs
**Then** stderr MUST NOT contain the first-run notice; `~/.specscore/install_id` MUST nonetheless be created (so a later interactive run on the same machine does not re-trigger the notice).

### AC: persistent-state-file-shape-rejected

**Requirements:** cli/telemetry#req:persistent-state-file-shape

**Given** `~/.specscore/telemetry.yaml` contains an unexpected key (e.g. `analytics_provider: posthog`)
**When** `specscore telemetry status` runs
**Then** stderr MUST contain a one-line warning naming the file and the unknown key; stdout MUST report both channels as disabled with source `invalid persistent state — see stderr`; exit code MUST be `0` (the user's command — querying status — succeeds; the malformed file is reported but not fatal).

### AC: docs-telemetry-md-skeleton-present

**Requirements:** cli/telemetry#req:docs-telemetry-md-skeleton, cli/telemetry#req:docs-telemetry-md-no-collect-invariant

**Given** a fresh checkout after this Feature is implemented
**When** the file `docs/telemetry.md` is opened
**Then** the file MUST contain the following second-level headings, in order: `## Overview`, `## Channels`, `## Event Schema`, `## What We Don't Collect`, `## Opt-out`, `## Data Retention`. The `## What We Don't Collect` section MUST enumerate at least: spec content, feature names, file paths, project paths, git remotes, hostnames, usernames, and arbitrary user input. The `## Channels` section MUST contain third-level subsections `### usage-stats` and `### crash-reports` even if their content is initially populated by the child Features.

## Open Questions

- **Exact first-run-notice prose.** REQ:first-run-notice-content fixes the three-line shape, the proper-noun "SpecScore" capitalization, the two channel names (`usage-stats`, `crash-reports`), and the literal command/env-var strings that MUST appear. The prose connective tissue gets a copy-review at Plan time before v0.2.0 ships.

> **Resolved during user review (2026-05-21):**
> - Boundary-check implementation → `internal/telemetry/boundary_test.go` (Go test, walks AST, fails build). REQ:vendor-sdk-import-confinement still permits the other two forms; the Plan locks `_test.go`.
> - Subcommand surface → `specscore telemetry <verb> [channel]` (positional channel arg), not `specscore telemetry <channel> <verb>`. Ownership moves from children to parent; children register via `RegisterChannel(name, transmit)`.
> - Channel names → `usage-stats` (was `usage`) and `crash-reports` (was `errors`). Feature directory slugs (`usage-telemetry`, `errors-telemetry`) stay as-is; the mapping is documented in this Feature's Not Doing section and the child Features' top-of-file.
> - First-run notice line 3 → MUST surface per-channel disable commands, not only the global form. Global `specscore telemetry disable` MAY be mentioned but MUST NOT replace per-channel.
> - Malformed-persistent-state `status` behavior → keep the unix split: stdout shows the channel table with `source: invalid persistent state — see stderr`, stderr prints the diagnostic warning, exit `0`. AC `persistent-state-file-shape-rejected` stands unchanged.

---
*This document follows the https://specscore.md/feature-specification*
