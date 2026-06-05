---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: Idea (CLI)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/idea?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/idea?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/idea?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/idea?op=request-change) |
>
> **AI skill:** [GitHub](https://github.com/specscore/ai-plugin-specscore/blob/main/skills/idea/SKILL.md) · [local](../../../../../ai-plugin-specscore/skills/idea/SKILL.md) — if this command's CLI signature or behavior changes, update the linked skill to keep agents in sync.

**Status:** Stable

## Summary

`specscore idea` commands manage SpecScore Idea artifacts — pre-spec, lintable one-pagers stored at `spec/ideas/<slug>.md`. The group covers scaffolding (`idea new`) and lifecycle transitions (`idea change-status`), each producing or maintaining lint-clean output.

## Problem

Ideas have a strict required-sections contract defined by the [idea](../../idea/README.md) feature. Hand-authoring them means remembering HMW framing, `Not Doing` entries, owner attribution, and status rules. A scaffolder produces a skeleton that is lint-clean on creation, so authors edit content rather than fight structure.

## Contents

| Directory | Description |
|---|---|
| [change-status/](change-status/README.md) | Transition an Idea's status per the legal-transition matrix; `--to=archived` also relocates the file under `spec/ideas/archived/` |
| [new/](new/README.md) | Scaffold a new Idea artifact at `spec/ideas/<slug>.md` (or a change-request proposal at `spec/features/<target>/proposals/<slug>.md`) |
| [relocate/](relocate/README.md) | Move an Idea or sidekick-seed artifact across SpecScore-managed repos, with cross-repo link cleanup and per-repo auto-commit |

### change-status

Transitions an Idea per the kind's legal-transition matrix: `Draft → Approved`, and any active status → `Archived`. The `--to=archived` path additionally moves the file from `spec/ideas/<slug>.md` to `spec/ideas/archived/<slug>.md` (rollback covers both rewrite and relocation; collision exits `1`). Implements the [lifecycle-transitions](../lifecycle-transitions/README.md) shared contract. Illegal `(from, to)` pairs — including re-running on the target status — exit `4` (InvalidTransition).

### list

Lists all Idea slugs (one per line, sorted alphabetically). By default, archived ideas are excluded. `--include-archived` (or `--all`) includes them. `--status <value>` filters by lifecycle status. `--format yaml|json` emits structured output.

### new

Creates a lint-clean Idea skeleton with every required section, HTML-comment prompts describing what belongs in each, and either flag-supplied or interactively prompted content for the core fields (title, HMW, owner, not-doing entries). Supports `--type=change-request --targets=<feature-slug>` to scaffold a Proposal at `spec/features/<target>/proposals/<slug>.md`, and `--phase` to pre-populate a Phase field.

### relocate

Moves an Idea or sidekick-seed artifact from the current project to a different SpecScore-managed repo. Auto-resolves slug to `spec/ideas/<slug>.md` first, then `spec/ideas/seeds/<slug>.md`. Pre-flight clean-tree check across source, target, and every sibling SpecScore repo whose docs reference the artifact. Copies the file (with `specscore/*` → `specscore/*` and "this repo" rewrites), updates markdown-link references to the new location across all affected repos, auto-commits per repo by default (`--no-commit` flag stages without committing). Stop-on-first-commit-failure semantics; cross-repo rollback is the user's responsibility.
| [promote](promote/README.md) | Promote a sidekick seed into a lint-clean Idea: same-repo git mv + transform + back-link reconcile; cross-repo archive + promoted_to pointer. |

## Behavior

### Scope of this group

Commands under `specscore idea` operate on a single Idea artifact at `spec/ideas/<slug>.md`. They MUST NOT create features — `feature new` is the entry point for features, and this split keeps the two flows discoverable.

#### REQ: ideas-only

No subcommand of `specscore idea` may mutate `spec/features/` except when scaffolding a change-request idea (`--type=change-request`), which writes to `spec/features/<target>/proposals/<slug>.md`. The canonical write target for feature-request ideas is `spec/ideas/`.

### Lint-clean output

Every artifact scaffolded under this group MUST pass `specscore spec lint` on first creation.

#### REQ: lint-clean-on-create

A file produced by any `specscore idea` mutation command MUST satisfy every lint rule applicable to the Idea document kind. Commands that cannot produce a lint-clean artifact for the given inputs (e.g., a bad slug) MUST exit with a failure code BEFORE writing the file.

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [idea](../../idea/README.md) | Defines the required shape of an Idea. `idea new` produces instances. |
| [feature/new](../feature/new/README.md) | Sibling scaffolder for features. Lives in the `feature` group because features are queried there too. |
| [CLI](../README.md) | Inherits shared exit-code contract, `--project`, `--format`. |
| [`idea/` skill](https://github.com/specscore/ai-plugin-specscore/blob/main/skills/idea/SKILL.md) (ai-plugin-specscore) | Agent-side wrapper for `idea new`. Treats this feature spec as the authoritative contract. |

## Open Questions

- ~~Should `specscore idea` grow read commands (`idea list`, `idea info`)?~~ Resolved: `idea list` is now implemented. `idea info` remains open.
- Should `specscore idea info <slug>` be added for parity with `feature info`?

---
*This document follows the https://specscore.md/feature-specification*
