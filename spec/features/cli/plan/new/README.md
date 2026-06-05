# Feature: Plan New

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan/new?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan/new?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan/new?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan/new?op=request-change) |

**Status:** Approved

## Summary

`specscore plan new <slug>` scaffolds a lint-clean Plan artifact at `spec/plans/<slug>.md`. The scaffold emits the plan's body-metadata header block, the required sections (`## Summary`, `## Approach`, `## Tasks`, `## Open Questions`) with HTML-comment prompts, the adherence footer, AND the frontmatter `format:` / `status:` fields mandated by the [artifact-frontmatter-convention](../../../../../../specscore/spec/features/artifact-frontmatter-convention/README.md) feature — so a freshly scaffolded plan carries its machine-readable surfaces from creation rather than acquiring them on a later `lint --fix`. A plan is created against a Source Feature (`--feature`) or a Source Idea (`--idea`).

## Synopsis

```
specscore plan new <slug> (--feature <feature-slug> | --idea <idea-slug>) [--title <text>] [--owner <id>] [--force] [--project <path>]
```

## Problem

Plans are the only status-bearing spec artifact with no creation verb: the `plan` group is read-only (`list`/`info`), so plans are hand-authored by `specstudio:plan` from a Markdown template the skill carries. That template predates the artifact-frontmatter-convention, so newly created plans lack the `format:`/`status:` frontmatter every other artifact will carry, and the plan body's required-section contract is re-implemented in skill prose rather than enforced at the source. A `plan new` verb that scaffolds a lint-clean plan — frontmatter included — makes the CLI the single source of canonical plan structure, the same way `idea new` and `feature new` already are for their types.

## Behavior

### Slug argument

The `<slug>` positional argument becomes the file name (`spec/plans/<slug>.md`). Plan slugs are flat — plans have no hierarchy.

#### REQ: slug-required

`<slug>` MUST be supplied. Absence MUST exit `2` (InvalidArgs).

#### REQ: slug-format

The slug MUST be lowercase, hyphen-separated, URL-safe, and MUST NOT contain `/`. An invalid slug MUST exit `2` with a message naming the offending slug.

### Source binding

A plan decomposes exactly one source: a Feature or an Idea. Exactly one of `--feature` / `--idea` MUST be supplied.

#### REQ: source-required

`specscore plan new` MUST require exactly one of `--feature <feature-slug>` or `--idea <idea-slug>`. Supplying neither, or both, MUST exit `2` (InvalidArgs) with a message naming the conflict. `--feature` scaffolds a `**Source Feature:**` header line; `--idea` scaffolds a `**Source:** idea:<slug>` header line, per the [plan](../../../../../../specscore/spec/features/plan/README.md) feature's two source modes.

### Scaffold content

The generated file is lint-clean on creation. Authored content is left as HTML-comment prompts the author fills in.

#### REQ: scaffolds-lint-clean

