---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Plan (CLI)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/plan?op=request-change) |
**Status:** Approved
**Date:** 2026-06-04
**Owner:** alexander.trakhimenok
**Source Ideas:** specscore-cli-should-expose-a-plan-verb-with-list-and-query
**Supersedes:** —

## Summary

`specscore plan` commands work with Plan artifacts in the routed Plans namespace (same-repository `spec/plans/` by default, or an external repository's `spec/plans/{host}/{owner}/{repo}/` when routed there — see REQ:plan-repository-routing) — listing them, inspecting one plan's metadata and task rollup, reporting whether its declared prerequisites permit execution, scaffolding a new plan, transitioning a plan's lifecycle status, and correcting the record when work landed outside the tracked flow — so agents and humans can answer "what plans exist and what status do they hold?", determine which plan may execute next, create new ones, advance them through one stable entry point, and fix the record when it fell behind reality. The `list`, `info`, and `readiness` subcommands are read-only; `new` scaffolds a fresh plan; `change-status` transitions an existing plan's `**Status:**` without creating one; `reconcile` corrects an existing plan's `**Status:**` (and its embedded tasks') out of band, when `change-status`'s legal-transition matrix cannot reach where the work actually already is.

## Contents

| Child | Description |
|---|---|
| [list](list/README.md) | Flat, alphabetically sorted list of plan slugs, with optional `--status` filter and structured output |
| [info](info/README.md) | Metadata and task rollup for a single plan |
| [new](new/README.md) | Scaffold a lint-clean plan (body + `format:`/`status:` frontmatter) against a Source Feature or Idea |
| [readiness](readiness/README.md) | Stable query of whether declared prerequisites permit execution, with every unmet slug/status |
| [change-status](change-status/README.md) | Transition a plan's `**Status:**` through the human-authored prep band and dispositions |
| [reconcile](reconcile/README.md) | Correct a plan's `**Status:**` and embedded task statuses to match work delivered outside the tracked flow — evidence-gated, never a silent bypass |

## Problem

Plans (`spec/plans/*.md`) are the only first-class spec artifact without a CLI query surface. The top-level verbs are `idea`, `feature`, `task`, `issue`, and `proposal` — there is no `plan`, so `specscore plan` errors with `Unknown command "plan"`. The `pkg/plan` package already parses plans (tasks, source feature, deferred-AC coverage) for lint, but exposes no command surface and no plan-level status. As a result, answering "what plans are open?" requires `ls spec/plans/` plus a `grep` over the `**Status:**` line, and stale plan statuses cannot be surfaced programmatically. A structured query surface — mirroring the existing `feature` and `idea` groups — lets scripts, agents, and humans navigate plans through one stable entry point.

## Behavior

### Command group

The `plan` group is additive. Its query verbs introduce no changes to how plans are authored or stored; its single create verb scaffolds new plans only.

#### REQ: subcommands

`specscore plan` MUST expose the `list`, `info`, `new`, `readiness`, `change-status`, and `reconcile` subcommands. Invoking `specscore plan` with no subcommand MUST print the group help and exit `0` (not error as an unknown command).

#### REQ: mutation-scope

The `list`, `info`, and `readiness` subcommands MUST NOT create, edit, or transition plan files — they read `spec/plans/*.md` only. The `new` subcommand (see [new](new/README.md)) MAY create a new plan file but MUST NOT edit or transition existing plans. The `change-status` subcommand (see [change-status](change-status/README.md)) transitions a plan's lifecycle status through the legal-transition matrix; the `reconcile` subcommand (see [reconcile](reconcile/README.md)) corrects a plan's status and embedded task statuses OUTSIDE that matrix, for work already delivered but never recorded. Neither `change-status` nor `reconcile` ever creates a plan.

### Shared flags

Every command in this group accepts the shared flags defined in the [CLI parent](../README.md): `--project`, `--format`, and `-h/--help`.

#### REQ: format-selection

`--format` MUST accept `yaml`, `json`, and `text`. The default format is per-subcommand (`text` for `list`, `yaml` for `info`). Any other value MUST exit `2` (InvalidArgs) with a message naming the offending value.

### Plan-slug resolution

