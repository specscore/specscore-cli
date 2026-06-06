---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: Agent Setup (CLI)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/agent/setup?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/agent/setup?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/agent/setup?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/agent/setup?op=request-change) |

**Status:** Stable
**Source Ideas:** ai-agent-configuration-cli

## Summary

`specscore agent setup` generates agent-specific instruction/rules files for AI coding agents in a SpecScore-managed project. Each generated file teaches the agent about SpecScore conventions, the spec tree structure, key CLI commands (with the correct `--caller` flag), and (where applicable) plugin installation instructions. The command supports seven agents in its MVP: Claude Code, Codex, GitHub Copilot, Cursor, Antigravity, Pi, and OpenCode.

## Synopsis

```
specscore agent setup <agent-name>... [--all] [--force] [--project <path>]
```

## Problem

SpecScore-managed projects benefit greatly from AI coding agents that understand the spec tree, lifecycle conventions, and CLI commands. Today, teaching an agent about SpecScore requires manually authoring an instruction file (`CLAUDE.md`, `.cursorrules`, etc.) with project-specific content, or knowing about the `ai-plugin-specscore` Claude Code plugin. Most users skip this step, leading to agents that ignore specs or invoke the CLI without `--caller`, making telemetry segmentation impossible.

`agent setup` closes this gap with a one-line command: `specscore agent setup claude copilot cursor` generates correctly structured instruction files for all three agents, each containing the project title, spec tree overview, CLI command examples with `--caller`, and agent-specific extras (plugin pointers, MDC frontmatter). The command mirrors the `init` pattern: idempotent, skip-existing-unless-forced, documented exit codes.

## Behavior

### Project root requirement

The command requires an existing SpecScore-managed project.

#### REQ: specscore-project-required

The command MUST resolve the project root via the shared `resolveSpecRoot` heuristic (walk up from `--project` or cwd looking for `specscore.yaml`). If `specscore.yaml` does not exist at the resolved root, the command MUST exit `6` (TargetNotSpecScore) with a message suggesting `specscore init`. The project root resolution follows the same convention as `idea new`, `decision new`, and other spec-mutating verbs.

### Agent selection

Users specify which agents to configure via positional arguments, `--all`, or both (which is rejected).

#### REQ: agent-names-required

At least one agent name or `--all` MUST be supplied. If neither is provided, the command MUST exit `2` (InvalidArgs) with a message listing the supported agent names.

#### REQ: agent-name-validation

Each positional agent name MUST be validated against the supported set. An unrecognised name MUST exit `2` with a message naming the unknown value and listing supported agents.

#### REQ: all-flag-exclusive

`--all` and positional agent names are mutually exclusive. Supplying both MUST exit `2`.

### Supported agents

#### REQ: supported-agents-mvp

The MVP supports seven agents with the following config file mappings:

| Agent | Caller ID | Config File |
|---|---|---|
| Claude Code | `claude` | `CLAUDE.md` |
| GitHub Copilot | `copilot` | `.github/copilot-instructions.md` |
| Cursor | `cursor` | `.cursor/rules/specscore.mdc` |
| Codex (OpenAI) | `codex` | `codex.md` |
| Antigravity (Google) | `antigravity.google` | `GEMINI.md` |
| Pi (StackBlitz) | `pi.dev` | `AGENTS.md` |
| OpenCode | `opencode` | `AGENTS.md` |

Codex uses `codex.md` (not `AGENTS.md`) to avoid clobbering the project's existing `AGENTS.md` maintainer documentation. Pi and OpenCode both target `AGENTS.md`; when both are requested in the same run, the first writes the file and the second is skipped with an informational message.

### File writing

#### REQ: idempotent-skip

When the target file already exists and `--force` is NOT supplied, the command MUST skip the file without error and print a skip message to stdout. The existing file MUST be preserved byte-identical.

#### REQ: force-overwrites

`--force` replaces existing config files. With `--force`, the file is overwritten with freshly rendered content.

#### REQ: parent-dirs-created

Parent directories (`.github/`, `.cursor/rules/`) MUST be created automatically when absent. The command MUST NOT require the user to pre-create directory structure.

#### REQ: duplicate-path-dedup

When multiple agents map to the same file path (e.g., `pi.dev` and `opencode` both target `AGENTS.md`), only the first agent in the resolved list writes the file. Subsequent agents targeting the same path MUST be skipped with an informational message.

### Content

#### REQ: content-includes-caller

Every generated file MUST include `--caller <agent-caller-id>` in its CLI command examples so the agent passes the correct telemetry caller on every invocation.

#### REQ: project-title-in-content

The generated content MUST include the project title read from `specscore.yaml`'s `project.title` field. If the field is absent or `specscore.yaml` is unparseable, the command MUST fall back to the project root directory's basename.

#### REQ: cursor-mdc-format

The Cursor config file (`.cursor/rules/specscore.mdc`) MUST use MDC format: YAML frontmatter delimited by `---` containing `description` and `alwaysApply: true`, followed by Markdown content.

#### REQ: claude-plugin-pointer

The Claude Code config file (`CLAUDE.md`) MUST include the `ai-plugin-specscore` plugin install command (`/plugin install specscore@specscore`) so users can opt in to richer agent skill integration.

### Exit codes

| Code | Condition |
|---|---|
| `0` | At least one file created, or all requested files already exist (idempotent) |
| `2` | No agents specified, unknown agent name, `--all` combined with positional args, or invalid `--project` path |
| `3` | Project root not found (no `specscore.yaml` or `spec/features/` in any ancestor) |
| `6` | Project root found but `specscore.yaml` absent (e.g., found via `spec/features/` fallback) |
| `10` | Unexpected I/O failure |

#### REQ: exit-code-discipline

