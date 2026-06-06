---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Agent Setup (CLI)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/agent/setup?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/agent/setup?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/agent/setup?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/agent/setup?op=request-change) |

**Status:** Approved
**Source Ideas:** ai-agent-configuration-cli, agent-setup-skill-bundles

## Summary

`specscore agent setup` generates agent-specific instruction/rules files for AI coding agents in a SpecScore-managed project. Each generated file teaches the agent about SpecScore conventions, the spec tree structure, key CLI commands (with the correct `--caller` flag), and (where applicable) plugin installation instructions. The command supports seven agents in its MVP: Claude Code, Codex, GitHub Copilot, Cursor, Antigravity, Pi, and OpenCode. For agents that read skills from a known directory (Claude and Cursor), the command also copies SpecScore's invokable skill bundles into that directory, so agents without a skill marketplace get the same skills Claude Code installs from its plugin. Every path the command adds, modifies, or skips is reported.

## Synopsis

```
specscore agent setup <agent-name>... [--all] [--force] [--no-skills] [--project <path>]
```

Agent names may be given as separate positional arguments (`claude codex`) or as a single comma-separated value (`claude,codex`).

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

#### REQ: comma-separated-agent-list

Agent names MAY be supplied as a single comma-separated value (`claude,codex`), as separate positional arguments (`claude codex`), or any mix (`claude,codex cursor`). The command MUST split each positional argument on commas, trim surrounding whitespace, and discard empty fields before validating each resulting name against the supported set per `agent-name-validation`. A comma-separated list is equivalent to the same names passed positionally.

### Supported agents

#### REQ: supported-agents-mvp

The MVP supports seven agents with the following config file mappings. The **Skills Dir** column gives the directory into which skill bundles are copied; `—` marks agents with no known skills directory (instruction file only).

| Agent | Caller ID | Config File | Skills Dir |
|---|---|---|---|
| Claude Code | `claude` | `CLAUDE.md` | `.claude/skills/` |
| GitHub Copilot | `copilot` | `.github/copilot-instructions.md` | — |
| Cursor | `cursor` | `.cursor/rules/specscore.mdc` | `.cursor/skills/` |
| Codex (OpenAI) | `codex` | `codex.md` | — |
| Antigravity (Google) | `antigravity.google` | `GEMINI.md` | — |
| Pi (StackBlitz) | `pi.dev` | `AGENTS.md` | — |
| OpenCode | `opencode` | `AGENTS.md` | — |

Codex uses `codex.md` (not `AGENTS.md`) to avoid clobbering the project's existing `AGENTS.md` maintainer documentation. Pi and OpenCode both target `AGENTS.md`; when both are requested in the same run, the first writes the file and the second is skipped with an informational message.

#### REQ: skills-dir-agents-mvp

Only agents with a confirmed skills directory receive skill bundles: `claude` (`.claude/skills/`) and `cursor` (`.cursor/skills/`). Agents whose Skills Dir is `—` MUST receive only their instruction file; the command MUST NOT invent a skills directory for them. As other agents adopt a stable skills-directory convention, they are added to this set.

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

### Skill bundle copying

For agents with a skills directory, `agent setup` copies SpecScore's invokable skill bundles into that directory alongside writing the instruction file. This gives agents without a skill marketplace (notably Cursor) the same skills Claude Code installs from its plugin.

#### REQ: skill-copy-default-on

For each requested agent that has a skills directory (per `skills-dir-agents-mvp`), the command MUST copy the SpecScore skill bundles into that directory by default — no extra flag required. Agents without a skills directory are unaffected.

#### REQ: no-skills-flag

When `--no-skills` is supplied, the command MUST NOT copy any skill bundles for any agent; it writes only the instruction files. With `--no-skills`, the command's file output is identical to its behavior before skill copying existed.

#### REQ: skill-bundle-source

The skill bundles are downloaded from GitHub via the SpecScore marketplace registry, in three steps: (1) fetch the marketplace manifest (`specscore/ai-marketplace` → `.claude-plugin/marketplace.json`); (2) for each referenced plugin, fetch that plugin repo's manifest; (3) download the skill files the plugin manifests enumerate. The marketplace base location is overridable via an environment variable for testing and self-hosted mirrors, defaulting to the public GitHub source. There is no binary-embedded fallback: when GitHub is unreachable, or any manifest or skill file cannot be fetched, the command MUST still write the instruction files, report a clear error for the skill-copy step that names the unreachable source, exit `10`, and MUST NOT leave a partial skill directory behind.

#### REQ: skill-copy-raw-markdown

Skill bundles MUST be copied as raw skill files (markdown and any sibling assets) into a per-skill subdirectory of the agent's skills directory. The command MUST NOT transpile, reformat, or rewrite skill content per agent in this MVP — bytes are copied as-is.

#### REQ: skill-copy-idempotent

Skill copying follows the same idempotency rule as instruction files: an existing skill file MUST be preserved byte-identical and skipped unless `--force` is supplied. `--force` overwrites existing skill files with the freshly sourced content. Claude is copied to like any other skills-directory agent — the skip-existing rule, not a marketplace check, prevents clobbering a plugin-managed install.

#### REQ: skills-parent-dirs-created

The agent's skills directory and per-skill subdirectories MUST be created automatically when absent, consistent with `parent-dirs-created`.

### Change reporting

#### REQ: per-path-change-report

