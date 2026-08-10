---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Lesson List

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/list?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/list?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/list?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/list?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Summary

`specscore lesson list` lists lesson slugs, one per line, sorted alphabetically. `--not-enforced` is the headline query — "what have we learned but not yet enforced?" — matching every lesson in `Recorded` **or** `Stated` (only `Enforced`, Tier 2, actually binds; a filter that matched only `Recorded` would silently miss every advisory-but-unenforced lesson sitting at `Stated`, which is most of them in practice). `--status` accepts one or more statuses, comma-separated and case-insensitive, and rejects an unrecognized value outright rather than silently matching nothing. `--min-recurred <N>` composes with either status filter so "which lessons have recurred and are still not enforced?" is `--not-enforced --min-recurred=1`, not eyeballing a listing. `--fields` and `--format` expose structured output (including the `recurred` count) for scripts.

## Synopsis

```
specscore lesson list [--not-enforced | --status <status>[,<status>...]] [--min-recurred <N>] [--fields <list>] [--format <text|yaml|json>] [--project <path>]
```

## Problem

Before this verb, "what have we learned but not yet enforced?" required opening `LESSONS-LEARNED.md`, reading every `**Status:**` line by eye, and trusting the hand-maintained "Open: needs to graduate" list was current. A flat, filterable, scriptable listing — mirroring [cli/plan/list](../../plan/list/README.md) and [cli/idea/list](../../idea/list/README.md) exactly — makes the query mechanical. A single-value, exact-match `--status` is not enough on its own: "not yet enforced" spans two of the ladder's three rungs (`Recorded` and `Stated`), and a query that only reaches one of them returns a confidently wrong "nothing needs attention" the moment any lesson sits at `Stated` — which the real `LESSONS-LEARNED.md` this feature formalizes shows is the common case, not the exception.

## Behavior

### Default listing

#### REQ: default-listing-pipeable

With no flags, `lesson list` MUST print every lesson's slug, one per line, sorted alphabetically, to stdout, with a single trailing newline and no extra blank lines. Output MUST be empty (not an error) when no lessons exist.

### Status filters

#### REQ: status-filter-selects

`--status <value>[,<value>...]` MUST restrict output to lessons whose `**Status:**` matches ANY of the given values, case-insensitively (each compared after trimming) — a union, not a single exact match. Empty parts from stray commas MUST be skipped silently. A filter matching zero lessons MUST print nothing and exit `0` — not an error. An unrecognized status name in the list MUST exit `2` (InvalidArgs) naming the offending value and the legal set — it MUST NOT be treated as "matches nothing," since a silently empty result reads as good news (nothing left to graduate) rather than a malformed query.

#### REQ: not-enforced-flag

`--not-enforced` MUST be accepted as a boolean flag equivalent to `--status=recorded,stated` — the headline query this verb exists to answer. `--status` and `--not-enforced` MUST be mutually exclusive: supplying both MUST exit `2` before any output.

### Recurrence filter

#### REQ: min-recurred-filter

`--min-recurred <N>` MUST restrict output to lessons whose recurrence count is at least `N` (default `0`, meaning no filter). Canonical counts derive from validated child Occurrences; compatibility flat counts use `**Recurred:**`. It MUST compose with either status filter (AND semantics), not replace it. A negative `N` MUST exit `2`.

### Recurrence surfaced in text output

#### REQ: text-shows-recurrence

In the default `text` format, a lesson with derived recurrence count `N > 0` MUST render as `<slug> (recurred N)` instead of the bare slug, so a recurring-but-not-yet-graduated lesson is visible without a separate query.

### Structured output

#### REQ: fields-returns-yaml

`--fields <comma-list>` (valid names: `status`, `recurred`, `date`, `owner`) MUST emit one YAML (or JSON with `--format json`) map per matching lesson, keyed by `slug` plus every requested field — every key present even when its value is empty (no `omitempty`). A `--format text` (or unset) combined with `--fields` MUST auto-upgrade to `yaml`, since field data cannot render as bare slug lines. An unrecognized field name MUST exit `2` naming the offending field.

## Parameters / Flags

| Flag | Required | Description |
|---|---|---|
| `--status` | No | Filter by one or more statuses, comma-separated, case-insensitive: `recorded`, `stated`, `enforced`, `withdrawn`, `superseded`. Mutually exclusive with `--not-enforced`. An unrecognized value exits `2`. |
| `--not-enforced` | No | The headline query: shorthand for `--status=recorded,stated`. Mutually exclusive with `--status`. |
| `--min-recurred` | No | Restrict to lessons with derived recurrence count `>= N` (default `0` = no filter). Composes with either status filter. |
| `--fields` | No | Comma-separated: `status`, `recurred`, `date`, `owner`. Upgrades output to structured form. |
| `--format` | No | `text` (default), `yaml`, `json`. |
| `--project` | No | Project root (autodetected). |

## Exit codes

| Code | Condition |
|---|---|
| `0` | Listing succeeded (possibly empty). |
| `2` | Invalid `--format`; an unrecognized `--fields` name; an unrecognized `--status` value; `--status` combined with `--not-enforced`; a negative `--min-recurred`. |

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

### AC: status-filter-is-exact-single-value (verifies REQ:status-filter-selects)

**Given** a lesson `stale-check` with `**Status:** Recorded`, a lesson `advisory-check` with `**Status:** Stated`, and a lesson `old-check` with `**Status:** Enforced`
**When** the user runs `specscore lesson list --status=recorded`
**Then** stdout is exactly `stale-check\n` — `advisory-check` (also unenforced, but a different rung) does NOT appear.

### AC: not-enforced-unions-recorded-and-stated (verifies REQ:not-enforced-flag)

**Given** the same three lessons as above
**When** the user runs `specscore lesson list --not-enforced`
**Then** stdout contains exactly `advisory-check` and `stale-check`, and does NOT contain `old-check`.

### AC: status-comma-list-unions (verifies REQ:status-filter-selects)

**When** the user runs `specscore lesson list --status=recorded,stated`
**Then** the result is identical to `--not-enforced`.

### AC: unrecognized-status-exits-2 (verifies REQ:status-filter-selects)

**When** the user runs `specscore lesson list --status=recorded,bogus`
**Then** the command exits `2`, naming `bogus`, with empty stdout — never an empty-but-successful result.

### AC: status-and-not-enforced-conflict (verifies REQ:not-enforced-flag)

**When** the user runs `specscore lesson list --status=recorded --not-enforced`
**Then** the command exits `2` before producing any output.

### AC: empty-match-exits-zero (verifies REQ:status-filter-selects)

**Given** no lesson with `**Status:** Withdrawn`
**When** the user runs `specscore lesson list --status=Withdrawn`
**Then** the command exits `0` with empty stdout.

### AC: min-recurred-composes-with-not-enforced (verifies REQ:min-recurred-filter)

**Given** a `Stated` lesson `flaky-check` with `**Recurred:** 2` and a `Stated` lesson `quiet-check` with `**Recurred:** 0`
**When** the user runs `specscore lesson list --not-enforced --min-recurred=1`
**Then** stdout is exactly `flaky-check\n`.

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
