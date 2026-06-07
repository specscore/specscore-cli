---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Sidekick New

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/sidekick/new?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/sidekick/new?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/sidekick/new?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/sidekick/new?op=request-change) |
**Status:** Approved

## Summary

`specscore sidekick new "<one-liner>"` scaffolds a lint-clean sidekick-seed at `spec/ideas/seeds/<slug>.md`. The slug is **derived from the one-liner** (the seed is a scaled-down Idea, so capture is quick and slug-free at the call site). The scaffold emits the minimal `sidekick-seed` frontmatter (`captured_by`, `status`) and an H1 body whose heading is the one-liner verbatim — so a freshly scaffolded seed passes `specscore spec lint` (the `sidekick-seed` rule) by construction. Captured seeds are identified by location, so `type: sidekick-seed` is added only when a seed is archived, not at capture time.

## Synopsis

```
specscore sidekick new "<one-liner>" [--slug <slug>] [--captured-by <id>] [--body <markdown>] [--force] [--project <path>]
```

## Problem

The `specstudio:sidekick` skill hand-writes seeds from an embedded template that duplicates the `sidekick-seed` lint rule's schema. That duplication drifts as the rule evolves and prevents the skill from adopting the Required-CLI Artifact Creation policy its sibling producers already follow. A `sidekick new` verb that scaffolds a lint-clean seed — frontmatter and H1 included — makes the CLI the single source of canonical seed structure, the same way `idea new` and `plan new` are for their types.

## Behavior

### One-liner argument

The single positional argument is the seed's one-liner: the `# <one-liner>` H1 body and the basis for the derived slug.

#### REQ: one-liner-required

Exactly one positional one-liner MUST be supplied. A missing one-liner, or one that is empty/whitespace-only after trimming, MUST exit `2` (InvalidArgs) with a message stating a non-empty one-liner is required.

#### REQ: one-liner-length

The trimmed one-liner MUST NOT exceed 500 characters. A longer one-liner MUST exit `2` (InvalidArgs) naming the limit.

### Slug derivation

By default the slug is derived from the one-liner — matching the `specstudio:sidekick` skill's algorithm so the CLI and the skill produce identical slugs for identical input. A caller that owns its own slug and collision policy (e.g. the skill's `-N` never-overwrite disambiguation) MAY override the derived slug with `--slug`.

#### REQ: slug-derivation

When `--slug` is **not** supplied, the slug MUST be derived from the trimmed one-liner by: lowercasing; replacing every run of characters outside `[a-z0-9]` with a single `-`; trimming leading and trailing `-`; and, when the result exceeds 60 characters, truncating to the nearest preceding `-` boundary that yields a slug ≤ 60 characters. If derivation yields an empty slug (a one-liner with no `[a-z0-9]` characters), the command MUST exit `2` (InvalidArgs).

#### REQ: slug-override

When `--slug <slug>` is supplied, the command MUST use it verbatim as the file name — derivation is skipped. The override MUST be a lowercase, hyphen-separated, URL-safe identifier (no `/`) of at most 60 characters; an invalid override MUST exit `2` (InvalidArgs) naming the offending value. The one-liner is still required (it remains the H1 body), so `--slug` decouples the file identity from the H1 text.

### Scaffold content

The generated file is lint-clean on creation against the `sidekick-seed` rule. Authored content beyond the one-liner is optional.

#### REQ: scaffolds-lint-clean

The scaffold MUST emit a single file `spec/ideas/seeds/<slug>.md` carrying the minimal `sidekick-seed` frontmatter keys (`captured_by:`, `status: queued`) and a body whose first non-blank line is the H1 `# <one-liner>`. `type: sidekick-seed` MUST NOT be written at capture time — it is added only when a seed is archived. `specscore spec lint` immediately afterwards MUST report no new error-severity violations outside the scaffolded file itself.

#### REQ: frontmatter-values

`captured_by` defaults to `user` and is overridable via `--captured-by`. `status` is always `queued`.

#### REQ: optional-body

When `--body <markdown>` is supplied, the scaffold MUST append a blank line and the markdown after the H1 line. The total body region (the H1 line through the end of the optional content) MUST NOT exceed 2000 characters; exceeding it exits `2` (InvalidArgs) naming the limit.

### Overwrite behavior

#### REQ: no-clobber-default

If `spec/ideas/seeds/<slug>.md` already exists, the command MUST exit `1` (Conflict) with a message naming the path, unless `--force` is supplied. No partial write may occur before the collision check.