The command MUST report every path it touches, one line per path, each prefixed with an explicit action: `added <path>` for a newly written file, `modified <path>` for a path overwritten under `--force`, and `skipped <path>` for an existing path left untouched (the skip line MUST mention `--force`). The report covers both instruction files and copied skill files. When the command makes no changes (everything skipped), it still prints the skip lines and exits `0`.

| Code | Condition |
|---|---|
| `0` | At least one file created, or all requested files already exist (idempotent) |
| `2` | No agents specified, unknown agent name, `--all` combined with positional args, or invalid `--project` path |
| `3` | Project root not found (no `specscore.yaml` or `spec/features/` in any ancestor) |
| `6` | Project root found but `specscore.yaml` absent (e.g., found via `spec/features/` fallback) |
| `10` | Unexpected I/O failure, or skill bundles could not be downloaded from the GitHub marketplace source |

#### REQ: exit-code-discipline

The command MUST exit with one of the documented codes above and no other code. Unexpected conditions surface as code `10` with a descriptive stderr message.

## Parameters

| Flag | Type | Default | Description |
|---|---|---|---|
| `--all` | bool | `false` | Configure all supported agents |
| `--force` | bool | `false` | Overwrite existing config files and skill files |
| `--no-skills` | bool | `false` | Write instruction files only; do not copy skill bundles |
| `--project` | string | cwd | Project root directory (autodetected from current directory if omitted) |

Positional arguments: one or more agent names from the supported set, given separately (`claude codex`) or comma-separated (`claude,codex`).

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
**Then** `CLAUDE.md` is replaced with freshly rendered content; stdout reports it as `modified`; the command exits `0`.

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

### AC: comma-separated-agents

**Requirements:** cli/agent/setup#req:comma-separated-agent-list

**Given** an initialized SpecScore project
**When** `specscore agent setup claude,cursor --project <root>` runs
**Then** both `CLAUDE.md` and `.cursor/rules/specscore.mdc` are created exactly as if `claude cursor` had been passed as separate arguments; the command exits `0`.

### AC: skill-copy-cursor-default

**Requirements:** cli/agent/setup#req:skill-copy-default-on, cli/agent/setup#req:skills-dir-agents-mvp, cli/agent/setup#req:skill-copy-raw-markdown, cli/agent/setup#req:skills-parent-dirs-created

**Given** an initialized SpecScore project with no `.cursor/` directory
**When** `specscore agent setup cursor --project <root>` runs
**Then** `.cursor/skills/` is created and populated with per-skill subdirectories whose skill files match the source bundles byte-for-byte; the command exits `0`.

### AC: skill-copy-claude-always

**Requirements:** cli/agent/setup#req:skill-copy-idempotent

**Given** an initialized SpecScore project
**When** `specscore agent setup claude --project <root>` runs
**Then** `.claude/skills/` is created and populated with the SpecScore skill bundles regardless of whether the `ai-plugin-specscore` marketplace plugin is installed; the command exits `0`.

### AC: no-skills-suppresses-copy

**Requirements:** cli/agent/setup#req:no-skills-flag

**Given** an initialized SpecScore project
**When** `specscore agent setup cursor --no-skills --project <root>` runs
**Then** `.cursor/rules/specscore.mdc` is created but no `.cursor/skills/` directory is created; the command exits `0`.

### AC: non-skilldir-agent-instruction-only

**Requirements:** cli/agent/setup#req:skills-dir-agents-mvp

**Given** an initialized SpecScore project
**When** `specscore agent setup codex --project <root>` runs
**Then** `codex.md` is created and no skills directory is created for Codex; the command exits `0`.

### AC: skill-copy-skips-existing

**Requirements:** cli/agent/setup#req:skill-copy-idempotent

**Given** an initialized SpecScore project where a skill file under `.cursor/skills/` already exists with `custom content`
**When** `specscore agent setup cursor --project <root>` runs without `--force`
**Then** that skill file is preserved byte-identical; stdout contains a skip line mentioning `--force`; the command exits `0`.

### AC: change-report-output

**Requirements:** cli/agent/setup#req:per-path-change-report

**Given** an initialized SpecScore project with no existing agent files
**When** `specscore agent setup cursor --project <root>` runs
**Then** stdout contains an `added` line for `.cursor/rules/specscore.mdc` and an `added` line for each copied skill file; the command exits `0`.

### AC: skill-source-offline-fails

**Requirements:** cli/agent/setup#req:skill-bundle-source

**Given** an initialized SpecScore project with the GitHub marketplace source unreachable
**When** `specscore agent setup cursor --project <root>` runs
**Then** `.cursor/rules/specscore.mdc` is still written, the skill-copy step reports an error naming the unreachable source, no partial `.cursor/skills/` content is left behind, and the command exits `10`.

## Open Questions

- Should `agent setup` also generate `.gitignore` entries for agent-specific files that users might not want tracked (e.g., `.cursor/`)? Some teams track these, others don't.
- Should a future `agent remove <name>` verb be added to clean up generated files?
- Should the command support `--dry-run` to preview which files would be created without writing them?
- Which agents beyond Claude and Cursor have a stable skills-directory convention worth adding to `skills-dir-agents-mvp` (Codex, Copilot, Antigravity, Pi, OpenCode)?
- Should copied skill bundles eventually be transpiled per agent (e.g., Cursor-native skill/command format) rather than copied as raw markdown?
- How is the copied skill-bundle set versioned relative to the CLI release, and should the embedded fallback snapshot be pinned per release?

---
*This document follows the https://specscore.md/feature-specification*
