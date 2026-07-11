---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: rehearse run --filter — run scenarios by acceptance criterion

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/run-filter?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/run-filter?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/run-filter?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/run-filter?op=request-change) |
**Status:** Stable
**Source Ideas:** —
**Supersedes:** —

## Summary

`specscore rehearse run <scenario-dirs> --filter <ac-ref>` runs only scenarios whose `**Verifies:**` field matches the specified AC reference (e.g., `cli/studio/index#ac:index-two-repos`). Filters scenarios before execution, skipping those that don't match the AC. Useful for focused testing: run scenarios for a single AC, feature, or feature family during development. Multiple `--filter` flags accumulate (OR semantics: matches any filter).

## Problem

Rehearse scenarios are organized by feature, but developers often want to run just the scenarios for one AC while working on it. Currently, they must either run all scenarios in a directory (includes unrelated ACs) or manually select scenario files. A `--filter` flag lets developers specify exactly which AC(s) to test, reducing feedback time and test noise during iterative development.

## Behavior

### Filter flag usage

#### REQ: filter-flag-syntax

`specscore rehearse run <scenario-dirs> --filter <ac-ref>` accepts one or more `--filter` flags. Each filter is a full AC reference: `<feature-slug>#ac:<ac-slug>` (same format as `rehearse new` accepts).

Examples:
```
specscore rehearse run spec/features/cli/studio/index/_tests --filter cli/studio/index#ac:index-two-repos
specscore rehearse run spec/features --filter cli/studio/index#ac:index-two-repos --filter cli/rehearse/new#ac:resolve-ac-reference
```

#### REQ: filter-matching

A scenario matches a filter if its `**Verifies:**` field (parsed from the frontmatter) exactly equals the filter AC reference. Matching is case-sensitive and exact (no substring or prefix matching).

If the scenario has multiple `**Verifies:**` lines (one per AC), the scenario matches if any of them match the filter. The `**Verifies:**` field is parsed from the scenario body (not YAML frontmatter).

#### REQ: filter-multiple-accumulate

Multiple `--filter` flags accumulate with OR semantics: if any filter matches, the scenario is included. No scenario is run twice.

#### REQ: no-filter-default

If no `--filter` is provided, all scenarios in the directories are run (existing behavior).

#### REQ: filter-output

When filtering is active, the output prefix for each scenario includes the filter status:
- `[filter-match] <scenario-path>: pass` — scenario matched the filter and passed
- `[filter-skip] <scenario-path>: skipped` — scenario did not match any filter
- Skipped scenarios do not run and do not count toward pass/fail totals

In JSON output format, skipped scenarios have `"status": "skip"` and are omitted from the final pass/fail counts (displayed separately).

### Error handling

#### REQ: filter-syntax-invalid

Invalid AC reference format (e.g., missing `#ac:` part) → exit 2 with an error message explaining the expected format.

#### REQ: filter-no-matches

If no scenarios match any filter, exit 0 but output a message: "No scenarios matched filter(s): <list of filters>". This is not an error (no work to do is OK).

## Architecture & Components

- `internal/runner` — extend the runner to:
  - Accept a `--filter` flag (string slice, can repeat)
  - Parse each filter as an AC reference (validate format)
  - After loading scenarios, filter the slice by checking each scenario's `**Verifies:**` field
  - Adjust output labeling to distinguish matched vs. skipped

- `pkg/scenario` — extend parsing to expose the `Verifies` field(s) as a public slice

No new packages needed; filtering happens in the runner's scenario-loading phase.

## Testing Strategy

Unit tests:
- `TestRunFilter_MatchesVerifies` — scenario matches filter AC
- `TestRunFilter_MultipleFiltersOR` — multiple filters accumulate (OR semantics)
- `TestRunFilter_NoMatches` — exit 0 when no scenarios match
- `TestRunFilter_InvalidFormat` — exit 2 on bad AC reference format
- `TestRunFilter_SkippedNotRun` — skipped scenarios are not executed

E2e: scenarios in `spec/features/cli/rehearse/run-filter/_tests/` verify filtering behavior with fixture scenarios.

## Not Doing / Out of Scope

- No **negative filters** (e.g., `--exclude-filter`) — can be added in v0.7
- No **glob patterns** or regex in AC references — exact match only
- No **feature-level filters** (e.g., filter by feature slug alone, excluding AC) — v0.7 candidate
- No **tag-based filtering** — out of scope for v0.6

## Acceptance Criteria

### AC: filter-flag-syntax

Scenario: --filter flag accepts AC reference
Given a scenario with `**Verifies:** cli/studio/index#ac:index-two-repos`
When I run `specscore rehearse run spec/features/cli/studio/index/_tests --filter cli/studio/index#ac:index-two-repos`
Then the scenario is run and exits 0 (if the scenario passes)

### AC: filter-matching-exact

Scenario: only exact AC reference matches
Given two scenarios: one verifying `cli/studio/index#ac:index-two-repos`, another verifying `cli/studio/index#ac:index-one-repo`
When I run with `--filter cli/studio/index#ac:index-two-repos`
Then only the first scenario runs, the second is skipped

### AC: filter-multiple-or

Scenario: multiple filters use OR semantics
Given scenarios verifying `cli/studio/index#ac:index-two-repos` and `cli/rehearse/new#ac:resolve-ac-reference`
When I run with `--filter cli/studio/index#ac:index-two-repos --filter cli/rehearse/new#ac:resolve-ac-reference`
Then both scenarios run

### AC: no-filter-default

Scenario: without --filter, all scenarios run
Given a directory with multiple scenarios
When I run `specscore rehearse run <dir>` (no --filter)
Then all scenarios are run (existing behavior unchanged)

### AC: filter-invalid-syntax

Scenario: invalid filter AC reference exits 2
Given a malformed filter like `cli/studio/index` (missing #ac: part)
When I run with `--filter cli/studio/index`
Then the command exits 2 with an error message

### AC: filter-output-labels

Scenario: filtered scenarios are labeled in output
Given scenarios matching and not matching the filter
When I run with `--filter cli/studio/index#ac:index-two-repos`
Then matching scenarios show `[filter-match]` prefix, skipped scenarios show `[filter-skip]` prefix

### AC: filter-no-matches

Scenario: no scenarios matched by filter exits 0
Given a filter that matches no scenarios
When I run with `--filter cli/nonexistent#ac:nonexistent`
Then the command exits 0 and prints "No scenarios matched filter(s): ..."

## Open Questions

None at this time.

## Autonomous Decisions

- Multiple filters use OR semantics, not AND. Simpler to reason about and more useful for the "run scenarios for this AC or that AC" workflow.
- Filtering happens after scenarios are loaded, not before (no filesystem filtering). Cleaner implementation and log visibility.
- Skipped scenarios are labeled `[filter-skip]` in output and have `status: skip` in JSON, distinct from failures. No second invocation needed to verify filter behavior.
- Invalid filter format → exit 2 (same as other CLI input validation errors), not exit 1.

---
*This document follows the https://specscore.md/feature-specification*
