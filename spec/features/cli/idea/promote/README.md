---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Idea Promote

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/idea/promote?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/idea/promote?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/idea/promote?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/idea/promote?op=request-change) |

**Status:** Approved
**Source Ideas:** —

## Summary

`specscore idea promote <slug>` turns a sidekick seed (`spec/ideas/seeds/<slug>.md`) into a lint-clean Idea (`spec/ideas/<slug>.md`). It is a deterministic, non-interactive mutation in the family of `idea new` and `idea relocate`: it relocates and transforms, it does not author prose. The output is a lint-clean Idea *skeleton* carrying the seed's content, ready for an author (or `specstudio:ideate`) to fill.

## Synopsis

```
specscore idea promote <slug> [--force] [--verdict=pointer|full|drop] [--project <path>]
```

## Problem

A seed and an Idea are different on-disk schemas: a seed is YAML-frontmatter plus free prose, an Idea is bold-prefix metadata under a `# Idea:` title with required sections. There is no defined path between them, so promotion is done by hand — and a naive `git mv` produces a file that fails idea lint, silently breaks the `## Sidekick Seeds Generated` back-links that point at the seed, and orphans the consilium verdict carried on the seed. This verb makes promotion a single, deterministic, lint-clean operation with provenance preserved and same-repo links reconciled.

## Behavior

The verb resolves the seed, classifies the back-links that reference it, then takes one of two disjoint paths — same-repo (move + reconcile) or cross-repo (archive + mark) — and finally carries the consilium verdict forward per configuration. It never authors Idea prose beyond folding the seed body into `## Context`.

### Slug resolution

#### REQ: promote-resolves-seed

The verb MUST resolve `<slug>` to `spec/ideas/seeds/<slug>.md` in the source project. When no such file exists, the verb MUST exit `3` (NotFound) with a message naming the missing path and MUST NOT create, move, or modify any file.

### Destination collision

#### REQ: collision-refusal

When `spec/ideas/<slug>.md` already exists, the verb MUST exit `1` (Collision) unless `--force` is supplied. On refusal, no file is created, moved, or modified.

### Pre-flight clean-tree check

#### REQ: preflight-clean-tree

Before any mutation, the verb MUST verify the source project's working tree is clean in the paths it will modify (the seed, the destination Idea path, and any same-repo source artifact carrying a reconcilable back-link). If any such path has uncommitted changes, the verb MUST exit `7` (DirtyTree) without mutating anything, so that a failed run never interleaves with pre-existing edits.

### Back-link classification

#### REQ: backlink-discovery-and-classification

The verb MUST discover the `## Sidekick Seeds Generated` back-link entries that reference the seed — both in the source repo and in sibling SpecScore repos (the same discovery `idea relocate` performs) — and classify each entry as **same-repo** (a bare relative-path target) or **cross-repo** (a `<repo-slug>:` repo-qualified target, per the `sidekick-capture` back-link format). The presence of any cross-repo reference selects the cross-repo path; otherwise the same-repo path applies.

### Same-repo promotion

#### REQ: same-repo-move-and-transform

When every back-link is same-repo (or the seed has none), the verb MUST `git mv spec/ideas/seeds/<slug>.md spec/ideas/<slug>.md` (preserving history via rename detection), then transform the moved file in place: replace the seed frontmatter with Idea body-metadata (`**Status:** Draft`, `**Date:**`, `**Owner:**`, `**Promotes To:** —`, `**Supersedes:** —`, `**Related Ideas:** —`), retitle the body to `# Idea: <title>`, fold the seed's prose into `## Context`, and insert HTML-comment prompts for every unfilled canonical Idea section. The verb MUST run lint-fix so the result is lint-clean by construction.

#### REQ: same-repo-backlink-reconcile

In the same-repo path, the verb MUST rewrite every same-repo `## Sidekick Seeds Generated` entry that pointed at `spec/ideas/seeds/<slug>.md` to point at `spec/ideas/<slug>.md`. No same-repo back-link may dangle after a successful promotion.

### Cross-repo promotion

#### REQ: cross-repo-archive-and-mark

When any back-link is cross-repo, the verb MUST NOT rewrite the sibling repo's back-link (a single-repo operation cannot reach another repo). It MUST: create the new Idea at `spec/ideas/<slug>.md` by copying and transforming the seed body (the same transform as `same-repo-move-and-transform`); `git mv` the seed to `spec/ideas/archived/<slug>.md` with its frontmatter `status` set to `promoted` and a frontmatter key `promoted_to: <slug>` identifying the created Idea; and run lint-fix so the new Idea is lint-clean. The archived seed and the new Idea both exist after this path. Reconciling the cross-repo back-link to the moved seed is delegated to lint/UI cross-repo reference resolution (see `## Open Questions`), not to this verb.

