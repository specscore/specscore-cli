---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: rehearse nested scenario suites — Given/When/Then as describe/context/it

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/nested-suites?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/nested-suites?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/nested-suites?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/nested-suites?op=request-change) |
**Status:** Draft
**Source Ideas:** —
**Supersedes:** —

## Summary

A scenario file may branch: a `## Given` context with **multiple `### When` branches**, each with its own `#### Then` outcomes. This is the `describe/context/it` model (RSpec/Jest) — it groups shared setup and lets behaviour branch (success *and* failure from one Given) instead of forcing duplicate flat scenarios. Because two mutually-exclusive `When`s cannot run in one pass, such a file is a **suite**: each root-to-leaf path (`Given` + one `When`) is executed as an isolated **case** in its own working directory, and reported per case.

## Problem

Flat Given/When/Then (peers) can't express branching from shared setup: "Given a Google account — *when* the callback succeeds → these outcomes; *when* it fails → those" forces two separate scenarios that each re-declare the Given, duplicating setup and scattering related behaviour. Authors reach for nesting because it matches how they think about behaviour — a decision tree rooted in context.

## Behavior

### Detecting a suite

#### REQ: suite-detection

A scenario is a **nested suite** when it contains one or more `### When` (H3) branch headings (outside fenced code). Flat scenarios — which use `## When` (H2) narrative headings, or none — are unaffected and run exactly as before. `### When` inside a fenced block is ignored.

### Splitting and executing

#### REQ: suite-split

A suite is split into the shared **Given** preamble (everything before the first `### When`, including frontmatter and body metadata such as `**Verifies:**`) and one branch per `### When` (from its heading to the next `### When` or end of file). The `#### Then` headings under a `When` are narrative; a branch's blocks, `### Assert:` checks, and `**Use:**` invocations all belong to that branch.

#### REQ: suite-cases

Each branch runs as an independent **case**: the Given preamble concatenated with that branch is executed as a scenario in its own fresh working directory (so the Given setup runs per branch and branches never share state). A file with N `### When` branches yields N reports.

#### REQ: suite-reporting

Each case report carries the file path plus a `case` field — the `### When` branch label — and the file-level `**Verifies:**`. Human output labels a case `<file> › <When …>`. Cases pass or fail independently; one branch's failure never affects another.

## Architecture & Components

- `internal/rehearse/scenario/suite.go` — `SplitSuite(data)` detects the `### When` branches and returns the Given preamble + one `SuiteWhen{Label, Content}` per branch.
- `internal/rehearse/runner/run.go` — `runScenarioFile` reads a file, and for a suite runs each `given+branch` slice through the existing per-scenario executor (`runScenarioData`) as a case, tagging the report's `Case`. `ScenarioReport` gains a `case` field. Flat scenarios are a single case with an empty label — no behaviour change.
- Maximal reuse: a case is just a synthesized flat scenario, so all existing parsing, stepping, assertions, checks, context bag, and expected-fail behaviour apply per branch unchanged.

## Testing Strategy

Unit tests for `SplitSuite` (flat vs nested, fence-ignored) and the runner (two branches each a case, branch isolation with mixed pass/fail, shared Given per branch, file-level Verifies per case, human labelling). One e2e suite in `_tests/` runs through the real CLI. 100% coverage of touched packages; wired into the corpus.

## Not Doing / Out of Scope

- **Per-branch `**Verifies:**`/evidence** — each case currently inherits the file-level Verifies; branch-scoped criteria are a v2 refinement.
- **Depth beyond Given → When → Then** (e.g. nested Whens) — three levels only for now.
- **Shared-Given-run-once optimisation** — the Given is re-run per branch for isolation (matches per-scenario sandboxing); a run-once mode is a later option.

## Acceptance Criteria

### AC: suite-two-cases

Scenario: a two-branch suite runs each branch as a case
Given a `## Given` preamble and two `### When` branches
When the suite runs
Then two reports are produced, each carrying its `### When` label and the file-level Verifies

### AC: suite-shared-given

Scenario: the Given setup runs per branch
Given a `## Given` that writes a seed file, and branches that read it
When the suite runs
Then every branch sees the Given setup (in its own working directory)

### AC: suite-branches-isolated

Scenario: branches are independent
Given one branch that passes and one that fails
When the suite runs
Then exactly one case passes and one fails; the failure does not affect the other

### AC: flat-unchanged

Scenario: flat scenarios are unaffected
Given a scenario using only `## When` (H2) headings, or none
When it runs
Then it produces exactly one report with no `case`, exactly as before

## Open Questions

- Should a case's `case` label strip the leading "When " for brevity, or keep it? (Leaning: keep it — reads naturally as "file › When the callback succeeds".)

## Autonomous Decisions

- `### When` (H3) is the branch marker: it can never collide with a flat `## When` (H2) or a `#### Then` (H4), so nesting is opt-in and flat scenarios are strictly unaffected.
- A case is executed by synthesizing a flat scenario (Given + branch) and reusing `runScenarioData` — no second execution engine, and every existing per-scenario behaviour composes per branch for free.

---
*This document follows the https://specscore.md/feature-specification*
