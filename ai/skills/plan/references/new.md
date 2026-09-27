# specscore plan new

Scaffold a lint-clean Plan artifact at `spec/plans/<slug>.md`. The scaffold emits the plan's body-metadata header, the required sections (`## Summary`, `## Approach`, `## Tasks`, `## Open Questions`) with HTML-comment prompts, the adherence footer, and the mandated `format:` / `status:` frontmatter — so a freshly scaffolded plan carries its machine-readable surfaces from creation. A plan binds to **at most one** source: a Source Feature (`--feature`), a Source Idea (`--idea`), or no source at all (pass neither — a source-less plan, recorded as `**Source:** none`).

**CLI reference:** [`cli/plan/new`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/plan/new/README.md)

## When to use

- **Feature → Plan** step in a workflow when an approved Feature is ready to decompose
- **Idea → Plan** when planning directly from an Idea (no intervening Feature)
- **Source-less plan** when planning work that is tied to neither a Feature nor an Idea (pass neither flag)
- **Seeding a plan skeleton** the CLI guarantees is lint-clean, instead of hand-authoring from a template

This command is the **single source of canonical plan structure** — prefer it over hand-writing a plan file. It is **not** for transitioning plan status.

## Command

```bash
specscore plan new <slug> \
  [--feature <feature-slug> | --idea <idea-slug>] \
  [--parent <plan-ref>] \
  [--title <text>] \
  [--owner <id>] \
  [--force] \
  [--project <path>]
```

## Parameters

| Flag | Required | Description |
|---|---|---|
| `<slug>` | Yes | Plan slug — lowercase, hyphen-separated, URL-safe, no `/`. Becomes the file name (`spec/plans/<slug>.md`). |
| `--feature` | Optional | Source Feature slug. Mutually exclusive with `--idea`. Omit both flags for a source-less plan. |
| `--idea` | Optional | Source Idea slug. Mutually exclusive with `--feature`. Omit both flags for a source-less plan. |
| `--parent` | No | Parent (master) plan reference for cross-repo plan composition — a same-repo slug or a `<repo-slug>:<plan-slug>` cross-repo soft ref. Emits a `**Parent:**` line, recorded verbatim; validated by lint rule `P-005`, not by this verb. An empty value exits `2`. |
| `--title` | No | Plan title. Defaults to the title-cased slug. |
| `--owner` | No | Owner / author. Defaults to `$USER`. |
| `--force` | No | Overwrite an existing plan file at that slug. |
| `--project` | No | Project root path. Autodetected from `cwd` if omitted. |

## Exit codes

| Code | Meaning | What to do |
|---|---|---|
| `0` | Plan created (lint-clean) | Parse the output for the new path, then fill the section prompts. |
| `1` | Slug collision — `spec/plans/<slug>.md` already exists | Pick a different slug, or pass `--force` to overwrite. |
| `2` | Missing `<slug>`, both source flags together, bad slug format, or invalid flag value | Check arguments — supply at most one of `--feature` / `--idea` (or neither for a source-less plan). |
| `3` | `--feature` / `--idea` names a non-existent source artifact | Verify the source slug; no file is written on validation failure. |
| `10+` | Unexpected I/O failure | Inspect the working tree before retrying. |

## Examples

### From a Feature

```bash
specscore plan new cli-idea-promote --feature cli/idea/promote
```

### From an Idea

```bash
specscore plan new cross-repo-plan-composition --idea cross-repo-plan-composition
```

### With an explicit title and owner

```bash
specscore plan new ship-onboarding \
  --feature onboarding \
  --title "Ship onboarding flow" \
  --owner alex
```

### Source-less (no Feature, no Idea)

```bash
specscore plan new cloud-run-migration
# scaffolds **Source:** none
```

### As a cross-repo sub-plan (records a Parent)

```bash
specscore plan new feature-1-plan-composition-cli \
  --feature cli/plan/new \
  --parent specscore:cross-repo-plan-composition
# scaffolds **Parent:** specscore:cross-repo-plan-composition (verbatim; P-005 validates it)
```

## Notes

- **Source is optional and exclusive:** supply at most one of `--feature` or `--idea`; passing neither produces a source-less plan (`**Source:** none`). Plans bound to an Idea or that are source-less carry no `features` list; plans bound to a Feature do.
- **Lint-clean by construction:** the scaffold passes `specscore spec lint` on creation. Your subsequent edits (filling the section prompts) must keep it that way.
- **No `--force` by default:** collisions exit `1` rather than overwriting. Pass `--force` only to replace an existing draft.
- **Plan slugs are flat** in the current CLI surface (no `/`). After scaffolding, fill the `## Summary`, `## Approach`, and `## Tasks` prompts.
- To read plans back use [`plan list`](list.md) and [`plan info`](info.md).
