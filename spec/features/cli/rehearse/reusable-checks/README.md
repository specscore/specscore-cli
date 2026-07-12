---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: rehearse reusable checks — `**Use:**`

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/reusable-checks?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/reusable-checks?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/reusable-checks?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/reusable-checks?op=request-change) |
**Status:** Draft
**Source Ideas:** —
**Supersedes:** —

## Summary

A **check** is a reusable, parameterized verification unit — a Markdown file with declared params and a verification block — that many scenarios invoke with a `**Use:**` directive: a clickable Markdown link to the check plus its parameters. This is the **keystone** of the shared-verification model: write a verification once (e.g. "the session cookie is hardened"), then reuse it across every flow that needs it (email, Google, Facebook sign-in) instead of copy-pasting the same assertion steps into each scenario.

It is the concept un-conflated from Rehearse's original `_acs` format: an AC states *what must be true* (intent); a **check** is the reusable *how we verify it*.

## Problem

Cross-scenario verification is copy-pasted today. Three sign-in flows that share identical post-authentication checks (session issued, cookie hardened, redirected to dashboard) must each repeat those steps verbatim; tightening a rule means editing every copy and risking drift. There is no primitive for "write the verification once, reference it from many flows" — the exact capability Rehearse markets, and the load-bearing piece of the thin-AC model (once proof moves out of the AC, shared proof needs a home that isn't copy-paste).

## Behavior

### The check artifact

#### REQ: check-artifact

A check is a Markdown file (convention: `<slug>.check.md`) containing:
- a `# Check: <slug>` heading (the slug is its id),
- a `**Params:** <name>, <name>, …` line declaring its parameters (may be absent when it takes none),
- a single fenced verification block (`bash` for the MVP) that returns non-zero to fail.

A check is standalone and referenceable — not bound to any one scenario.

### Invoking a check

#### REQ: use-directive

A scenario invokes a check with a `**Use:**` directive line: a proper Markdown link (clickable on GitHub and in Studio, and lint-resolvable per Decision 0010 — references are URLs) plus an optional `with` clause:

```
**Use:** [<label>](<check-url>) with <name>=<value> <name>=<value> …
```

`<check-url>` is a URL reference to the check file (relative shorthand = current repo, per Decision 0010). The `with` clause is optional when the check declares no params. `**Use:**` is a **positional** directive — it may appear multiple times in document order, typically under a `## Then` step — and is deliberately a bold-metadata line (matching `**Verifies:**` / `**Params:**`) rather than a `###` heading, so it does not pollute the document outline.

#### REQ: param-binding

Each `<name>=<value>` binds a parameter. A value is a literal or a `{{context}}` reference resolved from the scenario's context bag. Inside the check's verification block, a param is available as `{{name}}` (reusing the existing context-bag interpolation). A param the check declares but the invocation omits is an error (REQ: use-validation).

#### REQ: check-execution

Checks run after the scenario's steps, in the scenario's working directory (data passes between steps and checks through files in that dir, as steps already do), with the scenario's context bag available plus the bound params overlaid (params do not leak back into the scenario bag). A non-zero exit fails the scenario; the failure detail names the check ref and includes the check's output. A passing check is silent, matching `### Assert:` behavior.

#### REQ: use-validation

An unresolvable `<check-url>`, a malformed `**Use:**` line, or a missing required param fails the scenario with a clear, actionable error that names the check and the problem.

### Structure-agnostic

#### REQ: structure-agnostic

`**Use:**` works under a flat `## Then` step and under a nested `#### Then` (the `describe/context/it` suite model) identically. This feature is independent of scenario nesting; nested suites are a separate sibling feature.

## Architecture & Components

- `internal/rehearse/scenario/` — parse `**Use:**` directive lines into `CheckUse{Ref, Params, Line}` (document order, sibling to `FileAssertions`); parse `<slug>.check.md` into `Check{Slug, Params, Body}`.
- `internal/rehearse/runner/run.go` — after steps and file assertions, resolve each `CheckUse` relative to the scenario, validate params, overlay them on a child context, interpolate and execute the check body via the existing bash executor, and fail the scenario on non-zero exit or validation error.
- Reuses the context bag, `{{name}}` interpolation, and bash executor — no new execution engine.

## Testing Strategy

Unit tests for parsing (`**Use:**` directive, `.check.md`) and runner (pass, fail, param binding, context param, missing check, missing param). One e2e scenario in `_tests/` exercising a real check reused across two scenarios. 100% coverage of touched packages; wired into the corpus.

## Not Doing / Out of Scope

- **Non-bash check bodies** (sql/hurl/etc.) — bash only for the MVP; other kinds follow once the primitive is proven.
- **Check composition** (a check that `**Use:**`s another) — deferred.
- **Thin AC linkage** (a check `Proves:` an AC) — the sibling `thin-ac` feature.
- **Nested `describe/context/it` suites** — the sibling nested-suites feature; `**Use:**` is agnostic to it.

## Acceptance Criteria

### AC: use-runs-and-passes

Scenario: an invoked check whose verification passes
Given a check `assert-ok.check.md` whose bash block exits 0
When a scenario runs `**Use:** [assert-ok](./assert-ok.check.md)`
Then the scenario passes and the check adds no output

### AC: use-failure-fails-scenario

Scenario: an invoked check whose verification fails
Given a check whose bash block exits non-zero
When a scenario uses it
Then the scenario fails and the detail names the check ref and its output

### AC: use-param-binding

Scenario: a bound param is visible inside the check
Given a check with `**Params:** expected` whose block asserts `{{expected}}` appears in a file
When a scenario runs `**Use:** [contains](./contains.check.md) with expected=hello`
Then the check sees `expected=hello` and the assertion holds

### AC: use-context-param

Scenario: a captured context value passed as a param
Given a prior step captured `token` into the context bag
When a scenario runs `**Use:** [assert-token](./assert-token.check.md) with value={{token}}`
Then the check receives the captured value

### AC: use-reused-across-scenarios

Scenario: one check, two scenarios
Given a single `assert-session.check.md`
When two different scenarios both `**Use:**` it after different auth flows
Then both scenarios evaluate the same check (no copy-paste)

### AC: use-missing-check-errors

Scenario: an unresolvable check reference
When a scenario runs `**Use:** [nope](./nope.check.md)`
Then the scenario fails with an error naming the missing check

### AC: use-missing-param-errors

Scenario: a required param is not supplied
Given a check declaring `**Params:** expected`
When a scenario runs `**Use:** [contains](./contains.check.md)` with no `expected`
Then the scenario fails with an error naming the missing param

## Open Questions

- Should `with` values support shell-style quoting for spaces, or are `{{context}}` + literals enough for the MVP? (Leaning: keep MVP simple; quoting later if needed.)

## Autonomous Decisions

- Reuse the context bag + `{{name}}` interpolation + bash executor — a check body is "a bash block with declared inputs," params a scoped overlay on the bag.
- `**Use:**` is a bold-metadata directive (not a `###` heading) with a real Markdown link: clickable, outline-clean, and Decision-0010-compliant. (The shipped `### Assert:` heading may migrate to `**Assert:**` for family consistency under the migration story — separate work.)
- Checks run after steps like assertions (data flows via workdir files), so no positional-execution engine is needed for the MVP.

---
*This document follows the https://specscore.md/feature-specification*