A plan's slug is its filename without the `.md` extension for the legacy flat form (e.g., `cli-rules`), or its path under the Plans namespace for the canonical directory form (e.g., `cli-rules` resolving to `cli-rules/README.md`; nested plans use slash-separated slugs such as `roadmap/child`).

#### REQ: slug-resolution

Commands that take a `<slug>` argument MUST resolve it against the routed Plans namespace (see REQ:plan-repository-routing below): the canonical directory form `<plans-dir>/<slug>/README.md` first, falling back to the legacy flat form `<plans-dir>/<slug>.md` when the directory form does not exist. Both forms existing for the same slug is a conflict, not an implicit winner.

#### REQ: not-found-exit-code

When a `<slug>` does not resolve to an existing plan file, commands MUST exit `3` (NotFound) with a message that names the requested slug.

### Plan repository routing

Every Plan operation resolves ONE authoritative Plans namespace before touching any Plan artifact — same-repository (`<spec>/plans/`) by default only when explicitly self-routed, or an external repository's `spec/plans/{host}/{owner}/{repo}/` namespace when routed there. The full precedence, identity-normalization, and checkout-validation rules are the [Repo Config](../../repo-config/README.md) Feature's contract (canonical upstream, not duplicated here); this section covers only how the CLI applies it.

#### REQ: plan-repository-routing

Every dedicated Plan verb (`list`, `info`, `new`, `readiness`, `change-status`, `reconcile`, and any `task` command that writes a plan-embedded task) MUST resolve a Plans repository route before reading or writing any Plan artifact. An absent or ambiguous route MUST fail before touching Plan artifacts, with guidance naming the config files a route can be set in (`specscore.yaml`, `specscore.local.yaml`, the organization layer, or `~/.specscore.yaml`). Feature, Idea, and Lesson operations are unaffected by an absent or unresolved Plan route — they MUST NOT be gated on Plan routing.

#### REQ: project-flag-names-source

`--project` always names the SOURCE project — the repository whose Features/ACs a Plan's `**Source Feature:**`/`**Verifies:**` lines reference — even when the resolved Plans namespace lives in a different, externally-routed repository checkout. Running a Plan command from within the Plans checkout itself does not change what `--project` (or its cwd-autodetected default) names.

### Plan status

A plan's lifecycle status is the value of its `**Status:**` body-metadata line, read verbatim — the same trust model `feature` and `idea` use. Detecting drift between a plan's status and its Source Feature is out of scope for this group.

#### REQ: status-from-file

The reported status of a plan MUST be the literal value of its `**Status:**` line. A plan whose `**Status:**` line is missing or blank MUST report an empty/unset status rather than failing the command.

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [CLI](../README.md) | Inherits shared exit-code contract, `--format`/`--project` conventions, and project autodetection. |
| [cli/feature](../feature/README.md) | Sibling query group whose `list`/`info` contract this group mirrors. Feature operations are exempt from REQ:plan-repository-routing. |
| [Repo Config](../../repo-config/README.md) | Canonical authority for `plans_repo`, `plan_repos`, and `repo_checkouts` — schema, precedence across `specscore.local.yaml` / `specscore.yaml` / the organization layer / `~/.specscore.yaml`, and identity normalization. This Feature applies that contract; it does not restate it. |

## Acceptance Criteria

### AC: group-exposes-subcommands (verifies REQ:subcommands)

**Given** a project with a `spec/plans/` directory
**When** the user runs `specscore plan`
**Then** the group help is printed listing `list`, `info`, `new`, `readiness`, `change-status`, and `reconcile`, and the command exits `0`.

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

### AC: unrouted-project-fails-with-guidance (verifies REQ:plan-repository-routing)

**Given** a project with a lint-clean `specscore.yaml` that declares no `plans_repo` and no applicable `plan_repos` mapping in any config layer
**When** the user runs `specscore plan list`
**Then** the command fails before reading `spec/plans/`, and the error names `specscore.yaml`, `specscore.local.yaml`, and `~/.specscore.yaml` as the files a route can be set in.

## Open Questions

- Should `--status` validate against a canonical plan-status set, or string-match freely? `feature` and `idea` validate against a defined enum; plans have no canonical status set yet (observed values: `Approved`, `Completed`).
- Is `Completed` the terminal plan status, or should "done" be inferred from all tasks being `done`?

---
*This document follows the https://specscore.md/feature-specification*
