---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: Migrate

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/migrate?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/migrate?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/migrate?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/migrate?op=request-change) |

**Status:** Draft
**Source Ideas:** —

## Summary

`specscore migrate` performs the one-shot, deterministic, per-repo migration that brings every existing artifact into conformance with the artifact-frontmatter-convention (defined in the sibling `specscore` meta-spec): it backfills the leading-frontmatter `format:` field (the canonical spec URL for each artifact's type) and, for status-bearing types, the `status:` field (mirrored from the body `**Status:**`), and keeps each adherence-footer URL aligned with `format:`. The existing nested form, `specscore spec migrate`, remains an equivalent invocation for scripts and users who discover the verb through the `spec` command group. It is the migration step that lets the graced frontmatter lint rules (`format-field`, `status-mirror`, `footer-format-mirror`) flip from `warning` to `error` against an already-conformant tree.

## Problem

The frontmatter convention ships its lint rules graced (warning-only) so existing repos do not break on landing — but the artifacts themselves still lack frontmatter. In this repo alone that is dozens of feature READMEs, the idea files, the `*-index` READMEs, and the directory-form plan READMEs, none of which carry `format:`/`status:`. Hand-editing them is error-prone and non-deterministic, and `specscore spec lint --fix` deliberately does not fabricate frontmatter blocks (`status-mirror` only upserts into an existing block; `format-field` has no fixer). A single deterministic command is needed to backfill the whole tree at once, so the graced rules can then be flipped to `error` with the repo already conformant.

## Behavior

### Command

#### REQ: command-shape

`specscore migrate` MUST, in a single invocation, walk the spec tree and write the frontmatter-convention fields into every artifact the frontmatter lint rules enforce. It takes no positional arguments; `--project <path>` selects the project root (default: autodetected from the current directory).

The nested form `specscore spec migrate` MUST remain available and behaviorally equivalent to `specscore migrate`, including the same flags, output, exit codes, and on-disk rewrites. The root `specscore --help` output MUST list `migrate` so the one-shot maintenance command is discoverable without expanding `spec [command]`.

### Format backfill

#### REQ: format-backfill

For every artifact of a Document or Index Kind the convention rules walk, `migrate` MUST ensure a leading YAML frontmatter block carrying `format: <url>`, where `<url>` is the canonical spec URL for that artifact's type. An artifact already carrying the correct `format:` is left byte-unchanged.

### Status backfill

#### REQ: status-backfill

For a status-bearing artifact (Idea, Feature, Plan, Task, Decision), `migrate` MUST write a frontmatter `status:` mirroring the body `**Status:**` token. A status-less type (the `*-index` READMEs, scenarios, properties, entities) MUST NOT receive a `status:` field.

### Footer alignment

#### REQ: footer-alignment

`migrate` MUST leave each artifact's adherence-footer URL equal to its frontmatter `format:` — the frontmatter is canonical for the pair, so the footer is aligned to it, never the reverse.

### Determinism and idempotency

#### REQ: deterministic-offline

`migrate` MUST be deterministic and offline: the same tree yields byte-identical output with no network access.

#### REQ: idempotent

Re-running `migrate` on an already-migrated tree MUST be a no-op — exit `0` with no file changes.

### Rule cutover

#### REQ: rule-cutover

Once the target repo is migrated, the graced frontmatter rules (`format-field`, `status-mirror`, `footer-format-mirror`) MUST be flipped from `warning` to `error` severity, so `specscore spec lint` thereafter enforces the convention by default. A migrated tree MUST pass `spec lint` at the post-cutover `error` severity.

## Acceptance Criteria

### AC: backfills-format-and-status

**Requirements:** cli/spec/migrate#req:command-shape, cli/spec/migrate#req:format-backfill, cli/spec/migrate#req:status-backfill

**Given** a feature README and an idea file with body `**Status:**` lines but no frontmatter
**When** `specscore migrate` runs
**Then** each gains a leading frontmatter block with the type's canonical `format:` URL and a `status:` mirroring its body `**Status:**`.

### AC: status-less-types-excluded

**Requirements:** cli/spec/migrate#req:status-backfill

**Given** an `*-index` README (a status-less type) with no frontmatter
**When** `specscore migrate` runs
**Then** it gains `format:` but no `status:` field.

### AC: footer-aligned-to-format

**Requirements:** cli/spec/migrate#req:footer-alignment

**Given** a migrated artifact
**When** `specscore spec lint` runs
**Then** its adherence-footer URL equals its frontmatter `format:` (no `footer-format-mirror` violation).

### AC: migration-idempotent

**Requirements:** cli/spec/migrate#req:idempotent, cli/spec/migrate#req:deterministic-offline

**Given** an already-migrated tree
**When** `specscore migrate` runs again
**Then** it exits `0` and changes no files.

### AC: root-alias-discoverable

**Requirements:** cli/spec/migrate#req:command-shape

**Given** a SpecScore CLI binary
**When** `specscore --help` runs
**Then** the command list includes `migrate`; and invoking `specscore migrate --project <root>` performs the same migration as `specscore spec migrate --project <root>`.

### AC: rules-enforce-after-cutover

**Requirements:** cli/spec/migrate#req:rule-cutover

**Given** a migrated repo with the frontmatter rules flipped to `error` severity
**When** `specscore spec lint` runs
**Then** it reports no frontmatter violations; and an artifact whose frontmatter is removed is then flagged at `error` severity.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
