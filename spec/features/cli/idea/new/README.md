---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: Idea New

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/idea/new?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/idea/new?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/idea/new?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/idea/new?op=request-change) |
>
> **AI skill:** [GitHub](https://github.com/specscore/ai-plugin-specscore/blob/main/skills/idea/references/new.md) · [local](../../../../../../ai-plugin-specscore/skills/idea/references/new.md) — if this command's CLI signature or behavior changes, update the linked skill to keep agents in sync.

**Status:** Stable
**Source Ideas:** —

## Summary

`specscore idea new <slug>` scaffolds a lint-clean Idea artifact at `spec/ideas/<slug>.md`. Each required section is emitted with an HTML-comment prompt describing what belongs there. Content can be supplied via flags or gathered interactively with `-i`.

## Synopsis

```
specscore idea new <slug> [--title <text>] [--owner <id>] [--hmw <text>] [--context <text>] [--recommended-direction <text>] [--mvp <text>] [--not-doing "<thing> — <reason>" ...] [--type <feature-request|change-request>] [--targets <feature-slug>] [--phase <text>] [--interactive] [--force] [--project <path>]
```

## Problem

Ideas have a strict required-sections contract defined by the [idea](../../../idea/README.md) feature. Hand-writing them means remembering HMW framing, `Not Doing` entries, owner attribution, and status rules. The scaffolder produces a skeleton that is lint-clean on creation, so authors edit content rather than fight structure.

## Behavior

### Slug argument

The `<slug>` positional argument becomes the file name (`spec/ideas/<slug>.md`) and the Idea's declared `id`.

#### REQ: slug-required

`<slug>` MUST be supplied. Absence MUST exit `2` (InvalidArgs).

#### REQ: slug-format

The slug MUST satisfy the Idea slug-format rule: lowercase, hyphen-separated, URL-safe characters only. Invalid slugs MUST exit `2` with a message naming the offending slug.

### Content sources

Content for required fields can come from flags, interactive stdin prompts, or fallback defaults.

| Flag | Target section / field |
|---|---|
| `--title` | Idea title (defaults to title-cased slug) |
| `--owner` | Owner / author (defaults to `$USER`) |
| `--hmw` | Problem Statement sentence |
| `--context` | Context section body |
| `--recommended-direction` | Recommended Direction body |
| `--mvp` | MVP Scope body |
| `--not-doing` | Repeatable `Not Doing` entry (format: `"<thing> — <reason>"`) |
| `--type` | Idea type: `feature-request` (default) or `change-request` |
| `--targets` | Target feature slug (required when `--type=change-request`) |
| `--phase` | Optional lifecycle phase to pre-populate in the header |

#### REQ: interactive-mode

`-i` / `--interactive` MUST prompt for each field on stdin, using the flag value as the default when supplied. Non-interactive runs use flag values plus defaults without prompting.

#### REQ: sensible-defaults

When a field has no flag value and the command is not interactive, the scaffolder MUST emit an HTML-comment prompt (`<!-- TODO: ... -->`) in place of the missing content. The resulting file MUST still be lint-clean — prompts are valid Markdown and do not break required-sections checks.

### Change-request mode

When `--type=change-request` is supplied with `--targets=<feature-slug>`, the artifact is scaffolded at `spec/features/<feature-slug>/proposals/<slug>.md` instead of the default Idea path. The title is prefixed with `# Proposal:` and the header includes `**Type:** change-request` and `**Targets:** <feature-slug>` fields. The target feature directory must exist.

#### REQ: change-request-location

When `--type=change-request --targets=<feature-slug>` is supplied, the file MUST be written to `spec/features/<feature-slug>/proposals/<slug>.md` (creating the `proposals/` directory if absent). The `--targets` flag MUST be required when `--type=change-request` and MUST be rejected when `--type=feature-request` (or absent).

#### REQ: change-request-container-is-reserved

The generated `proposals/` directory is a reserved change-request container, not a child Feature. It MUST NOT require its own `README.md` or a row in the parent Feature's Contents table. Proposal files inside it remain subject to every Idea and change-request lint rule. A newly scaffolded change request MUST leave the complete spec tree lint-clean except for violations that already existed before the command.

### Phase pre-population

The optional `--phase` flag pre-populates a `**Phase:**` header field in the generated artifact. When omitted, no Phase field is emitted.

### Overwrite behavior

By default, the command MUST refuse to overwrite an existing file. `--force` opts in to overwrite.

#### REQ: no-clobber-default

If `spec/ideas/<slug>.md` already exists, the command MUST exit `1` (Conflict) with a message naming the path, unless `--force` is supplied. No partial write may occur before the collision check.

### Ancestor index materialization

`idea new` is the entry point to writing the first Idea in a project. The Idea file alone is not enough — `spec/` and `spec/ideas/` each require a `README.md` for `spec lint` to be clean. Authors who skip `specscore init` (or who land in a partially-scaffolded tree) MUST still end up with a lint-clean tree after a single `idea new` invocation, modulo violations in the new Idea file itself.

#### REQ: ancestor-indexes-materialized

When `idea new` writes `spec/ideas/<slug>.md`, the command MUST also materialize `spec/README.md` and `spec/ideas/README.md` when they do not already exist, using the same templates as `specscore init` (project-aware view-link, canonical headings, adherence-footer URL on the ideas index). Existing files MUST be left untouched — this step is idempotent. The Idea file itself MUST NOT be written until both ancestor indexes are in place, so that a failure to materialize them does not leave a half-scaffolded state.

## Parameters

| Name | Required | Description |
|---|---|---|
| `slug` | Yes | Idea slug — becomes the file name. |

## Exit codes

| Code | Condition |
|---|---|
| `0` | Idea file created |
| `1` | File already exists and `--force` not supplied |
| `2` | Missing or invalid `slug`, invalid flag value |
| `10` | Unexpected I/O failure while writing |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [idea](../../../idea/README.md) | Source of truth for required Idea sections, slug rules, and lint checks. |
| [idea (CLI group)](../README.md) | Parent group. Inherits the lint-clean-on-create guarantee. |

## Acceptance Criteria

### AC: scaffolded-idea-is-lint-clean

**Requirements:** cli/idea/new#req:sensible-defaults

`specscore idea new my-idea` with no other flags creates `spec/ideas/my-idea.md`. `specscore spec lint` immediately afterwards reports no new violations, even though several fields contain `<!-- TODO: ... -->` prompts.

### AC: scaffolded-change-request-is-lint-clean

**Requirements:** cli/idea/new#req:change-request-location, cli/idea/new#req:change-request-container-is-reserved, cli/idea/README#req:lint-clean-on-create

Given a lint-clean project with an `auth` Feature and no existing `auth/proposals/` directory, `specscore idea new add-mfa --type change-request --targets auth` creates `spec/features/auth/proposals/add-mfa.md`. A subsequent full `specscore spec lint` reports no error for the `proposals/` container, while continuing to validate `add-mfa.md` as a change-request Idea.

### AC: existing-file-conflict

**Requirements:** cli/idea/new#req:no-clobber-default

Running the command twice for the same slug, without `--force`, exits `1` on the second run and leaves the existing file untouched. With `--force`, the second run overwrites and exits `0`.

### AC: invalid-slug-rejected

**Requirements:** cli/idea/new#req:slug-format

`specscore idea new My_Idea` exits `2` with a message that the slug contains invalid characters. No file is created.

### AC: lint-clean-after-bare-project

**Requirements:** cli/idea/new#req:ancestor-indexes-materialized, cli/idea/README#req:lint-clean-on-create

In a project that has `specscore.yaml` but no `spec/` tree, running `specscore idea new my-idea` creates `spec/README.md`, `spec/ideas/README.md`, and `spec/ideas/my-idea.md`. A subsequent `specscore spec lint` returns no error-severity violations outside `spec/ideas/my-idea.md` itself.

## Open Questions

- Should `idea new` accept an `--edit` flag that opens the scaffolded file in `$EDITOR` after writing, for a smoother author flow?
- Should the `--owner` default be GitHub username (from git config / `gh auth status`) instead of `$USER`, matching how `gh` attributes authorship?

---
*This document follows the https://specscore.md/feature-specification*
