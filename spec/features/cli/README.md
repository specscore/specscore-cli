---
format: https://specscore.md/feature-specification
status: Implementing
---

# Feature: CLI

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli?op=request-change) |

**Status:** Implementing
**Source Ideas:** —

## Summary

The `specscore` CLI is the tooling for working with SpecScore specification repositories. It validates specs, queries the feature tree, inspects source-to-spec links, manages tasks, scaffolds new artifacts, and reports its own identity.

This feature is the umbrella for command-level specifications. Each command or command group owns a child feature directory with its own contract — flags, output, exit codes, and behavior.

## Problem

Commands that grew inside the codebase without written specs accrete inconsistencies: output formats drift between releases, exit codes become arbitrary, flag names repeat the same idea in different words, and script authors cannot tell which behaviors are guaranteed versus incidental. A CLI that is the primary interface to a specification framework should itself be specified — both to pin its own contract and to dogfood the format on its own tooling.

## Contents

| Directory | Description |
|---|---|
| [agent/](agent/README.md) | AI coding agent integration — generate agent-specific instruction files (`CLAUDE.md`, `.cursorrules`, etc.) for SpecScore projects |
| [code/](code/README.md) | Source-code → SpecScore relationship queries |
| [entity/](entity/README.md) | Entity Doc-Kind: lint enforcement, managed-section rendering, `specscore entity list/refs/tree` |
| [feature/](feature/README.md) | Feature tree queries and scaffolding |
| [graph/](graph/README.md) | GraphSpec command group for authoring, validating, navigating, documenting, and reviewing GraphSpec artifacts. |
| [idea/](idea/README.md) | Idea artifact scaffolding and lifecycle |
| [init/](init/README.md) | `specscore init` scaffolds a SpecScore-managed project root: creates `specscore.yaml`, the `spec/` tree with lint-clean Ideas and Features indexes, and optional `spec/research`/`spec/decisions` trees. Refuses to clobber an existing `specscore.yaml` unless `--force`. |
| [issue/](issue/README.md) | Issue artifact scaffolding, lifecycle, and listing |
| [lifecycle-transitions/](lifecycle-transitions/README.md) | _Not a command group._ Shared contract every status-mutating verb satisfies (atomicity, rollback, output format, exit-code mapping). |
| [property/](property/README.md) | Property Doc-Kind: lint enforcement, managed-section rendering, `specscore property list/refs` |
| [spec/](spec/README.md) | Specification-tree validation and search |
| [task/](task/README.md) | Task board management |
| [version/](version/README.md) | CLI version reporting |
| [consilium](consilium/README.md) | The deterministic consilium engine: gate-rule arbitration, vote-schema and roster validation, gate configuration, and the parent command. |
| [event](event/README.md) | TODO: Add description. |
| [lesson](lesson/README.md) | Record and query process-gap lessons and advance them through their enforcement lifecycle. |
| [plan](plan/README.md) | Query Plan artifacts, including plan metadata and task rollups. |
| [publication-policy](publication-policy/README.md) | Mutate publication policy, resolve effective policy, validate branch guards, and support manifest-based publication. |
| [rehearse](rehearse/README.md) | Rehearse acceptance-evidence command group. |
| [rules](rules/README.md) | Discover lint rules, generate the lint-rule catalog, and check it for drift. |
| [self-update](self-update/README.md) | Detect the install method and perform verified CLI updates or redirect package-managed installs. |
| [sidekick](sidekick/README.md) | Scaffold sidekick-seed artifacts. |
| [studio](studio/README.md) | SpecScore Studio commands for indexing, facts, asking questions, serving, and MCP. |
| [telemetry](telemetry/README.md) | TODO: Add description. |
| [parked](parked/README.md) | A scheduling axis, orthogonal to Status, marking an artifact as deliberately deferred without touching its lifecycle maturity. |

### agent

