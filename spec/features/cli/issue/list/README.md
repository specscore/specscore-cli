---
format: https://specscore.md/feature-specification
status: Implementing
---

# Feature: Issue List

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/issue/list?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/issue/list?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/issue/list?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/issue/list?op=request-change) |

**Status:** Implementing

## Summary

`specscore issue list` scans all Issue artifacts from both `spec/issues/` and `spec/features/*/issues/` and prints a sorted, filterable table. Supports status, severity, and Feature filters, with text, JSON, and YAML output formats.

## Synopsis

```
specscore issue list [--status <value>] [--severity <value>] [--feature <slug>] [--format <text|json|yaml>] [--project <path>]
```

## Problem

Issue artifacts live at two different location patterns. Discovering all issues requires scanning both `spec/issues/` and every `spec/features/*/issues/` directory. A CLI command that aggregates, sorts, and filters the full issue corpus provides a single entry point for automation, triage dashboards, and agent workflows without re-implementing the scan logic.

## Behavior

### Scan scope

#### REQ: dual-location-scan

The command MUST scan both `spec/issues/*.md` and `spec/features/*/issues/*.md` for files with `type: issue` in frontmatter. Files that do not declare `type: issue` MUST be skipped silently.

### Default output

#### REQ: default-text-table

The default output format MUST be a text table with columns: `Slug`, `Title`, `Status`, `Severity`, `Feature`. The `Title` column contains the H1 text minus the `Issue: ` prefix. The `Severity` column displays the severity value or `—` when absent/unset. The `Feature` column displays the owning Feature slug for Feature-scoped issues or `—` for root-level issues.

### Sort order

#### REQ: sort-order

Output MUST be sorted by: (1) status priority — `open` first, then `investigating`, then `resolved` and `rejected` — then (2) `captured_at` descending within the same status group. This surfaces active issues at the top.

### Filters

#### REQ: status-filter

`--status <value>` MUST return only issues whose `status` field equals the given value. Valid values: `open`, `investigating`, `resolved`, `rejected`. Unknown values MUST exit `2` (InvalidArgs).

#### REQ: severity-filter

`--severity <value>` MUST return only issues whose `severity` field equals the given value. Valid values: `low`, `medium`, `high`, `critical`, `unset`. Unknown values MUST exit `2`.

#### REQ: feature-filter

`--feature <slug>` MUST return only issues scoped to the given Feature (at `spec/features/<slug>/issues/`). Root-level issues are excluded. If no issues exist under the given Feature, the output is empty (not an error). If `spec/features/<slug>/` does not exist at all, behavior is the same — empty result, not an error.

### Output formats

#### REQ: format-text

`--format text` (default) emits the table described by REQ `default-text-table`.

#### REQ: format-json

`--format json` emits a JSON array of objects, each with keys: `slug`, `title`, `status`, `severity` (string or null), `feature` (string or null), `captured_at`, `captured_by`.

#### REQ: format-yaml

`--format yaml` emits the same shape as JSON but YAML-encoded.

### Empty result

#### REQ: empty-result-success

When no issues match (either because none exist or all are filtered out), the command MUST exit `0` with appropriate empty output: an empty table for text, `[]` for JSON, empty list for YAML.

## Parameters

None. All inputs are flags.

## Flags

| Flag | Required | Description |
|---|---|---|
| `--status` | No | Filter by status: `open`, `investigating`, `resolved`, `rejected`. |
| `--severity` | No | Filter by severity: `low`, `medium`, `high`, `critical`, `unset`. |
| `--feature` | No | Filter to issues under `spec/features/<slug>/issues/` only. |
| `--format` | No | Output format: `text` (default), `json`, `yaml`. |
| `--project` | No | Project root. Autodetected per CLI conventions. |

## Exit codes

| Code | Condition |
|---|---|
| `0` | Listing printed (even if empty) |
| `2` | Invalid `--status`, `--severity`, or `--format` value |
| `10` | Unexpected I/O failure while scanning |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [issue-artifact-type](https://github.com/specscore/specstudio-skills/blob/main/spec/features/issue-artifact-type/README.md) | Defines the frontmatter schema and dual-location convention that this command reads. |
| [issue (CLI group)](../README.md) | Parent group. Contents table includes this sub-feature. |
| [cli/spec/lint/issue-rules](../../spec/lint/issue-rules/README.md) | Issue detection and path patterns are shared with the lint engine's artifact discovery. |

## Acceptance Criteria

### AC: lists-issues-from-both-locations

**Requirements:** cli/issue/list#req:dual-location-scan

**Given** issues at `spec/issues/foo.md` and `spec/features/auth/issues/bar.md`
**When** `specscore issue list` is run
**Then** both `foo` and `bar` appear in the output, with `bar` showing `auth` in the Feature column and `foo` showing `—`

### AC: sorted-by-status-then-captured-at

**Requirements:** cli/issue/list#req:sort-order

**Given** three issues: `a` (status: resolved, captured_at: 2026-01-01), `b` (status: open, captured_at: 2026-01-02), `c` (status: open, captured_at: 2026-01-03)
**When** `specscore issue list` is run
**Then** the output order is `c`, `b`, `a` — open issues first (newest first), then resolved

### AC: status-filter-applied

**Requirements:** cli/issue/list#req:status-filter

**Given** issues with statuses `open`, `investigating`, and `resolved`
**When** `specscore issue list --status open` is run
**Then** only the `open` issue appears

### AC: severity-filter-applied

**Requirements:** cli/issue/list#req:severity-filter

**Given** issues with severities `high`, `low`, and absent
**When** `specscore issue list --severity high` is run
**Then** only the issue with `severity: high` appears

### AC: feature-filter-applied

**Requirements:** cli/issue/list#req:feature-filter

**Given** `spec/issues/root.md` and `spec/features/auth/issues/scoped.md`
**When** `specscore issue list --feature auth` is run
**Then** only `scoped` appears; `root` is excluded

### AC: json-format-output

**Requirements:** cli/issue/list#req:format-json

**Given** at least one issue exists
**When** `specscore issue list --format json` is run
**Then** stdout is valid JSON: an array of objects each with `slug`, `title`, `status`, `severity`, `feature`, `captured_at`, `captured_by` keys

### AC: empty-result-exits-zero

**Requirements:** cli/issue/list#req:empty-result-success

**Given** no issue artifacts exist in the project
**When** `specscore issue list` is run
**Then** the command exits `0` with an empty table (text) or `[]` (JSON)

### AC: invalid-status-filter-rejected

**Requirements:** cli/issue/list#req:status-filter

**Given** any valid project
**When** `specscore issue list --status banana` is run
**Then** the command exits `2` with a message listing the valid status values

## Open Questions

- Should `--severity` filter accept `none` or `absent` to match issues where severity is not set?
- Should a `--sort` flag be exposed for custom sort orders, or is the fixed sort-order adequate?

---
*This document follows the https://specscore.md/feature-specification*