The command MUST exit with one of the documented codes above and no other code. Unexpected conditions surface as code `10` with a descriptive stderr message.

## Parameters

| Flag | Type | Default | Description |
|---|---|---|---|
| `--all` | bool | `false` | Configure all supported agents |
| `--force` | bool | `false` | Overwrite existing config files |
| `--project` | string | cwd | Project root directory (autodetected from current directory if omitted) |

Positional arguments: one or more agent names from the supported set.

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [Init](../../init/README.md) | `agent setup` requires an initialized project (`specscore.yaml` must exist). Error message suggests running `specscore init` first. |
| [CLI](../../README.md) | Inherits the shared exit-code contract and `--project` flag convention. |
| [Usage Telemetry](../../telemetry/usage-telemetry/README.md) | Generated content includes `--caller` flag from the telemetry caller enum. |
| [ai-plugin-specscore](https://github.com/specscore/ai-plugin-specscore) | Claude Code content references the plugin for deeper integration. |

## Acceptance Criteria

### AC: single-agent-claude

**Requirements:** cli/agent/setup#req:specscore-project-required, cli/agent/setup#req:content-includes-caller, cli/agent/setup#req:claude-plugin-pointer, cli/agent/setup#req:project-title-in-content

**Given** an initialized SpecScore project with `project.title: "My Project"` in `specscore.yaml`
**When** `specscore agent setup claude --project <root>` runs
**Then** `CLAUDE.md` is created at the project root; its content contains the string `My Project`, the string `--caller claude`, and the string `/plugin install specscore@specscore`; the command exits `0`.

### AC: multiple-agents

**Requirements:** cli/agent/setup#req:agent-names-required, cli/agent/setup#req:parent-dirs-created

**Given** an initialized SpecScore project
**When** `specscore agent setup claude copilot cursor --project <root>` runs
**Then** `CLAUDE.md`, `.github/copilot-instructions.md`, and `.cursor/rules/specscore.mdc` are all created; parent directories `.github/` and `.cursor/rules/` are created automatically; the command exits `0`.

### AC: all-flag

**Requirements:** cli/agent/setup#req:supported-agents-mvp, cli/agent/setup#req:duplicate-path-dedup

**Given** an initialized SpecScore project
**When** `specscore agent setup --all --project <root>` runs
**Then** all seven agent config files are created (with `AGENTS.md` created once and the second agent targeting it skipped); stdout contains a skip message for the duplicate path; the command exits `0`.

### AC: unknown-agent

**Requirements:** cli/agent/setup#req:agent-name-validation

**Given** an initialized SpecScore project
**When** `specscore agent setup notreal --project <root>` runs
**Then** the command exits `2` with an error message containing `unknown agent` and listing supported agents.

### AC: no-args-no-all

**Requirements:** cli/agent/setup#req:agent-names-required

**Given** an initialized SpecScore project
**When** `specscore agent setup --project <root>` runs (no agent names, no `--all`)
**Then** the command exits `2` with a message listing supported agents.

### AC: all-and-positional-conflict

**Requirements:** cli/agent/setup#req:all-flag-exclusive

**Given** an initialized SpecScore project
**When** `specscore agent setup --all claude --project <root>` runs
**Then** the command exits `2` with a message about mutual exclusivity.

### AC: skips-existing

**Requirements:** cli/agent/setup#req:idempotent-skip

**Given** an initialized SpecScore project with an existing `CLAUDE.md` containing `existing content`
**When** `specscore agent setup claude --project <root>` runs without `--force`
**Then** `CLAUDE.md` is preserved byte-identical (`existing content`); stdout contains a skip message mentioning `--force`; the command exits `0`.

### AC: force-overwrites

**Requirements:** cli/agent/setup#req:force-overwrites

**Given** an initialized SpecScore project with an existing `CLAUDE.md`
**When** `specscore agent setup claude --force --project <root>` runs
**Then** `CLAUDE.md` is replaced with freshly rendered content; stdout reports `overwritten`; the command exits `0`.

### AC: not-specscore-repo

**Requirements:** cli/agent/setup#req:specscore-project-required

**Given** a directory with no `specscore.yaml` and no `spec/features/` in any ancestor
**When** `specscore agent setup claude --project <dir>` runs
**Then** the command exits `3` (NotFound from resolveSpecRoot).

### AC: spec-features-but-no-yaml

**Requirements:** cli/agent/setup#req:specscore-project-required

**Given** a directory with `spec/features/` but no `specscore.yaml`
**When** `specscore agent setup claude --project <dir>` runs
**Then** the command exits `6` (TargetNotSpecScore) with a message suggesting `specscore init`.

### AC: cursor-mdc-format

**Requirements:** cli/agent/setup#req:cursor-mdc-format

**Given** an initialized SpecScore project
**When** `specscore agent setup cursor --project <root>` runs
**Then** `.cursor/rules/specscore.mdc` starts with `---`, contains `alwaysApply: true` and `description:` in the frontmatter, and contains SpecScore content below the closing `---`.

### AC: fallback-title

**Requirements:** cli/agent/setup#req:project-title-in-content

**Given** a `specscore.yaml` that `ReadSpecConfig` cannot parse (e.g., missing schema header)
**When** `specscore agent setup claude --project <root>` runs
**Then** the generated `CLAUDE.md` contains the project root's directory basename as the title; the command exits `0`.

## Open Questions

- Should `agent setup` also generate `.gitignore` entries for agent-specific files that users might not want tracked (e.g., `.cursor/`)? Some teams track these, others don't.
- Should a future `agent remove <name>` verb be added to clean up generated files?
- Should the command support `--dry-run` to preview which files would be created without writing them?

---
*This document follows the https://specscore.md/feature-specification*