The scaffold MUST emit the flat-file plan model that `specscore spec lint` actually enforces today — the same shape every plan in `spec/plans/*.md` uses and that `specstudio:plan` produces: a single file `spec/plans/<slug>.md` (not a directory), the body-metadata header block (`**Status:** Draft`, the resolved source line, `**Date:**`, `**Owner:**`, `**Supersedes:** —`), the `## Summary`, `## Approach`, `## Tasks`, and `## Open Questions` sections (with `<!-- TODO: ... -->` prompts where content is absent), and the adherence footer line `*This document follows the https://specscore.md/plan-specification*`. `specscore spec lint` immediately afterwards MUST report no new error-severity violations outside the scaffolded file itself. (The canonical `plan` Feature's prose currently describes an older directory-based model with lowercase statuses; lint does not enforce it and no plan follows it — see Open Questions.)

#### REQ: emits-frontmatter

The scaffold MUST emit, per the [artifact-frontmatter-convention](../../../../../../specscore/spec/features/artifact-frontmatter-convention/README.md) feature, a `format:` frontmatter field carrying the plan spec URL (`https://specscore.md/plan-specification`) and a `status:` frontmatter field mirroring the initial body `**Status:** Draft`. The footer URL and `format:` MUST agree on creation.

### Overwrite behavior

#### REQ: no-clobber-default

If `spec/plans/<slug>.md` already exists, the command MUST exit `1` (Conflict) with a message naming the path, unless `--force` is supplied. No partial write may occur before the collision check.

### Ancestor index materialization

#### REQ: ancestor-indexes-materialized

When `plan new` writes `spec/plans/<slug>.md`, the command MUST also materialize `spec/README.md` and `spec/plans/README.md` when they do not already exist, using the same templates as `specscore init`. Existing files MUST be left untouched (idempotent). The plan file MUST NOT be written until both ancestor indexes are in place.

## Parameters

| Name | Required | Description |
|---|---|---|
| `slug` | Yes | Plan slug — becomes the file name. |
| `--feature` | One of | Source Feature slug (mutually exclusive with `--idea`). |
| `--idea` | One of | Source Idea slug (mutually exclusive with `--feature`). |
| `--title` | No | Plan title (defaults to title-cased slug). |
| `--owner` | No | Owner/author (defaults to `$USER`). |
| `--force` | No | Overwrite an existing plan file. |

## Exit codes

| Code | Condition |
|---|---|
| `0` | Plan file created |
| `1` | File already exists and `--force` not supplied |
| `2` | Missing/invalid `slug`, or `--feature`/`--idea` not exactly one |
| `10` | Unexpected I/O failure while writing |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [plan (CLI group)](../README.md) | Parent group. `new` is the first mutating verb in the group; `list`/`info` stay read-only. |
| [plan](../../../../../../specscore/spec/features/plan/README.md) | Conceptual source for the two source modes (Feature-sourced vs Idea-sourced). The scaffold's concrete structure follows the lint-enforced flat-file model used in practice, which currently diverges from this Feature's directory-based prose (see Open Questions), not the spec's literal section/header/status list. |
| [artifact-frontmatter-convention](../../../../../../specscore/spec/features/artifact-frontmatter-convention/README.md) | Owns the `format:`/`status:` scaffold-emission contract this verb implements for plans. |
| [cli/idea/new](../../idea/new/README.md) | Sibling create verb whose slug/no-clobber/ancestor-index/lint-clean contract this verb mirrors. |

## Acceptance Criteria

### AC: scaffolded-plan-is-lint-clean

**Requirements:** cli/plan/new#req:scaffolds-lint-clean

`specscore plan new my-plan --feature some-feature` creates `spec/plans/my-plan.md`. `specscore spec lint` immediately afterwards reports no new error-severity violations outside the scaffolded file, even though several fields contain `<!-- TODO: ... -->` prompts.

### AC: scaffold-emits-frontmatter

**Requirements:** cli/plan/new#req:emits-frontmatter

A plan scaffolded by `plan new` carries a frontmatter `format: https://specscore.md/plan-specification` and a `status: Draft` mirroring its body `**Status:** Draft`, and its adherence-footer URL matches `format:`.

### AC: source-required

**Requirements:** cli/plan/new#req:source-required

`specscore plan new my-plan` with neither `--feature` nor `--idea`, and the same command with both, each exit `2` naming the conflict; supplying exactly one succeeds.

### AC: existing-file-conflict

**Requirements:** cli/plan/new#req:no-clobber-default

Running the command twice for the same slug without `--force` exits `1` on the second run and leaves the existing file untouched. With `--force`, the second run overwrites and exits `0`.

### AC: invalid-slug-rejected

**Requirements:** cli/plan/new#req:slug-format

`specscore plan new My_Plan --feature some-feature` exits `2` with a message that the slug contains invalid characters. No file is created.

### AC: ancestor-indexes-materialized

**Requirements:** cli/plan/new#req:ancestor-indexes-materialized

**Given** a project with `specscore.yaml` but no `spec/plans/` tree
**When** the user runs `specscore plan new my-plan --feature some-feature`
**Then** `spec/README.md`, `spec/plans/README.md`, and `spec/plans/my-plan.md` are created; a subsequent `specscore spec lint` reports no error-severity violations outside the new plan file; and re-running the command leaves the pre-existing indexes untouched.

## Open Questions

- **Canonical Plan-spec drift (cross-repo, blocks nothing here but flagged):** `specscore/spec/features/plan/README.md` still specifies a directory-based plan (`spec/plans/<slug>/README.md`), sections `Context`/`Acceptance criteria`/`Tasks`, header `Features`/`Source type`/`Source`/`Author`/`Created`, and lowercase statuses `draft`/`in_review`/`approved`. None of that matches the flat-file model lint actually enforces or that any plan uses. This verb deliberately scaffolds the real lint-clean model; reconciling the canonical Plan Feature with reality is separate work owned by the `specscore` repo.
- Should `plan new` accept a `--task "<name>"` repeatable flag to pre-seed `### Task N:` blocks, or is an empty `## Tasks` with a prompt sufficient for the scaffold?
- Should `--feature`/`--idea` validate that the named source exists and is in an Approved-family status at scaffold time, or stay purely structural (defer resolution to lint)?

---
*This document follows the https://specscore.md/feature-specification*
