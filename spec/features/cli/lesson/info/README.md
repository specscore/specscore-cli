---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: Lesson Info

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/info?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/info?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/info?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/info?op=request-change) |
**Status:** Draft
**Source Ideas:** —

## Summary

`specscore lesson info <slug>` returns a single lesson's metadata (status, date, owner, recurrence count, successor when superseded) together with which of the four required sections are present versus missing. Default output is YAML; `--format` selects `json` or `text`.

## Synopsis

```
specscore lesson info <slug> [--format <yaml|json|text>] [--project <path>]
```

## Problem

Inspecting a single lesson otherwise means opening the file and reading it by eye — including checking, section by section, whether the load-bearing `## Process gap` and `## Enforcement` sections actually exist. `info` mirrors [cli/plan/info](../../plan/info/README.md) and [cli/idea/info](../../idea/info/README.md) so the same query shape applies across kinds.

## Behavior

### Metadata and section coverage

#### REQ: info-returns-metadata

`lesson info <slug>` MUST return the lesson's `slug`, `status`, `date`, `owner`, `recurred` (count), and `superseded_by` (empty unless the lesson is `Superseded`) as structured fields — every key present even when its value is empty (no `omitempty`).

#### REQ: info-returns-section-coverage

The output MUST include `sections` (the required sections present, in canonical order) and `missing_sections` (the required sections absent). A lesson with all four sections present has an empty `missing_sections`.

### Slug resolution

#### REQ: slug-resolution

`<slug>` MUST resolve to `spec/lessons/<slug>.md`. A slug that does not resolve MUST exit `3` (NotFound) naming the requested slug, with no partial stdout.

## Parameters

| Name | Required | Description |
|---|---|---|
| `slug` | Yes | Lesson slug — resolves to `spec/lessons/<slug>.md`. |
| `--format` | No | `yaml` (default), `json`, `text`. |
| `--project` | No | Project root (autodetected). |

## Exit codes

| Code | Condition |
|---|---|
| `0` | Metadata returned. |
| `2` | Missing/extra positional argument, or invalid `--format`. |
| `3` | No lesson at `spec/lessons/<slug>.md`. |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [lesson (CLI group)](../README.md) | Parent group; read-only query verb. |
| [cli/plan/info](../../plan/info/README.md) | Structural sibling this verb's format/exit-code contract mirrors directly. |

## Acceptance Criteria

### AC: info-returns-metadata (verifies REQ:info-returns-metadata)

**Given** `spec/lessons/kinder-fake.md` with `**Status:** Stated`, `**Date:** 2026-07-25`, `**Owner:** alex`, `**Recurred:** 2`
**When** the user runs `specscore lesson info kinder-fake`
**Then** stdout is YAML containing `slug: kinder-fake`, `status: Stated`, `date: "2026-07-25"`, `owner: alex`, and `recurred: 2`.

### AC: info-returns-section-coverage (verifies REQ:info-returns-section-coverage)

**Given** a lesson whose body carries only `## Incident` and `## Process gap`
**When** the user runs `specscore lesson info <slug>`
**Then** `missing_sections` lists `Check` and `Enforcement`, and `sections` lists `Incident` and `Process gap`.

### AC: not-found-exits-3 (verifies REQ:slug-resolution)

**Given** no lesson named `does-not-exist`
**When** the user runs `specscore lesson info does-not-exist`
**Then** the command exits `3` naming `does-not-exist`, with no partial stdout.

### AC: superseded-by-surfaced (verifies REQ:info-returns-metadata)

**Given** a lesson in `**Status:** Superseded` with `**Superseded By:** newer-lesson`
**When** the user runs `specscore lesson info <slug>`
**Then** the output includes `superseded_by: newer-lesson`.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