#### REQ: promoted-not-deprecated

The verb sets only `status: promoted` on a retained (archived) seed. It MUST NOT set `deprecated` — that terminal state belongs to the consilium's reject path, not to promotion.

### Verdict carry-forward

#### REQ: verdict-carry-forward

The created Idea MUST carry the seed's `## Consilium Verdict` forward according to a setting with both a project default and a per-invocation override: a `specscore.yaml` `promote.verdict_carry_forward` key (`pointer` | `full` | `drop`, default `pointer`) sets the project default, and a `--verdict=<pointer|full|drop>` flag overrides it for a single run (the flag wins). `pointer` writes a single-line provenance pointer to the verdict; `full` copies the entire `## Consilium Verdict` section into the Idea; `drop` omits it. When the seed carries no verdict, the verb omits the pointer regardless of the setting.

### Output

#### REQ: stdout-format

On success the verb MUST emit a deterministic summary naming: the created Idea path, the seed's fate (`moved` to the Idea path in the same-repo path, or `archived` to `spec/ideas/archived/<slug>.md` in the cross-repo path), and each reconciled same-repo back-link. `--format yaml|json` emits the same facts as structured output (mirroring `idea relocate`'s `stdout-format`).

## Parameters

| Name | Required | Description |
|---|---|---|
| `slug` | Yes | Seed slug. Resolved to `spec/ideas/seeds/<slug>.md`. Missing exits `3`. |

## Flags