### Ancestor index materialization

#### REQ: ancestor-indexes-materialized

When `sidekick new` writes the seed, the command MUST also materialize `spec/README.md` and `spec/ideas/README.md` when they do not already exist, using the same templates as `specscore init`, and MUST create the `spec/ideas/seeds/` directory. Existing files MUST be left untouched (idempotent). The seed file MUST NOT be written until the ancestor indexes are in place.

## Parameters

| Name | Required | Description |
|---|---|---|
| `one-liner` | Yes | Seed one-liner — becomes the H1 body and the basis for the derived slug. |
| `--slug` | No | Override the derived slug verbatim (lowercase, hyphen-separated, URL-safe, ≤60 chars). |
| `--captured-by` | No | Capturer identity (defaults to `user`). |
| `--body` | No | Additional markdown appended after the H1. |
| `--force` | No | Overwrite an existing seed file. |

## Exit codes

| Code | Condition |
|---|---|
| `0` | Seed file created |
| `1` | File already exists and `--force` not supplied |
| `2` | Empty/too-long one-liner, empty derived slug, invalid `--slug`, or over-cap body |
| `10` | Unexpected I/O failure while writing |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [sidekick (CLI group)](../README.md) | Parent group. `new` is the group's only mutating verb. |
| [cli/idea/new](../../idea/new/README.md) | Sibling create verb whose slug/no-clobber/ancestor-index/lint-clean contract this verb mirrors. |
| `sidekick-seed` lint rule | Owns the seed schema this scaffold emits; the verb is lint-clean against it by construction. |

## Acceptance Criteria

### AC: scaffolded-seed-is-lint-clean

**Requirements:** cli/sidekick/new#req:scaffolds-lint-clean, cli/sidekick/new#req:frontmatter-values

`specscore sidekick new "specscore needs a seed verb"` creates `spec/ideas/seeds/specscore-needs-a-seed-verb.md` with the minimal `sidekick-seed` frontmatter (`captured_by`, `status: queued`) and an H1 `# specscore needs a seed verb`. `specscore spec lint` immediately afterwards reports no new error-severity violations outside the scaffolded file.

### AC: slug-derived-from-one-liner

**Requirements:** cli/sidekick/new#req:slug-derivation

`specscore sidekick new "Add Batch Mode!"` derives the slug `add-batch-mode` and writes `spec/ideas/seeds/add-batch-mode.md`. A one-liner longer than 60 derived characters is truncated at a `-` boundary to ≤ 60 characters.

### AC: slug-override-used-verbatim

**Requirements:** cli/sidekick/new#req:slug-override

`specscore sidekick new "Add batch mode" --slug add-batch-mode-2` writes `spec/ideas/seeds/add-batch-mode-2.md` (the file name uses the override; the H1 stays `# Add batch mode`). `specscore sidekick new "x" --slug Bad_Slug` exits `2` naming the invalid value and creates no file.

### AC: empty-one-liner-rejected

**Requirements:** cli/sidekick/new#req:one-liner-required, cli/sidekick/new#req:slug-derivation

`specscore sidekick new "   "` exits `2` with a message that a non-empty one-liner is required, and `specscore sidekick new "!!!"` (no `[a-z0-9]`) exits `2` for an empty derived slug. No file is created in either case.

### AC: existing-file-conflict

**Requirements:** cli/sidekick/new#req:no-clobber-default

Running the command twice for the same derived slug without `--force` exits `1` on the second run and leaves the existing file untouched. With `--force`, the second run overwrites and exits `0`.

### AC: ancestor-indexes-materialized

**Requirements:** cli/sidekick/new#req:ancestor-indexes-materialized

**Given** a project with `specscore.yaml` but no `spec/ideas/` tree
**When** the user runs `specscore sidekick new "first seed"`
**Then** `spec/README.md`, `spec/ideas/README.md`, and `spec/ideas/seeds/first-seed.md` are created; a subsequent `specscore spec lint` reports no error-severity violations outside the new seed file; and re-running the command leaves the pre-existing indexes untouched.

## Open Questions

- Should a derived-slug collision auto-disambiguate with a `-2`/`-3` suffix (as the `specstudio:sidekick` skill does) instead of exiting `1` (Conflict)? This verb follows the CLI house style (`Conflict` + `--force`, like `idea new`/`plan new`) for predictability; the skill can pass `--force` or vary the one-liner. Auto-suffix is deferred.

---
*This document follows the https://specscore.md/feature-specification*
