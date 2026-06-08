---
format: https://specscore.md/feature-specification
status: Implementing
---

# Feature: Issue New

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/issue/new?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/issue/new?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/issue/new?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/issue/new?op=request-change) |

**Status:** Implementing
**Source Ideas:** —

## Summary

`specscore issue new <slug>` scaffolds a lint-clean Issue artifact at `spec/issues/<slug>.md` (root) or `spec/features/<feature>/issues/<slug>.md` (Feature-scoped). Each required section is emitted with an HTML-comment prompt. The artifact is created with `status: open` and a generated RFC 3339 `captured_at` timestamp.

## Synopsis

```
specscore issue new <slug> [--feature <feature-slug>] [--severity <level>] [--affected-component <feature-slug>] [--captured-by <id>] [--title <text>] [--force] [--project <path>]
```

## Problem

Issues have a strict required-sections contract enforced by lint rules `I-001`–`I-015`. Hand-writing them means remembering the `# Issue:` H1, three required H2 sections in order, RFC 3339 timestamps, and frontmatter schema. The scaffolder produces a skeleton that is lint-clean on creation, so authors edit content rather than fight structure.

## Behavior

### Slug argument

The `<slug>` positional argument becomes the file name and the Issue's declared `slug` frontmatter field.

#### REQ: slug-required

`<slug>` MUST be supplied. Absence MUST exit `2` (InvalidArgs).

#### REQ: slug-format

The slug MUST satisfy the Issue slug-derivation algorithm: lowercase, hyphen-separated, `[a-z0-9-]` only, no leading/trailing hyphens, max 60 characters. Invalid slugs MUST exit `2` with a message naming the offending slug.

### Target location

#### REQ: root-location-default

Without `--feature`, the file MUST be written to `spec/issues/<slug>.md`. The `spec/issues/` directory MUST be created if absent.

#### REQ: feature-scoped-location

When `--feature <feature-slug>` is supplied, the file MUST be written to `spec/features/<feature-slug>/issues/<slug>.md`. The `issues/` subdirectory MUST be created if absent. If `spec/features/<feature-slug>/README.md` does not exist, the command MUST exit `3` (NotFound) with a message naming the missing parent Feature.

### Frontmatter generation

#### REQ: frontmatter-schema

The generated frontmatter MUST include: `type: issue`, `slug: <slug>`, `status: open`, `captured_at: <RFC 3339 timestamp>`, `captured_by: <value>`. The `captured_at` timestamp MUST be generated at invocation time in UTC. The `captured_by` value MUST default to `$USER` when `--captured-by` is not supplied.

#### REQ: optional-frontmatter-flags

When `--severity` is supplied, it MUST be included in frontmatter. Valid values: `low`, `medium`, `high`, `critical`. Invalid values MUST exit `2`. When `--affected-component` is supplied, it MUST be included in frontmatter; the referenced Feature MUST exist (same validation as `--feature`). When neither is supplied, the fields are omitted from frontmatter.

### Body generation

#### REQ: body-skeleton

The generated body MUST include: an `# Issue: <Title>` H1 (where title defaults to title-cased slug when `--title` is not supplied), then `## Description`, `## Steps to Reproduce`, `## Expected vs Actual` H2 sections in that order. Each section MUST contain an HTML-comment prompt (`<!-- TODO: ... -->`) as placeholder content.

### Overwrite behavior

#### REQ: no-clobber-default

If the target file already exists, the command MUST exit `1` (Conflict) with a message naming the path, unless `--force` is supplied. No partial write may occur before the collision check.

### Post-scaffolding lint sync

#### REQ: lint-fix-after-scaffold

After writing the Issue file, the command MUST run `specscore spec lint --fix` to create or update the corresponding `issues/README.md` index. This ensures the index stays in sync without a separate manual step.

### Ancestor index materialization

#### REQ: ancestor-indexes-materialized

When writing the first Issue in a directory, the command MUST materialize any missing ancestor index READMEs (`spec/README.md`, `spec/issues/README.md`, or the Feature-scoped `spec/features/<feature>/issues/README.md`) before writing the Issue file. Existing files MUST be left untouched.

## Parameters

| Name | Required | Description |
|---|---|---|
| `slug` | Yes | Issue slug — becomes the file name and frontmatter `slug` value. |

