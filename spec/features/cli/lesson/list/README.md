---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: Lesson List

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/list?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/list?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/list?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/list?op=request-change) |
**Status:** Draft
**Source Ideas:** —

## Summary

`specscore lesson list [--status=<status>]` lists lesson slugs, one per line, sorted alphabetically. `--status` filters case-insensitively on the exact status value, so `--status=recorded` answers the single most valuable question a lessons log must answer — "what have we learned but not yet enforced?" — in one command instead of a hand read of prose. `--fields` and `--format` expose structured output (including the `recurred` count) for scripts.

## Synopsis

```
specscore lesson list [--status <status>] [--fields <list>] [--format <text|yaml|json>] [--project <path>]
```

## Problem

Before this verb, "what have we learned but not yet enforced?" required opening `LESSONS-LEARNED.md`, reading every `**Status:**` line by eye, and trusting the hand-maintained "Open: needs to graduate" list was current. A flat, filterable, scriptable listing — mirroring [cli/plan/list](../../plan/list/README.md) and [cli/idea/list](../../idea/list/README.md) exactly — makes the query mechanical.

## Behavior

### Default listing

#### REQ: default-listing-pipeable

With no flags, `lesson list` MUST print every lesson's slug, one per line, sorted alphabetically, to stdout, with a single trailing newline and no extra blank lines. Output MUST be empty (not an error) when no lessons exist.

### Status filter

#### REQ: status-filter-selects

`--status <value>` MUST restrict output to lessons whose `**Status:**` matches `value` case-insensitively (exact match after trimming). A filter matching zero lessons MUST print nothing and exit `0` — not an error.

### Recurrence surfaced in text output

#### REQ: text-shows-recurrence

In the default `text` format, a lesson with `**Recurred:** N` where `N > 0` MUST render as `<slug> (recurred N)` instead of the bare slug, so a recurring-but-not-yet-graduated lesson is visible without a separate query.

### Structured output

#### REQ: fields-returns-yaml

`--fields <comma-list>` (valid names: `status`, `recurred`, `date`, `owner`) MUST emit one YAML (or JSON with `--format json`) map per matching lesson, keyed by `slug` plus every requested field — every key present even when its value is empty (no `omitempty`). A `--format text` (or unset) combined with `--fields` MUST auto-upgrade to `yaml`, since field data cannot render as bare slug lines. An unrecognized field name MUST exit `2` naming the offending field.

## Parameters / Flags

| Flag | Required | Description |
|---|---|---|
| `--status` | No | Filter by status (case-insensitive exact match): `recorded`, `stated`, `enforced`, `withdrawn`, `superseded`. |
| `--fields` | No | Comma-separated: `status`, `recurred`, `date`, `owner`. Upgrades output to structured form. |
| `--format` | No | `text` (default), `yaml`, `json`. |
| `--project` | No | Project root (autodetected). |

## Exit codes

| Code | Condition |
|---|---|
| `0` | Listing succeeded (possibly empty). |
| `2` | Invalid `--format` or an unrecognized `--fields` name. |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [lesson (CLI group)](../README.md) | Parent group; read-only query verb. |
| [cli/plan/list](../../plan/list/README.md) | Structural sibling this verb's flag/output contract mirrors directly. |

## Acceptance Criteria

### AC: default-listing-pipeable (verifies REQ:default-listing-pipeable)

**Given** lessons `zebra-fix`, `alpha-check` in a project
**When** the user runs `specscore lesson list`
**Then** stdout is exactly `alpha-check\nzebra-fix\n`.

### AC: status-filter-recorded (verifies REQ:status-filter-selects)

**Given** a lesson `stale-check` with `**Status:** Recorded` and a lesson `old-check` with `**Status:** Enforced`
**When** the user runs `specscore lesson list --status=recorded`
**Then** stdout is exactly `stale-check\n`.

### AC: empty-match-exits-zero (verifies REQ:status-filter-selects)

**Given** no lesson with `**Status:** Withdrawn`
**When** the user runs `specscore lesson list --status=Withdrawn`
**Then** the command exits `0` with empty stdout.

### AC: text-shows-recurrence (verifies REQ:text-shows-recurrence)

**Given** a lesson `flaky-check` with `**Recurred:** 2`
**When** the user runs `specscore lesson list`
**Then** stdout includes the line `flaky-check (recurred 2)`.

### AC: fields-returns-yaml (verifies REQ:fields-returns-yaml)

**Given** a lesson with `**Status:** Stated` and `**Recurred:** 1`
**When** the user runs `specscore lesson list --fields status,recurred`
**Then** stdout is YAML containing `status: Stated` and `recurred: "1"` alongside the `slug` key.

### AC: unknown-field-exits-2 (verifies REQ:fields-returns-yaml)

**When** the user runs `specscore lesson list --fields bogus`
**Then** the command exits `2` naming `bogus`.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
