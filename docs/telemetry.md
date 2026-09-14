# SpecScore CLI Telemetry

> Canonical reference: <https://specscore.md/cli/telemetry>

This document describes what the `specscore` CLI sends to external services,
where it goes, what it does not collect, and how to disable it.

## Overview

`specscore` collects two distinct kinds of anonymous data so the maintainers
can find bugs and learn what is used:

- **`usage-stats`** — product analytics. One event per CLI invocation
  describing *what command ran* and *whether it succeeded*.
- **`crash-reports`** — error reporting. One event when the CLI panics or
  exits with an unexpected error code (≥10), describing *what broke* with a
  scrubbed stack trace.

Both channels are **opt-out**, **EU-hosted**, and **independent** — you can
disable one without disabling the other.

## Channels

### usage-stats

Anonymous product analytics on every `specscore` CLI invocation. The channel
sends one PostHog event per command, named **`cli.command.completed`**, to
the **PostHog EU region** (`eu.i.posthog.com`). The transport is the official
PostHog Go SDK in batched-async mode, bounded by the parent-Feature's 500 ms
hard timeout — a slow or unreachable endpoint never blocks your command.

Each event carries the closed-enum 10-property payload defined in
[Event Schema → usage-stats events](#usage-stats-events) below. The PostHog
`distinct_id` is set to your `install_id`, so PostHog groups events per
machine. Two machines belonging to the same human appear as two installs;
that's a deliberate trade — we don't correlate across machines, ever.

What we measure with this channel: how many people install, which commands
they run, whether commands succeed, how long they take, and which AI agents
(if any) drive the CLI. We do **not** measure: which files you act on, what
they contain, who you are, or where you live beyond the country-level
inference PostHog itself does from your IP for its own infrastructure (we
do not query that field).

PostHog's data-handling policy:
<https://posthog.com/privacy>.

### crash-reports

Anonymous error reporting for crashes and unexpected error paths. The channel
sends a Sentry event **only when**:

- The CLI panics (any `panic()`, including nil-dereferences and other
  runtime panics), OR
- A command exits with code **≥10** (the documented "unexpected" code
  per `pkg/exitcode`; documented expected errors with codes 1–9 are
  never reported).

Transmission goes to the **Sentry EU region** (`*.ingest.de.sentry.io`).
The transport is the official Sentry Go SDK, configured with
`AttachStacktrace=true` and **`SendDefaultPII=false`** so the SDK never
auto-collects OS user, hostname, or local-variable values from your
environment. Every event is bounded by the parent-Feature's 500 ms hard
timeout — a slow or unreachable endpoint never blocks your command.

A defensive `defer recover()` inside the transmit callback guarantees
that a bug in the crash-reporting code itself (scrubber bug, SDK panic,
malformed payload) cannot mask your command's exit code. If telemetry
fails, your invocation still exits with whatever code your command
intended.

Events carry the `release` tag (= your `cli_version`) so Sentry alerts
can filter by release. They also carry `debug=true` when emitted via
`specscore debug error` — operators filter these out of paging alerts.

Sentry's data-handling policy:
<https://sentry.io/security/>.

## Event Schema

### usage-stats events

<!-- Per cli/telemetry/usage-telemetry#req:docs-usage-stats-section, this
     subsection MUST enumerate all 10 property keys (matching
     REQ:usage-stats-event-properties) AND the full 20-value caller enum
     (matching REQ:caller-enum-known-values) end-to-end. -->

Each `cli.command.completed` event will carry these ten properties (the closed
enum enforced at compile time by `internal/telemetry/telemetry.go`):

| Property | Value source |
|---|---|
| `command` | Cobra command path, dot-separated (e.g. `feature.create`). |
| `success` | `true` iff exit code is `0`. |
| `duration_ms` | Wall-clock duration, integer milliseconds. |
| `exit_code` | Integer exit code returned by the command. |
| `cli_version` | The version string from `specscore --version`. |
| `os` | `runtime.GOOS` (`darwin`, `linux`, `windows`). |
| `arch` | `runtime.GOARCH` (`amd64`, `arm64`, …). |
| `caller` | The AI-agent identifier (see enum below). |
| `install_id` | Per-machine UUID v4 (also used as PostHog `distinct_id`). |
| `is_first_run` | `true` iff the install_id file was just created. |

The `caller` field is a closed enum populated by the agent driving the CLI
(or `cli` by default for human-typed invocations):

| Value | Agent |
|---|---|
| `cli` | Default — human at a terminal, no agent set. |
| `claude` | Claude Code (Anthropic). Includes invocations via `specstudio-skills`. |
| `codex` | Codex CLI (OpenAI). |
| `aider` | Aider — open-source AI pair programmer. |
| `opencode` | OpenCode — open-source AI coding agent. |
| `goose` | Goose (Block / Square). |
| `cursor` | Cursor (Anysphere). |
| `gemini` | Gemini CLI (Google). |
| `copilot` | GitHub Copilot CLI. |
| `devin` | Devin (Cognition Labs). |
| `cline` | Cline — open-source VS Code AI agent. |
| `roo` | Roo Code — Cline fork. |
| `continue` | Continue — open-source VS Code / JetBrains AI assistant. |
| `windsurf` | Windsurf — Codeium's AI-native IDE. |
| `zed` | Zed — collaborative editor with AI mode. |
| `amazon-q` | Amazon Q Developer (AWS). |
| `tabnine` | Tabnine. |
| `deepseek` | DeepSeek Harness (DeepSeek). |
| `pi.dev` | Pi (https://pi.dev/). |
| `antigravity.google` | Antigravity (https://antigravity.google/). |
| `other` | Catch-all for any unrecognized value. |

To set a caller, pass `--caller <value>` to any `specscore` invocation, or set
`SPECSCORE_CALLER=<value>` in the environment. The flag takes precedence over
the env var. Unrecognized values are coerced to `other` before transmission
— the flag itself accepts any string so scripts cannot accidentally fail.

### crash-reports events

Each Sentry event carries:

- **`message` field** — one of:
  - A literal `messageID` string from the SafePanic closed-enum allowlist
    (when the panic was wrapped via `telemetry.SafePanic("known-id", err)`
    AND `known-id` is in the production allowlist).
  - The literal string **`unscrubbed panic`** otherwise (plain string
    panics, unwrapped errors, runtime panics, unknown messageIDs). When
    this case applies, the event ALSO carries the tag
    **`message: unscrubbed`** so operators can grep for these and decide
    which sites to retrofit with `SafePanic`.
  - For exit-code-≥10 events: the synthesized string
    `unexpected exit code <N> from cmd <dot.path>` where `<N>` is the
    integer exit code and `<dot.path>` is the cobra command path. Both
    are by construction free of user-authored content.

- **Stack frames** (when `AttachStacktrace=true` produced any). Each
  frame's file path is replaced with its basename only (`feature.go`,
  not `/Users/alice/.../feature.go`). Function name and line number are
  preserved verbatim. Local-variable values are NEVER attached.

- **Tags**:
  - `release` — your `cli_version`.
  - `debug` — `"false"` for real crashes; `"true"` for `specscore debug
    error` invocations.
  - `message` — `"unscrubbed"` only when the panic value wasn't an
    allowlisted SafePanic.

### `specscore debug error` interpretation contract

`specscore debug error --text "<msg>" [--force]` synthesizes one
crash-reports event for pipeline verification. The `--text` value is
treated as a **candidate SafePanic messageID** — not as free-form
prose. The scrubber decides:

- If `<msg>` is in the SafePanic allowlist, it ships verbatim as the
  Sentry event's `message`.
- Otherwise it coerces to `unscrubbed panic` with the
  `message: unscrubbed` tag.

Without `--force`, the command honors the crash-reports opt-out (no
event sent, no-op message to stdout). With `--force`, opt-out is
bypassed for a single invocation; the persistent
`~/.specscore/telemetry.yaml` is byte-identical before and after.

## What We Don't Collect

The `internal/telemetry/` Go package is the **only** place in the codebase
where data may leave the binary. A `_test.go`-based boundary check fails the
build if any other file imports a telemetry vendor SDK. Independently, the
public `telemetry.Emit(ctx, Event)` function accepts only a fixed-field
struct — `map[string]any` is a compile-time type error.

This means the following will **never** appear in any telemetry event:

- **Spec content** — the body of any `.md` file in your spec tree.
- **Feature names** — the slugs in `spec/features/<slug>/` (the `command`
  property carries the cobra command, e.g. `feature.create`, NOT the feature
  slug being acted upon).
- **File paths** — absolute paths to your projects (the crash-reports
  scrubber replaces every frame's file path with just its basename).
- **Project paths** — your `cwd`, your `$HOME`, your project directory.
- **Git remotes** — `git remote get-url origin` is never read for telemetry.
- **Hostnames** — your machine's hostname is never read.
- **Usernames** — `os.User`, `git config user.name`, `$USER` are never read.
- **Arbitrary user input** — any string you typed as a command argument
  (the `command` property is the cobra subcommand path, NOT the args you
  passed).
- **Environment variables** other than the recognized opt-out markers
  (`SPECSCORE_TELEMETRY`, `DO_NOT_TRACK`, `CI`, etc., and only their
  boolean evaluation — not their values).

If you find a counter-example, please file an issue.

## Opt-out

Telemetry is **opt-out**. You can disable it in several ways, evaluated in
this order (first positive signal wins):

1. **One-shot flag on a single invocation:**
   ```
   specscore --no-telemetry <subcommand>
   ```

2. **Environment variable for a session, script, or shell profile:**
   ```
   export SPECSCORE_TELEMETRY=0
   # or the rfc-ish convention:
   export DO_NOT_TRACK=1
   ```

3. **Continuous-integration auto-disable** (no action needed — happens
   automatically when `CI=true`, `GITHUB_ACTIONS=true`, `GITLAB_CI=true`,
   `BUILDKITE=true`, or `CIRCLECI=true`).

4. **Persistent preference** (written to `~/.specscore/telemetry.yaml`):
   ```
   specscore telemetry disable                  # global, both channels
   specscore telemetry disable usage-stats      # only product analytics
   specscore telemetry disable crash-reports    # only error reporting
   specscore telemetry enable                   # symmetric for re-enabling
   specscore telemetry status                   # show current state
   ```

A local opt-out always wins over any caller-tagging value — a plugin or
script setting `SPECSCORE_CALLER` cannot bypass your opt-out.

## Data Retention

### usage-stats

PostHog's free-tier retention applies: events are retained for **7 years**.
To request deletion of all events associated with your installation:

1. Find your `install_id` by running `cat ~/.specscore/install_id`.
2. Submit a deletion request to PostHog via
   <https://posthog.com/handling-personal-data> referencing that
   `install_id` as the `distinct_id`. PostHog typically processes deletion
   within 30 days.

You can also simply delete `~/.specscore/install_id` locally — your next
`specscore` invocation will generate a fresh ID, and the old install's
events become orphaned in PostHog (no longer associated with anyone you
can identify).

### crash-reports

Sentry's free-tier retention applies:

- **Individual events**: 30 days.
- **Issues / crash signatures**: 90 days.

To request deletion of events associated with your machine, find your
`install_id` (`cat ~/.specscore/install_id`) and submit a deletion request
via <https://sentry.io/legal/dpa/>. Sentry typically processes requests
within 30 days.

You can also delete `~/.specscore/install_id` locally — your next
`specscore` invocation will generate a fresh ID; any future crash events
ship under the new ID, and the old install's events become orphaned.

---

For the architectural contract behind these guarantees, see
<https://specscore.md/cli/telemetry>.
