# Idea: CLI Telemetry

**Status:** Implemented
**Date:** 2026-05-21
**Owner:** alexandertrakhimenok
**Promotes To:** cli/telemetry, cli/telemetry/usage-telemetry
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we measure whether developers are actually using the specscore CLI on real specs within 7 days of install, without breaching the trust developers place in dev-tool CLIs?

## Context

The 2026-Q2 GTM plan ([`marketing/PLAN-90D.md`](https://github.com/specscore/specscore-marketing/blob/main/PLAN-90D.md)) declares a single north-star metric: **100 developers who installed `specstudio-skills` AND used it on a real spec twice within 7 days of install.** The plan is unverifiable as written — Claude Code does not expose install or invocation metrics to plugin authors. Confirmed via the official docs: marketplaces are plain git repos with a manifest, and Claude Code's OpenTelemetry events (`Plugin loaded`, `Skill activated`, `Plugin installed`) are emitted to the *end user's* observability backend, not to us. Sources: [plugin-marketplaces](https://code.claude.com/docs/en/plugin-marketplaces.md), [monitoring-usage](https://code.claude.com/docs/en/monitoring-usage.md).

The only signal we can reliably observe is the CLI itself. Every meaningful `specstudio-skills` skill — `specstudio:init`, `:specify`, `:plan`, `:ideate`, plus the lower-level `specscore feature|spec|task|idea` invocations — shells out to the `specscore` binary. Instrumenting the CLI captures the exact actions that define the north star ("used on a real spec twice"), with cleaner semantics than plugin hooks could give us.

W1 of the GTM plan starts **2026-06-01**, ~10 days from now. Shipping telemetry before the first soft post (W4) means the early cohort is measured; shipping it later means they're invisible.

## Recommended Direction

Embed an opt-out PostHog client in `specscore-cli` that emits one event per command from a small `internal/telemetry` package, called from cobra's `PersistentPreRun`/`PersistentPostRun` on the root command. Async batched send with a 500 ms hard timeout — a dead PostHog must never block a user's CLI.

Event payload is a fixed enum of properties: `command` (e.g. `feature.create`, `spec.lint`), `success`, `duration_ms`, `exit_code`, `cli_version`, `os`, `arch`, `caller`, `install_id`, `is_first_run`. The `caller` value is resolved with precedence `--caller` flag > `SPECSCORE_CALLER` env var > default `cli`, and is constrained to a known-values enum (`cli`, `specstudio-skills`, `ci`, `script`, `agent`) with anything else coerced to `other` — this protects PostHog from cardinality explosions and enforces the no-user-authored-text invariant. The `--caller` flag is **documented in `--help`** (not hidden), explicitly described as the integration point for AI agents and CI scripts that drive the CLI without going through `specstudio-skills`. The `install_id` is a random UUID stored in `~/.specscore/install_id` on first run, per-machine (not per-project) so one developer working across multiple repos counts as one install. **Spec content, feature names, file paths, project paths, git remotes, hostnames, and usernames are never sent.** This is enforced at the package boundary, not by convention.

Defaults follow dev-tool CLI norms: opt-out, three-line first-run notice shown **only when `install_id` doesn't exist yet** (i.e. truly once per machine — never a prompt, never repeated), `specscore telemetry {enable|disable|status}` subcommand, and auto-disable when any of `SPECSCORE_TELEMETRY=0`, `DO_NOT_TRACK=1`, `CI=true`, or common CI env vars are set. Local opt-out always wins over any `caller` value — a plugin or script setting `SPECSCORE_CALLER=specstudio-skills` cannot override a user's `telemetry disable`. PostHog cloud, **EU region**, conservative default for GDPR posture. The exact event schema is published in `docs/telemetry.md` so anyone can audit what we send.

## Alternatives Considered

- **Plugin-side hooks (PreToolUse / PostToolUse) POSTing to our endpoint.** Loses: hooks see tool invocations, not semantic spec ops; users must trust per-plugin telemetry; harder to ship a clean opt-out UX; duplicates events when CLI is also instrumented.
- **Roll our own backend (Cloudflare Worker + D1) instead of PostHog.** Loses: ~2 days of work upfront plus ongoing maintenance for a worse funnel/retention UX. PostHog gives us 7-day-active-after-install as a native query. Revisit in Q3 if vendor posture or volume changes.
- **GitHub repo traffic stats + qualitative signal (DMs, `/feedback`, Issues) only.** Loses: cannot answer the "used it twice in 7 days" half of the north star at 20–100 users. Honor-system at the exact scale the goal targets. Keep as a complementary signal; insufficient alone.
- **Defer to v0.2 / Q3.** Loses: the W1–W4 cohort — the first users we touch in the soft-post wave — becomes unmeasurable retroactively. Pre-launch is the cheapest moment to ship telemetry.

## MVP Scope

By **2026-06-08 (end of W1)**: `specscore` v0.2.0 emits the event schema above to PostHog (EU region) on every command, with opt-out via env var and a `specscore telemetry` subcommand. README has a clearly-titled "Telemetry" section linking to `docs/telemetry.md` with the full event schema and opt-out instructions. The PostHog project has one funnel configured: `first_run → first feature.create → second feature.create within 7 days`. That funnel is the north-star dashboard.

Out of scope for *this* MVP: dashboards beyond the one funnel, custom cohort analysis, A/B testing infrastructure, session replay (not applicable to a CLI). Error reporting (Sentry) ships concurrently in W1 as a separate Feature per the sibling [`cli-error-telemetry`](cli-error-telemetry.md) Idea — see that Idea for scope and trade rationale.

## Not Doing (and Why)

- Plugin-side hooks instrumentation — CLI sits on the hot path of every meaningful skill, so duplicating events from hooks adds noise without signal
- Self-hosted analytics backend — PostHog cloud (EU region) is good enough at this scale; self-host is a Q3+ decision if vendor or compliance posture changes
- Sending spec content, file paths, feature names, or any user-authored text — hard invariant, not a configurable option
- Marketplace-level install counters — GitHub repo traffic stats remain the only signal for the pre-CLI funnel step; out of scope for the CLI binary
- Prompting on first run — CLIs that gate on prompts break CI and annoy users; disclosure is one-shot notice + README + opt-out env var

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | The dev-tool audience tolerates opt-out telemetry when disclosure is loud and the schema is publicly auditable. | Soft-post the W2 wedge essay with a footnote linking to `docs/telemetry.md`; track sentiment in replies and Issues. Threshold: <2 negative public reactions per 10 installs in W4–W6. |
| Must-be-true | CLI invocations are a faithful proxy for "used it on a real spec." A `feature.create` event from a real project ≈ a real spec action. | Manual spot-check in W4: cross-reference 5 install_ids that hit the funnel against unsolicited DMs / Issues from the same window. |
| Should-be-true | PostHog free tier (1M events/mo) covers Q2 at our volume. At 100 retained users × ~20 CLI invocations/week × 13 weeks ≈ 260k events — well under cap. | Monitor PostHog usage dashboard weekly; alert at 50% of quota. |
| Should-be-true | A 500 ms hard timeout on the PostHog client is invisible to users in the worst case (network down, PostHog 5xx). | Benchmark in CI: p99 wall-clock overhead < 5 ms on `specscore --version` with telemetry endpoint blackholed. |
| Might-be-true | The `SPECSCORE_CALLER` env-var convention is sufficient to separate plugin-driven from raw-CLI usage without changing the plugin. | Audit `specstudio-skills` skill bash blocks in W2; if any skill shells out without setting `SPECSCORE_CALLER`, file follow-up. |
| Might-be-true | EU-region PostHog is the right default for a project hosted under a `.studio` domain with no declared jurisdiction. | Defer until first paying-customer or first GDPR/privacy question arrives in Issues. |

## SpecScore Integration

This Idea promotes into a **two-feature structure** under a shared parent. The hierarchy is fixed by this Idea (and its sibling [`cli-error-telemetry`](cli-error-telemetry.md)) so `/specstudio:specify` lands the Features in the right place without re-litigating layout at design time.

**Feature paths to scaffold:**

| Path | Source | Holds |
|---|---|---|
| `spec/features/cli/telemetry/` | this Idea (parent of the two channels) | Shared scrubber package (`internal/telemetry`), shared opt-out plumbing, `specscore telemetry` root subcommand surface, single first-run notice covering both channels, `docs/telemetry.md` skeleton, the hard invariant "no user-authored text leaves this binary" |
| `spec/features/cli/telemetry/usage-telemetry/` | this Idea (product-analytics channel) | PostHog client wiring, event schema, `--caller` flag, `SPECSCORE_CALLER` env var, `install_id` lifecycle, the north-star funnel, `specscore telemetry usage {enable\|disable\|status}` subcommand |
| `spec/features/cli/telemetry/errors-telemetry/` | sibling Idea [`cli-error-telemetry`](cli-error-telemetry.md) | Sentry client wiring, `telemetry.SafePanic` allowlist, panic-site retrofit, release tagging, `specscore telemetry errors {enable\|disable\|status}` subcommand, `specscore debug error` verification subcommand |

`/specstudio:specify cli-telemetry` should scaffold the parent `cli/telemetry` Feature **and** the `cli/telemetry/usage-telemetry` child Feature in one pass (since both come from this Idea). `/specstudio:specify cli-error-telemetry` (run separately) scaffolds only `cli/telemetry/errors-telemetry`; the parent already exists.

**Existing Features affected:** the root `cli` command surface (PersistentPreRun/PostRun wiring); `cli/init` (writes the first-run notice on first invocation when `install_id` doesn't exist).

**Dependencies:** PostHog Go SDK (vendor); a PostHog project provisioned in EU region; one funnel configured (`first_run → first feature.create → second feature.create within 7 days`).

## Open Questions

- **Exact first-run notice copy** (three lines, shown once when no `install_id`) — wording to be settled at `/specify` time. Tone target: factual, not apologetic, links to `docs/telemetry.md`.
- **`other` rate threshold for re-thinking the `caller` enum.** Enum-with-`other` is the v1 design; if `other` exceeds some share of events (10%? 20%?) we revisit. Threshold and the "revisit" action to be defined when the PostHog funnel is configured.

> **Resolved during initial review (2026-05-21):**
> - `install_id` location → `~/.specscore/install_id` (per-machine, not per-project).
> - First-run notice trigger → shown only when no `install_id` exists (i.e. truly once).
> - Sentry-style error reporting → out of scope; captured as sibling Idea [`cli-error-telemetry`](cli-error-telemetry.md).
> - Opt-out vs. caller precedence → local opt-out always wins.
> - `--caller` visibility → documented in `--help`, described as the integration point for AI agents and CI scripts.
> - `caller` allowed values → enum (`cli`, `specstudio-skills`, `ci`, `script`, `agent`) with `other` fallback.
> - PostHog region → EU.

---
*This document follows the https://specscore.md/idea-specification*
