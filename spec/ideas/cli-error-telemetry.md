# Idea: CLI Error Telemetry

**Status:** Implemented
**Date:** 2026-05-21
**Owner:** alexandertrakhimenok
**Promotes To:** cli/telemetry/errors-telemetry
**Supersedes:** —
**Related Ideas:** extends:cli-telemetry

## Problem Statement

How might we learn when the specscore CLI crashes or hits unexpected error paths in the wild, without conflating crash reporting with product analytics or breaking the trust contract we set with cli-telemetry?

## Context

Surfaced during initial review of [`cli-telemetry`](cli-telemetry.md) on 2026-05-21. Product analytics (PostHog) tells us *what* users do; crash reports tell us *what broke*. The two answer different questions, draw on different trust budgets, and should be separately controllable. Conflating them — e.g. sending crash reports through the PostHog channel — risks both polluting the product-analytics dataset and raising the disclosure burden of `cli-telemetry` (panics tend to carry stack frames with file paths, which the product-analytics invariant forbids).

**Timing revised 2026-05-21 (post-Approval):** initially scoped as W6+ ("not now, but next") on the assumption that early-cohort crashes would surface as DMs and Issues. Reversed because the cost of losing a single early adopter to a silent crash is catastrophic at the 10–30-user scale where the founder is investing personally in each install. Every crash from the first cohort that we can fix-and-ship within hours converts a near-churn into a retained user; without crash telemetry we don't even know to act. The trade is ~2–3 incremental days of W1 scope on top of `cli-telemetry`. Ships **together with `cli-telemetry` in W1** when feasible; if W1 capacity is tight, `cli-telemetry` ships first and `cli-error-telemetry` slips to W2 — never compromise the scrubber to hit a date.

## Recommended Direction

Add a **Sentry** client (cloud, EU region, free tier), wired only into the CLI's top-level panic recovery handler and the "unexpected error" path (exit code ≥10 in the shared exit-code contract). Expected errors with documented exit codes 1–9 are never reported. Sentry was picked over GlitchTip for v1 on time-to-ship grounds; GlitchTip speaks the same Sentry wire protocol, so swapping later is a config change, not a rewrite.

The scrubber lives in the **same `internal/telemetry` package** as the product-telemetry scrubber, so the privacy invariant "no user-authored text leaves this binary" has one audit surface. Sentry SDK imports stay confined to `internal/telemetry/errors.go`; product-telemetry code does not transitively depend on Sentry. Before transmission, the scrubber strips all file paths, project paths, git remotes, and any string that looks like user-authored content; Sentry receives only function names, file basenames (e.g. `feature.go:142`, never `/Users/.../specscore-cli/internal/feature/feature.go:142`), and panic messages **only from a known-safe allowlist of formatters**. Panics that wrap user input via `fmt.Errorf("...%s...", userInput, err)` go through `telemetry.SafePanic("known-message-id", err)` to send a stable, content-free identifier; anything outside the allowlist sends stack frames only, tagged `message: unscrubbed`. This expands v1 scope — every existing panic site has to be audited and either wrapped or marked unscrubbed — but it preserves debugging signal in the common case without trusting heuristic scrubbing.

Opt-out is independent from `cli-telemetry`: `specscore telemetry error {enable|disable|status}` subcommand, separate env var `SPECSCORE_ERROR_TELEMETRY=0`. `DO_NOT_TRACK=1` and `CI=true` disable both. The first-run notice in `cli-telemetry` mentions both channels and links to a single `docs/telemetry.md` page with separate sections.

## Alternatives Considered

- **Use PostHog for crash reports too.** Loses: pollutes the product-analytics funnel; PostHog isn't a crash-reporting tool (no symbolication, no de-duplication, no release tracking); raises the data-disclosure floor of `cli-telemetry`.
- **GitHub Issues / `specscore feedback` as the only crash channel.** Loses: requires users to take action when they're already frustrated; silent gives-ups never surface; can't reproduce without environment context.
- **Self-host GlitchTip from day one.** Loses: ops burden for a solo founder. Defer to Q3+; Sentry's free tier (5k events/mo) is more than enough at this scale.
- **Merge into a single Feature with `cli-telemetry`.** Loses: opt-out granularity (users may want one channel off and the other on); single feature blocks itself if either side gets stuck; conflates two distinct trust-disclosure conversations. Ships *concurrently* with `cli-telemetry` in W1, but as a separate Feature with its own opt-out.
- **Defer to W6+ (original 2026-05-21 Approval position).** Loses: crash signal from the first soft-post cohort (W4–W5), which is the highest-value debugging input of Q2. Silent gives-ups at early-adopter scale are catastrophic — losing 1 of 10 users is a 10% retention hit. Reversed during second review same day.

## MVP Scope

**Timing:** **W1 of PLAN-90D, alongside `cli-telemetry`** (target end of W1 / 2026-06-08). Fallback if W1 capacity is tight: `cli-telemetry` ships first, `cli-error-telemetry` slips to W2 but no further. Crash signal from the first soft-post cohort (W4) is the highest-leverage debugging input the project will receive in Q2; shipping later means flying blind precisely when fix-and-ship velocity matters most.

