# Feature: Plan (CLI)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan?op=request-change) |
**Status:** Approved
**Date:** 2026-06-04
**Owner:** alexander.trakhimenok
**Source Ideas:** specscore-cli-should-expose-a-plan-verb-with-list-and-query
**Supersedes:** —

## Summary

`specscore plan` commands query Plan artifacts in `spec/plans/` — listing them and inspecting one plan's metadata and task rollup — so agents and humans can answer "what plans exist and what status do they hold?" without hand-grepping. This group is read-only: it provides `list` and `info`, with no creation or lifecycle mutation.

## Contents

| Child | Description |
|---|---|
| [list](list/README.md) | Flat, alphabetically sorted list of plan slugs, with optional `--status` filter and structured output |
| [info](info/README.md) | Metadata and task rollup for a single plan |

## Problem

Plans (`spec/plans/*.md`) are the only first-class spec artifact without a CLI query surface. The top-level verbs are `idea`, `feature`, `task`, `issue`, and `proposal` — there is no `plan`, so `specscore plan` errors with `Unknown command "plan"`. The `pkg/plan` package already parses plans (tasks, source feature, deferred-AC coverage) for lint, but exposes no command surface and no plan-level status. As a result, answering "what plans are open?" requires `ls spec/plans/` plus a `grep` over the `**Status:**` line, and stale plan statuses cannot be surfaced programmatically. A structured query surface — mirroring the existing `feature` and `idea` groups — lets scripts, agents, and humans navigate plans through one stable entry point.

## Behavior

### Command group

The `plan` group is read-only and additive. It introduces no changes to how plans are authored or stored.

#### REQ: subcommands

`specscore plan` MUST expose the `list` and `info` subcommands. Invoking `specscore plan` with no subcommand MUST print the group help and exit `0` (not error as an unknown command).

#### REQ: read-only-scope

The `plan` group MUST NOT create, edit, or transition plan files. It reads `spec/plans/*.md` only.

### Shared flags

Every command in this group accepts the shared flags defined in the [CLI parent](../README.md): `--project`, `--format`, and `-h/--help`.

#### REQ: format-selection

`--format` MUST accept `yaml`, `json`, and `text`. The default format is per-subcommand (`text` for `list`, `yaml` for `info`). Any other value MUST exit `2` (InvalidArgs) with a message naming the offending value.

### Plan-slug resolution

Plan slugs are flat: a plan's slug is its filename without the `.md` extension (e.g., `cli-rules`). Unlike features, plans have no hierarchy, so slugs never contain `/`.

#### REQ: slug-resolution

Commands that take a `<slug>` argument MUST resolve it to `spec/plans/<slug>.md` in the project root.

#### REQ: not-found-exit-code

When a `<slug>` does not resolve to an existing plan file, commands MUST exit `3` (NotFound) with a message that names the requested slug.

### Plan status

A plan's lifecycle status is the value of its `**Status:**` body-metadata line, read verbatim — the same trust model `feature` and `idea` use. Detecting drift between a plan's status and its Source Feature is out of scope for this group.

#### REQ: status-from-file

The reported status of a plan MUST be the literal value of its `**Status:**` line. A plan whose `**Status:**` line is missing or blank MUST report an empty/unset status rather than failing the command.

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [CLI](../README.md) | Inherits shared exit-code contract, `--format`/`--project` conventions, and project autodetection. |
| [cli/feature](../feature/README.md) | Sibling query group whose `list`/`info` contract this group mirrors. |

## Acceptance Criteria

### AC: group-exposes-subcommands (verifies REQ:subcommands)

**Given** a project with a `spec/plans/` directory
**When** the user runs `specscore plan`
**Then** the group help is printed listing `list` and `info`, and the command exits `0`.

### AC: invalid-format-rejected (verifies REQ:format-selection)

**Given** a project with at least one plan
**When** the user runs `specscore plan list --format xml`
**Then** the command exits `2` and stderr names `xml` as an invalid format value.

### AC: unknown-slug-exits-3 (verifies REQ:slug-resolution, REQ:not-found-exit-code)

**Given** a project with no plan named `does-not-exist`
**When** the user runs `specscore plan info does-not-exist`
**Then** the command exits `3` with a stderr message naming `does-not-exist`, and no partial output is written to stdout.

### AC: missing-status-is-empty (verifies REQ:status-from-file)

**Given** a plan file whose `**Status:**` line is absent
**When** the user inspects it via `specscore plan info <slug>`
**Then** the reported status is empty/unset and the command still exits `0`.

## Open Questions

- Should `--status` validate against a canonical plan-status set, or string-match freely? `feature` and `idea` validate against a defined enum; plans have no canonical status set yet (observed values: `Approved`, `Completed`).
- Is `Completed` the terminal plan status, or should "done" be inferred from all tasks being `done`?

---
*This document follows the https://specscore.md/feature-specification*
