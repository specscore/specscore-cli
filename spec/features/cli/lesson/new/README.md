---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Lesson New

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/new?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/new?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/new?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/new?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Summary

`specscore lesson new <slug>` scaffolds a lint-clean Lesson artifact at `spec/lessons/<slug>.md`: the `format:`/`status:` frontmatter, the body-metadata header (`**Status:** Recorded`, `**Date:**`, `**Owner:**`, `**Recurred:** 0`), the four required sections (`## Incident`, `## Process gap`, `## Check`, `## Enforcement`) with `<!-- TODO: ... -->` prompts, and the adherence footer. No flag beyond `<slug>` is required.

## Synopsis

```
specscore lesson new <slug> [--title <text>] [--owner <id>] [--force] [--project <path>]
```

## Problem

The value of a lessons log is that writing an entry is nearly free — the moment recording one becomes a chore, lessons silently go unwritten, and an unwritten lesson is worse than a scruffy one. A verb that scaffolds a lint-clean Lesson from a single positional argument, with every other field optional and defaulted, keeps that friction at zero while still guaranteeing the artifact carries the structure the enforcement ladder depends on.

## Behavior

### Slug argument

#### REQ: slug-required

`<slug>` MUST be supplied. Absence MUST exit `2` (InvalidArgs).

#### REQ: slug-format

The slug MUST be lowercase, hyphen-separated, URL-safe, and MUST NOT contain `/`. An invalid slug MUST exit `2` naming the offending slug.

### Scaffold content

#### REQ: scaffolds-lint-clean

The scaffold MUST emit a single file `spec/lessons/<slug>.md` (not a directory) carrying the body-metadata header (`**Status:** Recorded`, `**Date:**`, `**Owner:**`, `**Recurred:** 0`), the four required sections (`## Incident`, `## Process gap`, `## Check`, `## Enforcement`, each with a `<!-- TODO: ... -->` prompt), and the adherence footer. `specscore spec lint` immediately afterwards MUST report no new error-severity violations outside the scaffolded file itself — the verb runs an internal `spec lint --fix` pass (see [REQ: index-row-materialized](#req-index-row-materialized)) specifically so this holds without a separate step.

#### REQ: emits-frontmatter

The scaffold MUST emit, per the [artifact-frontmatter-convention](../../../../../../specscore/spec/features/artifact-frontmatter-convention/README.md) feature, a `format:` frontmatter field carrying `https://specscore.md/lesson-specification` and a `status:` field mirroring the initial body `**Status:** Recorded`. The footer URL and `format:` MUST agree on creation.

### Ancestor index and lessons-index materialization

#### REQ: ancestor-indexes-materialized

When `lesson new` writes `spec/lessons/<slug>.md`, the command MUST also materialize `spec/README.md` and `spec/lessons/README.md` when they do not already exist, using the same templates as `specscore init`. Existing files MUST be left untouched. The lesson file MUST NOT be written until both ancestor indexes are in place.

#### REQ: index-row-materialized

After writing the lesson file, the verb MUST run `specscore spec lint --fix` scoped to the project, so the new lesson's row is inserted into `spec/lessons/README.md` (rule L-003) in the same invocation — no separate `spec lint --fix` step is needed to keep the tree lint-clean. If the fix pass fails, the partial lesson file MUST be removed and the command MUST exit `10`. The verb then re-runs lint (without `--fix`) and MUST exit `10` if any error-severity violation remains that touches the new file or any file under `lessons/`.

### Overwrite behavior

#### REQ: no-clobber-default

If `spec/lessons/<slug>.md` already exists, the command MUST exit `1` (Conflict) naming the path, unless `--force` is supplied. No partial write may occur before the collision check.

## Parameters

| Name | Required | Description |
|---|---|---|
| `slug` | Yes | Lesson slug — becomes the file name. |
| `--title` | No | Lesson title (defaults to title-cased slug). |
| `--owner` | No | Owner/author (defaults to `$USER`). |
| `--force` | No | Overwrite an existing lesson file. |

## Exit codes

| Code | Condition |
|---|---|
| `0` | Lesson file created (and lint-clean, including its index row). |
| `1` | File already exists and `--force` not supplied. |
| `2` | Missing/invalid `slug`. |
| `10` | Unexpected I/O failure, the internal `spec lint --fix` pass failed, or a remaining error-severity violation touches the new file after the fix pass. |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [lesson (CLI group)](../README.md) | Parent group; `new` is the group's only mutating-creation verb. |
| [cli/plan/new](../../plan/new/README.md) | Sibling create verb whose slug/no-clobber/ancestor-index/lint-clean contract this verb mirrors. |
| [cli/idea/new](../../idea/new/README.md) | Sibling create verb whose internal `spec lint --fix` pass (so the freshly scaffolded artifact's index row lands in the same invocation) this verb mirrors. |
| [cli/spec/lint/lesson-rules](../../spec/lint/lesson-rules/README.md) | Owns the `L-001`–`L-004` rules this verb's scaffold and internal fix pass satisfy. |

## Acceptance Criteria

### AC: scaffolded-lesson-is-lint-clean (verifies REQ:scaffolds-lint-clean, REQ:index-row-materialized)

`specscore lesson new kinder-fake` creates `spec/lessons/kinder-fake.md` AND inserts its row into `spec/lessons/README.md` in the same invocation. `specscore spec lint` immediately afterwards reports no error-severity violations anywhere in the tree, even though every section carries only a `<!-- TODO: ... -->` prompt.

### AC: scaffold-emits-frontmatter (verifies REQ:emits-frontmatter)

A lesson scaffolded by `lesson new` carries `format: https://specscore.md/lesson-specification` and `status: Recorded` mirroring its body `**Status:** Recorded`, and its adherence-footer URL matches `format:`.

### AC: existing-file-conflict (verifies REQ:no-clobber-default)

Running the command twice for the same slug without `--force` exits `1` on the second run and leaves the existing file untouched. With `--force`, the second run overwrites and exits `0`.

### AC: invalid-slug-rejected (verifies REQ:slug-format)

`specscore lesson new Bad_Slug` exits `2` naming the offending slug. No file is created.

### AC: ancestor-indexes-materialized (verifies REQ:ancestor-indexes-materialized)

**Given** a project with `specscore.yaml` but no `spec/lessons/` tree
**When** the user runs `specscore lesson new my-lesson`
**Then** `spec/README.md`, `spec/lessons/README.md`, and `spec/lessons/my-lesson.md` are created; re-running the command for a different slug leaves the pre-existing indexes' prologue untouched.

## Open Questions

- Should `--title`/`--owner` become the only optional flags forever, or is there value in a `--note "<free text>"` flag that seeds the `## Incident` section directly, shaving one more edit off the zero-friction path? Deferred until real usage shows agents consistently editing the same section by hand immediately after `new`.

---
*This document follows the https://specscore.md/feature-specification*
