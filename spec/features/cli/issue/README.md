---
format: https://specscore.md/feature-specification
status: Implementing
---

# Feature: Issue (CLI)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/issue?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/issue?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/issue?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/issue?op=request-change) |

**Status:** Implementing
**Source Ideas:** —

## Summary

`specscore issue` commands manage SpecScore Issue artifacts — reported observations of broken behavior stored at `spec/issues/<slug>.md` (root) or `spec/features/<feature-slug>/issues/<slug>.md` (Feature-scoped). The group covers scaffolding (`issue new`), lifecycle transitions (`issue change-status`), and discovery (`issue list`), each producing or maintaining lint-clean output.

## Problem

Issues have a strict required-sections contract defined by the [issue-artifact-type](https://github.com/specscore/specstudio-skills/blob/main/spec/features/issue-artifact-type/README.md) Feature (implemented via `I-001`–`I-015` lint rules). Hand-authoring them means remembering the `# Issue:` H1, three required H2 sections, RFC 3339 timestamps, and status-dependent field rules. A CLI group provides structured scaffolding, validated transitions, and aggregated listing so authors manage content rather than fight structure.

## Contents

| Directory | Description |
|---|---|
| [new/](new/README.md) | Scaffold a new Issue artifact at `spec/issues/<slug>.md` or `spec/features/<feature>/issues/<slug>.md` |
| [change-status/](change-status/README.md) | Transition an Issue's status per the legal-transition matrix |
| [list/](list/README.md) | List all Issue artifacts with filtering and structured output |

### new

Creates a lint-clean Issue skeleton with every required section, HTML-comment prompts describing what belongs in each, and flag-supplied metadata for severity, affected-component, and captured-by. Supports `--feature <slug>` to scaffold Feature-scoped issues.

### change-status

Transitions an Issue per its legal-transition matrix: `open → investigating|resolved|rejected`, `investigating → resolved|rejected`. Enforces severity-required-on-transition and rejection-reason-required-on-rejected as gate checks before mutation.

### list

Lists all Issue artifacts from both `spec/issues/` and `spec/features/*/issues/`, with filters for status, severity, and owning Feature. Supports text, JSON, and YAML output formats.

## Behavior

### Scope of this group

Commands under `specscore issue` operate on Issue artifacts at the two documented locations. They MUST NOT create or mutate Feature artifacts — `feature new` and `feature change-status` are the entry points for features.

#### REQ: issue-artifacts-only

No subcommand of `specscore issue` may mutate `spec/features/*/README.md`. The canonical write targets are `spec/issues/<slug>.md` and `spec/features/<feature-slug>/issues/<slug>.md`.

### Lint-clean output

Every artifact scaffolded or mutated under this group MUST pass `specscore spec lint` after the operation completes.

#### REQ: lint-clean-on-mutation

A file produced or modified by any `specscore issue` mutation command MUST satisfy every lint rule applicable to the Issue document kind. Commands that cannot produce a lint-clean artifact for the given inputs (e.g., a bad slug, missing parent Feature) MUST exit with a failure code BEFORE writing the file.

### Dual-location awareness

All commands in this group MUST support both root-level (`spec/issues/`) and Feature-scoped (`spec/features/<feature-slug>/issues/`) issue locations.

#### REQ: dual-location-support

Query commands (`list`) MUST scan both locations. Mutation commands (`new`, `change-status`) MUST resolve or create at the correct location based on flags. Slug resolution for `change-status` MUST search both locations and fail with exit `3` if not found in either.

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [issue-artifact-type](https://github.com/specscore/specstudio-skills/blob/main/spec/features/issue-artifact-type/README.md) | Defines the required shape of an Issue. `issue new` produces instances. |
| [cli/spec/lint/issue-rules](../spec/lint/issue-rules/README.md) | The `I-001`–`I-015` lint rules that validate Issue artifacts. `issue new` produces lint-clean files; `change-status` runs lint after mutation. |
| [lifecycle-transitions](../lifecycle-transitions/README.md) | Shared contract for status-mutation verbs. `issue change-status` implements this contract. |
| [CLI](../README.md) | Inherits shared exit-code contract, `--project`, `--format`. |

## Acceptance Criteria

### AC: group-help-lists-subcommands

**Given** the CLI is built with this Feature's changes
**When** `specscore issue --help` is run
**Then** the output lists `new`, `change-status`, and `list` as available subcommands

### AC: unknown-subcommand-rejected

**Given** the CLI is built with this Feature's changes
**When** `specscore issue banana` is run
**Then** the command exits with a usage error naming the unknown subcommand

## Open Questions

- Should `specscore issue info <slug>` be added for parity with `feature info`?
- Should `specscore issue relocate` be added for moving issues between root and Feature-scoped locations?

---
*This document follows the https://specscore.md/feature-specification*
