# Feature: Plan List

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan/list?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan/list?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan/list?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan/list?op=request-change) |
**Status:** Approved
**Date:** 2026-06-04
**Owner:** alexander.trakhimenok
**Source Ideas:** specscore-cli-should-expose-a-plan-verb-with-list-and-query
**Supersedes:** —

## Summary

`specscore plan list` returns every plan in the project as a flat, alphabetically sorted list of slugs. With `--status` it filters to plans in a given lifecycle status; with `--fields` it returns structured YAML (or JSON) with metadata columns per plan.

## Synopsis

```
specscore plan list [--status <value>] [--fields <names>] [--format <yaml|json|text>] [--project <path>]
```

## Problem

Answering "what plans exist?" or "which plans are still open?" has no CLI path today — callers fall back to `ls spec/plans/` plus a `grep` over `**Status:**`. A flat slug list lets callers pipe into `grep`, `fzf`, or `xargs` without parsing; a `--status` filter answers the common "what's still Approved/open?" question directly; and structured `--fields` output covers tooling that wants metadata in one call. One command, two shapes — matching the `idea list` / `feature list` precedent.

## Behavior

### Default output

Without flags, the command prints one plan slug per line.

#### REQ: flat-text-default

The default output format MUST be text: one plan slug per line, sorted alphabetically, with no headers and no trailing blank line. This output is directly pipeable into standard Unix tools.

#### REQ: empty-listing-exit-zero

When the project contains no plans (or none match an active filter), the command MUST print nothing to stdout and exit `0` — an empty result is not an error.

### Status filtering

`--status <value>` restricts the listing to plans whose `**Status:**` matches `<value>`.

#### REQ: status-filter

`--status <value>` MUST match a plan's `**Status:**` case-insensitively and include only matching plans. The match is exact on the trimmed status string (no substring matching). The filter applies in both text and structured output.

### Structured output

When `--fields` is supplied, the command switches to structured output (YAML by default, JSON with `--format json`) with a top-level list; each entry has the plan slug plus the requested fields.

#### REQ: fields-forces-structured

When `--fields` is non-empty, `--format text` MUST NOT produce text output. If the caller explicitly sets `--format text` alongside `--fields`, the command MUST auto-upgrade to YAML rather than error, for parity with `feature list`.

#### REQ: fields-recognized-set

`--fields` MUST accept a comma-separated list drawn from the recognized plan field names: `status`, `source-feature`, `mode`, `date`, `owner`. An unrecognized field name MUST exit `2` (InvalidArgs) naming the offending field.

## Parameters

None. All inputs are flags.

## Exit codes

| Code | Condition |
|---|---|
| `0` | Listing printed (even if zero plans match) |
| `2` | Unknown `--fields` name, invalid `--format` value |
| `3` | No project found (no `specscore.yaml` reachable from `cwd` and no `--project`) |
| `10` | Unexpected I/O failure while reading the plans directory |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [cli/plan](../README.md) | Inherits the shared `--format`/`--project` contract and the status-from-file rule. |
| [cli/feature/list](../../feature/list/README.md) | Sibling whose text-default + structured-output contract this command mirrors. |

## Acceptance Criteria

### AC: default-listing-pipeable (verifies REQ:flat-text-default, REQ:empty-listing-exit-zero)

**Given** a project with plans `cli-event`, `cli-rules`, and `studio-toolbar`
**When** the user runs `specscore plan list`
**Then** stdout is exactly three lines — `cli-event`, `cli-rules`, `studio-toolbar`, in that order — with no headers or trailing blank, and the exit code is `0`.

### AC: status-filter-selects (verifies REQ:status-filter)

**Given** a project with one `Approved` plan and one `Completed` plan
**When** the user runs `specscore plan list --status approved`
**Then** only the `Approved` plan's slug is printed (case-insensitive match), and the `Completed` plan is excluded.

### AC: empty-match-exits-zero (verifies REQ:empty-listing-exit-zero)

**Given** a project with no plans in status `Deprecated`
**When** the user runs `specscore plan list --status Deprecated`
**Then** stdout is empty and the exit code is `0`.

### AC: fields-returns-yaml (verifies REQ:fields-forces-structured, REQ:fields-recognized-set)

**Given** a project with at least one plan
**When** the user runs `specscore plan list --fields status,source-feature`
**Then** the output is a YAML list where each entry has `slug`, `status`, and `source-feature` keys; and `--format text --fields status` upgrades to YAML rather than producing mixed text.

### AC: unknown-field-exits-2 (verifies REQ:fields-recognized-set)

**Given** a project with at least one plan
**When** the user runs `specscore plan list --fields bogus`
**Then** the command exits `2` and stderr names `bogus` as an unrecognized field.

## Open Questions

- Should `plan list` gain a `--source-feature <id>` filter to answer "which plans target this feature?", or is `--fields source-feature` plus `grep` adequate for now?

---
*This document follows the https://specscore.md/feature-specification*