**Scope:** panic-recovery handler + unexpected-error path emit to Sentry cloud with stack-frame scrubbing; allowlist-based panic-message gating via a `telemetry.SafePanic` wrapper; audit of every existing panic site in the CLI to either wrap it or mark it unscrubbed; separate opt-out; release tagging so we can correlate crashes to CLI versions; one Sentry alert (Slack DM to founder) on any new crash signature in the latest release. Plus a hidden-from-`--help` (but documented in `docs/telemetry.md`) verification subcommand `specscore debug error --text "<msg>"` that synthesises a test error event and emits it through the live pipeline — same scrubber, same Sentry project, tagged `debug: true` so the alert rule can suppress it from on-call noise. The command **honors error-telemetry opt-out by default** (no-ops with a clear message pointing at `specscore telemetry error enable`); a `--force` flag overrides the opt-out for a single invocation, intended for founder-driven pipeline tests without flipping the persistent setting. `--force` is documented as "diagnostic only" and prints what it's about to send before sending. The command is the only sanctioned way to verify "is error telemetry actually wired up?" end-to-end without crashing the binary, and it doubles as the post-deploy smoke test in CI (CI invocations will use neither `--force` nor enabled telemetry, so the smoke test exercises the *opt-out path* end-to-end — itself the more interesting test).

Out of scope: source-map / symbolication infrastructure beyond what Sentry's Go SDK does natively; user feedback collection on crash (Sentry's user-feedback dialog is irrelevant to a CLI); breadcrumbs that would require tracing every CLI command (too noisy, overlaps `cli-telemetry`).

## Not Doing (and Why)

- Sending stack traces that contain user-authored content (spec paths, project paths, file contents) — must be scrubbed before transmission, same hard invariant as cli-telemetry
- Replacing structured PostHog product telemetry — this is a separate channel for panics and unexpected errors, not a substitute for the cli-telemetry funnel
- Reporting expected errors (validation failures, exit codes 1-9 in the documented contract) — only unexpected paths, panics, and exit codes ≥10
- Coupling opt-out with cli-telemetry's opt-out — users may want product analytics off but crash reports on, or vice versa; separate toggle

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | Stack-frame scrubbing reliably strips file paths and any user-authored content before transmission. | Write fuzzed unit tests against the scrubber with adversarial paths (`/home/.../my-secret-project/...`, frames containing pasted spec content). Sentry receives only basenames + line numbers + function names. |
| Must-be-true | Sentry's free tier (5k events/mo) covers Q2 panic volume. At <100 retained users and an unstable-but-not-broken CLI, 5k crashes/mo would itself be the crisis. | Monitor Sentry quota weekly; if we approach cap, the CLI has a bigger problem than telemetry quota. |
| Should-be-true | Users tolerate a separate opt-out for crash telemetry alongside product telemetry. | Watch for confusion in Issues / DMs after launch; if "how do I turn off telemetry" complaints don't mention the distinction, the separation is fine. |
| Should-be-true | Panic-recovery + exit-code ≥10 captures the right set of errors without false positives from expected paths. | Audit the existing CLI exit-code call sites before W6; any code path returning ≥10 for an *expected* condition gets renumbered into 1–9 first. |
| Might-be-true | GlitchTip / self-host doesn't become necessary in Q2. Decided at the same review threshold as PostHog → self-host. | Defer; revisit when first GDPR / data-residency question arrives. |

## SpecScore Integration

This Idea promotes into a single child Feature under the shared `cli/telemetry` parent established by the sibling [`cli-telemetry`](cli-telemetry.md) Idea. See that Idea's *SpecScore Integration* section for the full hierarchy and naming convention.

**Feature path to scaffold:**

| Path | Holds |
|---|---|
| `spec/features/cli/telemetry/errors-telemetry/` | Sentry client wiring, scrubber-consumer code, `telemetry.SafePanic` allowlist wrapper, panic-site audit & retrofit, exit-code-≥10 hook into the recovery path, release tagging via goreleaser, `specscore telemetry errors {enable\|disable\|status}` subcommand, `specscore debug error --text "<msg>" [--force]` verification subcommand |

`/specstudio:specify cli-error-telemetry` should be run **after** `/specstudio:specify cli-telemetry` has scaffolded the parent `cli/telemetry` Feature (which owns the shared scrubber, opt-out plumbing, and first-run notice). This Idea does not scaffold the parent.

**Existing Features affected:** `cli` root command (panic recovery wiring); `cli/telemetry` (shared opt-out plumbing and first-run notice — adds the errors-channel section); the shared exit-code contract (audit any existing call site returning ≥10 for an *expected* condition and renumber it into 1–9 before launch).

**Dependencies:** Sentry Go SDK; a Sentry project provisioned in EU region (for symmetry with the PostHog choice in `cli-telemetry`); release-tagging wired into goreleaser so Sentry can correlate crashes to CLI versions.

## Open Questions

- **Initial allowlist of safe panic-message IDs.** The `telemetry.SafePanic` wrapper needs a starting set of stable identifiers (e.g. `spec-parse-failed`, `feature-not-found`, `lint-internal-error`). Enumerated at `/specify` time after the panic-site audit reveals the actual surface.
- **`SafePanic` retrofit migration path.** Audit, wrap, or mark-unscrubbed every existing panic site before v0.3.0 ships. Whether this is one PR or a series, and whether unwrapped panics block the release, gets settled when the audit produces a count.

> **Resolved during second review (2026-05-21):**
> - Vendor → **Sentry** (cloud, EU region) for v1. GlitchTip deferred; same wire protocol means later migration is a config change.
> - Scrubber location → **same `internal/telemetry` package** as the product-telemetry scrubber. Sentry SDK imports confined to `internal/telemetry/errors.go`.
> - Panic-message handling → **(a) allowlist via `telemetry.SafePanic` wrapper.** Unwrapped panics send stack frames only, tagged `message: unscrubbed`. Expands v1 scope by requiring an audit of every existing panic site.
> - `specscore debug error` opt-out behavior → **honor opt-out by default; `--force` flag overrides for a single invocation**, prints what it's about to send before sending, documented as "diagnostic only."
> - `debug` namespace design → **deferred.** No concrete sibling commands in mind right now. `debug error` ships with a flag shape that doesn't preclude siblings (positional subcommand under `debug`, not `--error` flag).

---
*This document follows the https://specscore.md/idea-specification*
