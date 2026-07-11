---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: rehearse expected-fail scenarios

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/expected-fail?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/expected-fail?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/expected-fail?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/expected-fail?op=request-change) |
**Status:** Stable
**Source Ideas:** —
**Supersedes:** —

## Summary

Add a `**Expect:** fail` scenario metadata directive so a scenario can declare that its steps/assertions are *supposed* to fail. The runner inverts the terminal outcome for such scenarios: a scenario that correctly fails reports `pass`; one that unexpectedly passes reports `fail`. This lets negative acceptance scenarios (those that prove the runner rejects bad input) join the CI corpus run, which otherwise exits non-zero on any failing scenario. With this in place, the file-assertion scenario families are wired into the corpus job for real end-to-end verification.

## Problem

The corpus runner (`specscore rehearse run <dirs>`) exits 1 if any scenario reports `fail` (`CountFailed`). Negative scenarios — e.g. `file-assertions/_tests/contains-fails.md`, which asserts the runner *fails* a mismatched `contains` — therefore cannot be in the corpus: they would turn CI red by design. As a result, the whole `file-assertions*` scenario family is excluded from the corpus job's hardcoded directory list, so those acceptance scenarios never actually execute in CI. Their only protection is Go unit tests; the promised end-to-end verification is illusory. A first-class expected-fail concept closes this gap.

## Behavior

### Scenario metadata

#### REQ: expect-metadata

A scenario may declare `**Expect:** fail` in its body metadata (alongside `**Status:**` / `**Verifies:**`). The value is one of:
- `pass` (default when the directive is absent) — the scenario is expected to pass.
- `fail` — the scenario is expected to fail (its steps or assertions should not all succeed).

The parser exposes this as `Scenario.Expect`, defaulting to `pass`. Any value other than `fail` is treated as `pass` (forward-compatible; no hard error on unknown values).

### Outcome inversion

#### REQ: expect-fail-inversion

For a scenario with `**Expect:** fail`, the runner inverts only the terminal `pass`/`fail` outcome:
- raw outcome `fail` → reported `pass` (the scenario correctly failed; it met expectation). The failing step/assertion detail is retained in the report for transparency.
- raw outcome `pass` → reported `fail`, with a scenario-level detail `scenario was expected to fail (**Expect:** fail) but passed`.

`no-steps` and `skipped` outcomes pass through unchanged (there was no pass/fail to invert). A scenario **parse error** is always a real `fail`, never inverted — a malformed scenario is a defect regardless of expectation.

#### REQ: expect-in-report

The JSON report carries the scenario's `expect` value when it is `fail` (omitted for the default `pass`), so a reader can see that a reported-`pass` scenario is a correctly-failing negative case rather than a plain success.

### Corpus wiring

#### REQ: corpus-includes-file-assertions

The CI corpus job runs the file-assertion scenario directories (`file-assertions`, `file-assertions-glob`, `file-assertions-glob-recursive`) in addition to the existing `run` and `evidence` directories, so those acceptance scenarios execute end-to-end on every push. Negative scenarios in those directories carry `**Expect:** fail`.

## Architecture & Components

- `internal/rehearse/scenario/scenario.go` — parse `**Expect:**` into `Scenario.Expect` (default `pass`).
- `internal/rehearse/runner/run.go` — apply outcome inversion in the single `finish` exit helper, driven by the parsed `Expect`.
- `internal/rehearse/runner/run.go` (`ScenarioReport`) — add an `Expect string` field (`json:"expect,omitempty"`).
- `.github/workflows/go-ci.yml` — add the three file-assertion `_tests` directories to the corpus run.
- `spec/features/cli/rehearse/file-assertions/_tests/contains-fails.md` — declare `**Expect:** fail`.

## Testing Strategy

Unit tests:
- `scenario`: `**Expect:** fail` parses to `Expect == "fail"`; absent → `"pass"`; unknown value → `"pass"`.
- `runner`: an expected-fail scenario whose step fails reports `pass` (with the failing step retained and `Expect == "fail"` in the report); an expected-fail scenario that passes reports `fail` with the explanatory detail; a parse error under `**Expect:** fail` stays `fail`.

E2e: the negative `contains-fails.md` scenario, now `**Expect:** fail`, is exercised by the corpus and reports `pass`. 100% coverage of the touched packages.

## Not Doing / Out of Scope

- No per-step expected-fail (`Expect` is scenario-level only).
- No expected-`skipped` or expected-`no-steps` semantics.
- No change to the default (`pass`) path for the existing corpus scenarios.

## Acceptance Criteria

### AC: expect-fail-parses

Scenario: the Expect directive is parsed
Given a scenario whose body contains `**Expect:** fail`
When it is parsed
Then `Scenario.Expect` is `"fail"` (and absent directive yields `"pass"`)

### AC: expected-fail-reports-pass

Scenario: a correctly-failing negative scenario reports pass
Given a scenario with `**Expect:** fail` whose file assertion fails
When the runner runs it
Then the scenario reports `pass`, the report's `expect` is `fail`, and the
failing step detail is retained

### AC: expected-fail-but-passed-reports-fail

Scenario: a negative scenario that unexpectedly passes reports fail
Given a scenario with `**Expect:** fail` whose steps all succeed
When the runner runs it
Then the scenario reports `fail` with a detail noting it was expected to fail

### AC: corpus-runs-file-assertions

Scenario: file-assertion scenarios run green in the corpus
Given the corpus job includes the file-assertion `_tests` directories and
`contains-fails.md` declares `**Expect:** fail`
When the corpus runs
Then every file-assertion scenario reports `pass` (the negative one via
inversion) and the corpus exits 0

## Open Questions

None at this time.

## Autonomous Decisions

- Inversion happens in the single `finish` helper so every terminal path (step fail, assertion fail, happy pass) is covered without threading expectation through each return.
- Parse errors are never inverted: a malformed scenario is a real defect, and inverting it would hide broken scenario files.
- Unknown `**Expect:**` values degrade to `pass` rather than erroring, keeping the directive forward-compatible.

---
*This document follows the https://specscore.md/feature-specification*