## Flags

| Flag | Required | Description |
|---|---|---|
| `--feature` | No | Target Feature slug. Creates the issue under `spec/features/<slug>/issues/`. |
| `--severity` | No | Initial severity: `low`, `medium`, `high`, `critical`. |
| `--affected-component` | No | Feature slug this issue affects. |
| `--captured-by` | No | Author identifier. Defaults to `$USER`. |
| `--title` | No | Human-readable title. Defaults to title-cased slug. |
| `--force` | No | Overwrite an existing file instead of exiting `1`. |
| `--project` | No | Project root. Autodetected per CLI conventions. |

## Exit codes

| Code | Condition |
|---|---|
| `0` | Issue file created and index synced |
| `1` | File already exists and `--force` not supplied |
| `2` | Missing or invalid `slug`, invalid flag value |
| `3` | Parent Feature not found (when `--feature` or `--affected-component` references a nonexistent Feature) |
| `10` | Unexpected I/O failure while writing |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [issue-artifact-type](https://github.com/specscore/specstudio-skills/blob/main/spec/features/issue-artifact-type/README.md) | Source of truth for required Issue sections, slug rules, and frontmatter schema. |
| [issue (CLI group)](../README.md) | Parent group. Inherits the lint-clean-on-mutation guarantee. |
| [cli/spec/lint/issue-rules](../../spec/lint/issue-rules/README.md) | Lint rules that validate the scaffolded output. `--fix` materializes indexes. |

## Acceptance Criteria

### AC: scaffolded-issue-is-lint-clean

**Requirements:** cli/issue/new#req:body-skeleton, cli/issue/new#req:frontmatter-schema

**Given** a project with `specscore.yaml`
**When** `specscore issue new menu-crashes` is run with no other flags
**Then** `spec/issues/menu-crashes.md` is created with `status: open`, a valid RFC 3339 `captured_at`, and `specscore spec lint` immediately afterwards reports no violations on the new file

### AC: feature-scoped-issue-created

**Requirements:** cli/issue/new#req:feature-scoped-location

**Given** a project with `spec/features/sidekick-capture/README.md`
**When** `specscore issue new bar --feature sidekick-capture` is run
**Then** `spec/features/sidekick-capture/issues/bar.md` is created with valid frontmatter and body

### AC: missing-parent-feature-rejected

**Requirements:** cli/issue/new#req:feature-scoped-location

**Given** a project with no `spec/features/nonexistent/README.md`
**When** `specscore issue new bar --feature nonexistent` is run
**Then** the command exits `3` with a message naming the missing Feature, and no file is created

### AC: existing-file-conflict

**Requirements:** cli/issue/new#req:no-clobber-default

**Given** `spec/issues/foo.md` already exists
**When** `specscore issue new foo` is run without `--force`
**Then** the command exits `1` and the existing file is untouched. With `--force`, the file is overwritten and exit is `0`

### AC: invalid-slug-rejected

**Requirements:** cli/issue/new#req:slug-format

**Given** no preconditions
**When** `specscore issue new My_Bad_Slug!` is run
**Then** the command exits `2` with a message naming the invalid characters, and no file is created

### AC: severity-flag-included

**Requirements:** cli/issue/new#req:optional-frontmatter-flags

**Given** a valid project
**When** `specscore issue new foo --severity high` is run
**Then** the generated frontmatter includes `severity: high`

### AC: invalid-severity-rejected

**Requirements:** cli/issue/new#req:optional-frontmatter-flags

**Given** a valid project
**When** `specscore issue new foo --severity extreme` is run
**Then** the command exits `2` with a message listing valid severity values

### AC: index-synced-after-scaffold

**Requirements:** cli/issue/new#req:lint-fix-after-scaffold, cli/issue/new#req:ancestor-indexes-materialized

**Given** a project with `specscore.yaml` but no `spec/issues/` directory
**When** `specscore issue new my-issue` is run
**Then** `spec/issues/README.md` exists after the command completes and contains the new issue in its Contents table

## Open Questions

- Should `issue new` accept an `--edit` flag that opens the scaffolded file in `$EDITOR` after writing?
- Should `--captured-by` default to GitHub username (from `gh auth status`) instead of `$USER`?

---
*This document follows the https://specscore.md/feature-specification*