| Flag | Required | Description |
|---|---|---|
| `--force` | No | Overwrite an existing `spec/ideas/<slug>.md` (otherwise the collision exits `1`). |
| `--verdict` | No | Verdict carry-forward mode: `pointer` (default), `full`, or `drop`. Overrides the `promote.verdict_carry_forward` project default. An unrecognized value exits `2`. |
| `--project` | No | Source project root. Autodetected per [CLI#req:project-autodetect](../../README.md#req-project-autodetect). |

## Exit codes

| Code | Condition |
|---|---|
| `0` | Promotion succeeded. |
| `1` | Collision: `spec/ideas/<slug>.md` already exists and `--force` was not supplied. |
| `2` | InvalidArgs: missing `<slug>`, unrecognized flag, or invalid `--verdict` value. |
| `3` | NotFound: no seed at `spec/ideas/seeds/<slug>.md`. |
| `7` | DirtyTree: the source project has uncommitted changes in a path the verb would modify. |
| `10` | I/O or git failure (rollback applied). |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [idea (CLI group)](../README.md) | Parent group; the Contents table includes this sub-feature. |
| [cli/idea/new](../new/README.md) | Shares the Idea-skeleton transform: `promote` produces the same lint-clean skeleton shape `new` scaffolds, but seeded from an existing seed's body rather than from flags. |
| [cli/idea/relocate](../relocate/README.md) | Shares back-link discovery and the sibling-repo reference scan, and the clean-tree pre-flight discipline. `relocate` moves an artifact across repos; `promote` transforms a seed into an Idea within one repo. |
| [cli/idea/change-status](../change-status/README.md) | Sibling verb; `change-status --to=archived` also relocates a file into `spec/ideas/archived/`. `promote`'s cross-repo path reuses that archive location for the retained seed. |
| [cli/spec/lint](../../spec/lint/README.md) | The verb runs lint-fix on its output so the created Idea is lint-clean. Cross-repo back-link reconciliation to a moved/archived seed is left to lint/UI resolution, not performed here. |
| Source Idea: [seed-to-idea-promotion](https://github.com/specscore/specstudio-skills/blob/main/spec/ideas/seed-to-idea-promotion.md) and Feature [seed-to-idea-promotion](https://github.com/specscore/specstudio-skills/blob/main/spec/features/seed-to-idea-promotion/README.md) | **Cross-repo Source-Idea/Feature reference.** The originating Idea and the umbrella Feature (which also covers the skill-side consilium-offer flow, out of scope here) live in `specscore/specstudio-skills`. Full GitHub URLs are used because relative-path resolution does not span repos. |
| [sidekick-capture (`cross-repo-back-link-format`)](https://github.com/specscore/specstudio-skills/blob/main/spec/features/sidekick-capture/README.md) | Defines the `<repo-slug>:` repo-qualified back-link target this verb classifies on to choose the same-repo vs cross-repo path. |
| `specstudio:ideate` (skill, post-MVP) | Invokes this verb to promote a seed, then fills the returned skeleton; also owns the interactive consilium offer for unreviewed seeds. That behavior is specified in the specstudio-skills `seed-to-idea-promotion` Feature, not here. |

## Acceptance Criteria

### AC: seed-not-found
**Requirements:** [#req:promote-resolves-seed](#req-promote-resolves-seed)

**Given** no file at `spec/ideas/seeds/ghost.md`
**When** `specscore idea promote ghost` runs
**Then** the command exits `3`, names the missing `spec/ideas/seeds/ghost.md` path, and no file is created, moved, or modified.

### AC: collision-without-force
**Requirements:** [#req:collision-refusal](#req-collision-refusal)

**Given** a seed at `spec/ideas/seeds/foo.md` and an existing `spec/ideas/foo.md`
**When** `specscore idea promote foo` runs without `--force`
**Then** the command exits `1` and leaves both files unchanged.

### AC: dirty-tree-rejected
**Requirements:** [#req:preflight-clean-tree](#req-preflight-clean-tree)

**Given** a seed at `spec/ideas/seeds/foo.md` and an uncommitted edit to that seed
**When** `specscore idea promote foo` runs
**Then** the command exits `7` and mutates nothing.

### AC: same-repo-promote-happy-path
**Requirements:** [#req:same-repo-move-and-transform](#req-same-repo-move-and-transform), [#req:backlink-discovery-and-classification](#req-backlink-discovery-and-classification)

**Given** a seed at `spec/ideas/seeds/bar.md` whose only back-links are same-repo
**When** `specscore idea promote bar` runs
**Then** `spec/ideas/seeds/bar.md` no longer exists, `spec/ideas/bar.md` exists with a `# Idea:` title and Idea body-metadata, `git log --follow spec/ideas/bar.md` reaches the original seed commit, and `specscore spec lint` passes.

### AC: same-repo-backlinks-reconciled
**Requirements:** [#req:same-repo-backlink-reconcile](#req-same-repo-backlink-reconcile)

**Given** a same-repo Feature whose `## Sidekick Seeds Generated` section links to `spec/ideas/seeds/bar.md`
**When** `specscore idea promote bar` completes via the same-repo path
**Then** that back-link points at `spec/ideas/bar.md` and no entry still references `spec/ideas/seeds/bar.md`.

### AC: cross-repo-archive
**Requirements:** [#req:cross-repo-archive-and-mark](#req-cross-repo-archive-and-mark), [#req:backlink-discovery-and-classification](#req-backlink-discovery-and-classification)

**Given** a seed at `spec/ideas/seeds/baz.md` with at least one cross-repo (`<repo-slug>:`) back-link
**When** `specscore idea promote baz` runs
**Then** `spec/ideas/baz.md` exists as a lint-clean Idea skeleton, the seed has moved to `spec/ideas/archived/baz.md` with frontmatter `status: promoted` and `promoted_to: baz`, no file remains at `spec/ideas/seeds/baz.md`, and the sibling repo's back-link was not rewritten.

### AC: never-deprecated
**Requirements:** [#req:promoted-not-deprecated](#req-promoted-not-deprecated)

**Given** any seed being promoted
**When** the verb sets a terminal state on a retained seed
**Then** the state is `promoted` and never `deprecated`.

### AC: verdict-carry-forward-modes
**Requirements:** [#req:verdict-carry-forward](#req-verdict-carry-forward)

**Given** a seed carrying a `## Consilium Verdict` section
**When** it is promoted under default configuration, then re-run with `--verdict=full`, then with `--verdict=drop`
**Then** the default produces a single-line pointer, `full` copies the whole section, `drop` omits it, and a `--verdict` flag overrides the `promote.verdict_carry_forward` project default when both are set.

### AC: stdout-summary
**Requirements:** [#req:stdout-format](#req-stdout-format)

**Given** a successful same-repo promotion
**When** the verb completes
**Then** stdout names the created Idea path, reports the seed as `moved`, lists each reconciled back-link, and `--format json` emits the same facts as structured output.

## Open Questions

- Cross-repo back-link reconciliation: after the seed moves to `spec/ideas/archived/<slug>.md`, the sibling repo's `<repo-slug>:`-qualified back-link still points at the old `seeds/` path. This verb cannot reach that repo; reconciliation is delegated to lint/UI cross-repo reference resolution. Confirm that mechanism resolves a moved/archived seed reference, and decide whether it points at the archived seed or the promoted Idea.
- Exact structured `stdout` schema (field names for `--format json|yaml`) — align with `idea relocate`'s `stdout-format` shape during implementation.

---
*This document follows the https://specscore.md/feature-specification*