Configures AI coding agents (Claude Code, Codex, GitHub Copilot, Cursor, Antigravity, Pi, OpenCode) for SpecScore-managed projects. `agent setup` generates agent-specific instruction/rules files that teach each agent about the spec tree, CLI commands, `--caller` telemetry flag, and available plugins.

### code

Queries relationships from source files to SpecScore resources. Scans `specscore:` annotations and URLs embedded in source comments and reports the features, plans, or docs those files depend on. Read-only.

### entity

CLI surface for the Entity Doc-Kind. Hosts the `entity-*` lint rules (frontmatter shape, additive-only inheritance, managed-section rendering for `## Properties` and `## Referenced by`) and three navigation verbs: `specscore entity list`, `specscore entity refs <id>`, `specscore entity tree`. Authoritative entity Doc-Kind contract lives in the meta-spec [entity Feature](https://github.com/specscore/specscore/blob/main/spec/features/entity/README.md); this CLI Feature is the implementation surface.

### feature

Queries the feature tree: list every feature, inspect a feature's metadata and section TOC, view the hierarchy as a tree, and follow dependency / reference chains. Also hosts `feature new`, which scaffolds a new feature directory with a lint-clean README.

### graph

Provides the CLI surface for GraphSpec. `graph new` scaffolds GraphSpec artifacts, validation commands check graph structure and references, navigation commands answer ownership/reference/dependency questions, documentation commands render diagrams and reports, and future AI commands prepare review and explanation packages. GraphSpec remains the language owner; the CLI consumes GraphSpec.

### idea

Scaffolds Idea artifacts at `spec/ideas/<slug>.md` and hosts lifecycle verbs (`idea new`, `idea list`, `idea change-status`). `idea new` emits a lint-clean Idea skeleton with HTML-comment prompts for every required section; with `--type=change-request --targets=<feature-slug>` it scaffolds a Proposal at `spec/features/<target>/proposals/<slug>.md`. `idea list` enumerates Ideas (excluding archived by default; use `--include-archived` to include them). Lifecycle verbs follow the [lifecycle-transitions](lifecycle-transitions/README.md) shared contract.

### issue

Manages Issue artifacts — reported observations of broken behavior. `issue new` scaffolds a lint-clean Issue skeleton at `spec/issues/<slug>.md` or Feature-scoped at `spec/features/<feature>/issues/<slug>.md`. `issue change-status` transitions an issue through its lifecycle (`open → investigating → resolved|rejected`) with severity and rejection-reason gating. `issue list` aggregates issues from both locations with status, severity, and Feature filters. Lifecycle verbs follow the [lifecycle-transitions](lifecycle-transitions/README.md) shared contract.

### lesson

Records and queries process-gap lessons — `spec/lessons/<slug>.md` files climbing a three-rung enforcement ladder (`Recorded` → `Stated` → `Enforced`) or retired via `Withdrawn`/`Superseded`. `lesson new` scaffolds a lint-clean skeleton with its four required sections (`Incident`, `Process gap`, `Check`, `Enforcement`); `lesson list --status=recorded` answers "what have we learned but not yet enforced?" in one command; `lesson recur` records that a gap manifested again without itself changing status. `change-status` follows the [lifecycle-transitions](lifecycle-transitions/README.md) shared contract.

### proposal

Convenience alias group for change-request Ideas. `proposal new <feature-slug> <slug>` delegates to `idea new <slug> --type change-request --targets <feature-slug>`. All `idea new` flags are forwarded.

### lifecycle-transitions

Not a command group — has no CLI surface of its own. Defines the shared cross-cutting contract every `specscore` verb that mutates an artifact's `Status` field satisfies: atomicity, rollback, output format, exit-code mapping, slug-positional argument, no coordination, and the scope boundary against coordination/concurrency concerns. Verb features (e.g., `cli/idea/approve`) reference this feature instead of restating these rules.

### property

CLI surface for the Property Doc-Kind. Hosts the `property-*` lint rules (frontmatter shape, `data_type` enumeration, check-key applicability, managed-section rendering for `## Referenced by`) and two navigation verbs: `specscore property list`, `specscore property refs <id>`. Authoritative property Doc-Kind contract lives in the meta-spec [property Feature](https://github.com/specscore/specscore/blob/main/spec/features/property/README.md); this CLI Feature is the implementation surface.

### spec

Validates the specification tree. Hosts `spec lint`, which runs the full checker suite (structural conventions, adherence footers, OQ sections, index completeness, and Idea-specific rules) and optionally applies autofixes. Future gated integrations may validate independent specifications such as ModelSpec while preserving their ownership boundaries. Reports violations with severity levels.

### task

Manages the project task board at `tasks/README.md` and individual task files under `tasks/<slug>/README.md`. Supports listing, inspecting, and creating tasks. Task status transitions and claim/release semantics are not part of the MVP surface — this group is the minimum needed to read and seed a board.

### version

Reports the CLI's build identity. `specscore version` prints the full human-readable line; `specscore --version` (and `-v`) prints the bare semver for scripts. See [version/README.md](version/README.md) for the full contract.
## Behavior

### Command-naming conventions

Commands follow a `specscore <resource> <action>` pattern with singular nouns and verb subcommands, matching the style of `gh`, `kubectl`, and `docker`.

#### REQ: singular-resource-names

Resource names in command paths MUST be singular (`feature`, `task`, `idea`), never plural. The resource name identifies a *type*; pluralization is an output-shape concern, not a command-name one.

#### REQ: verb-subcommands

Every action MUST be an explicit subcommand verb (`list`, `info`, `new`, `deps`, `refs`, `tree`, `lint`). A bare resource name (e.g., `specscore feature`) MUST show help — it MUST NOT perform an implicit default action like listing.

#### REQ: prefer-new-over-create

Commands that create new artifacts MUST use the verb `new`, never `create`. This matches the convention used by `gh issue new`, `gh pr new`, and similar resource-oriented CLIs.

### Shared exit-code contract

Every `specscore` command MUST observe the following exit-code contract. These codes match the constants exported by [`pkg/exitcode`](../../../pkg/exitcode), which the CLI uses uniformly.

| Exit code | Meaning |
|---|---|
| `0` | Success |
| `1` | Conflict (concurrent modification, stale read) |
| `2` | Invalid arguments (missing required flag, bad flag value, malformed input) |
| `3` | Resource not found |
| `4` | Invalid state transition |
| `5` | Ambiguous slug (auto-resolution found multiple candidates) |
| `6` | Target directory is not a SpecScore-managed repo |
| `7` | Working tree has uncommitted changes in paths to be modified |
| `8` | Unsupported subcommand (outdated `specscore` that predates a required subcommand) — distinct from the shell's `127` (binary absent) |
| `10` | Unexpected / catch-all runtime error |

Exit codes `9` and `11–19` are reserved for future standard codes and MUST NOT be used by individual commands.

#### REQ: standard-exit-codes

Commands MUST map errors to the standard code with the matching semantics. A command that has no notion of "conflict" or "invalid state transition" simply never returns those codes; it does not repurpose them.

#### REQ: error-on-stderr

On any non-zero exit, a human-readable explanation MUST be written to stderr. stdout MUST remain free of error prose so that pipelines consuming structured output (YAML/JSON) are not corrupted by error messages.

#### REQ: unsupported-subcommand-mapping

When a subcommand is not recognized — typically because the installed `specscore` predates a subcommand the caller requires — the CLI MUST exit `8` (UnsupportedCommand) rather than a generic `1`/`2`/`10`. This lets callers, notably capability-gated agent skills, distinguish a present-but-outdated binary from a genuine command failure; a wholly-absent binary remains the shell's `127`. The mapping is scoped to unknown subcommands only: unknown flags keep their `2` (InvalidArgs) semantics, and an error that already carries an exit code MUST NOT be reclassified. A human-readable message naming the unrecognized subcommand MUST be written to stderr.

### Output format conventions

Most read commands support `--format` for selecting between `text`, `yaml`, and `json`. Some also support `md` (task list).

#### REQ: yaml-default-for-structured

Read commands that return structured data (feature info, feature list, task info, task list, feature deps, feature refs, feature tree) MUST default to YAML output. `--format json` and `--format text` MUST be accepted as alternatives where documented on the individual command.

#### REQ: stable-yaml-keys

YAML and JSON output keys are part of the command's contract. Renaming or removing a key is a breaking change and MUST follow the deprecation path (announce in release notes, keep the old key for at least one release cycle). Adding new keys is always allowed.

### Shared flags

Several flags appear across multiple commands with identical semantics:

| Flag | Semantics |
|---|---|
| `--project` | Path to the project root. Autodetected from `cwd` (walks up until finding `specscore.yaml`) when omitted. |
| `--format` | Output format. Allowed values vary by command (always a subset of `yaml`, `json`, `text`, `md`). |
| `-h`, `--help` | Print help and exit `0`. Provided by cobra; commands MUST NOT override it. |

#### REQ: project-autodetect

When `--project` is not supplied, commands MUST autodetect the project root by searching upward from the current working directory for `specscore.yaml`. If no project is found, commands MUST exit `3` (NotFound) with a clear message.

### Documentation

User-facing surfaces must not silently drift from the implemented interface.

#### REQ: docs-track-interface-changes

Any change to a user-facing CLI interface — a new or renamed command or subcommand, an added / removed / renamed flag, a changed exit code, or a changed default output shape — MUST update the relevant documentation in the same change set:

- the command's child Feature spec under `cli/` — the canonical contract, surfaced on the website at `specscore.md/<command-path>`;
- this repository's `README.md`, when the command is part of its documented surface (the `## Usage` / `## Updating` sections); and
- any narrative install / usage pages on the website (authored in the [`specscore/specscore`](https://github.com/specscore/specscore) repo).

The Feature spec is the source of truth the website renders from; the `README.md` and website prose are hand-maintained and do NOT update themselves. A change that alters the interface without updating these surfaces is incomplete.

## Acceptance Criteria

### AC: unknown-subcommand-exits-8

**Requirements:** cli#req:unsupported-subcommand-mapping

**Given** an installed `specscore` whose command set does not include the requested subcommand (for example, `specscore consilium verdict` on a build that predates the `verdict` subcommand)
**When** the user invokes that unrecognized subcommand
**Then** the command exits `8` (UnsupportedCommand) and writes a stderr message naming the unrecognized subcommand; no output appears on stdout.

## Consumers

The [`ai-plugin-specscore`](https://github.com/specscore/ai-plugin-specscore) Claude Code plugin wraps every command group below as an agent skill. Each skill loads per-verb references on demand and treats the feature spec in this tree as the authoritative contract — when a flag, exit code, or output shape changes here, the corresponding skill follows. The wrapper-skill catalogue lives at [`skills/README.md`](https://github.com/specscore/ai-plugin-specscore/blob/main/skills/README.md#planned-cli-wrapper-catalogue).

## Open Questions

- The MVP task surface (list, info, new) does not include status transitions (claim, release, status update). Should those land as part of this feature or in a future `cli/task/status/…` expansion that tracks a `task-lifecycle` feature spec?
- Several commands (`feature deps`, `feature refs`, `feature tree`, `feature list`) share a `--fields` flag with overlapping semantics. Should that flag be promoted to a shared-flag REQ in this parent feature, or stay documented per-command until the semantics fully converge?
- Commands currently do not emit a machine-readable error envelope on non-zero exit — error details go to stderr as free prose. Should stderr output for structured formats (`--format json`, `--format yaml`) also be structured (JSON/YAML error object), or is free prose + exit code sufficient for the CLI's callers?

---
*This document follows the https://specscore.md/feature-specification*
