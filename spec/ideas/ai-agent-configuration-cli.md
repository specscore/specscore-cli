# Idea: AI Agent Configuration CLI

**Status:** Implemented
**Date:** 2026-05-27
**Owner:** alexander.trakhimenok
**Promotes To:** cli/agent, cli/agent/setup
**Supersedes:** —
**Related Ideas:** depends_on:cli-telemetry

## Problem Statement

How might we make it trivial for developers to configure their AI coding agents (Claude Code, Codex, GitHub Copilot, Cursor, Gemini/Antigravity, Pi, OpenCode) to understand SpecScore conventions and invoke the CLI correctly, so that agents produce spec-aligned output and pass the correct `--caller` telemetry flag?

## Context

SpecScore-managed projects benefit greatly from AI coding agents that understand the spec tree, lifecycle conventions, and CLI commands. Today, teaching an agent about SpecScore requires one of two manual steps:

1. **Hand-authoring an instruction file** — each agent has its own config format (CLAUDE.md, `.github/copilot-instructions.md`, `.cursor/rules/*.mdc`, `codex.md`, `GEMINI.md`, `AGENTS.md`). The author must know the correct file path, format, and content for each agent they use.

2. **Installing the `ai-plugin-specscore` plugin** — available only for Claude Code via the marketplace. Other agents have no plugin equivalent.

Most users skip both steps. The consequences:
- Agents ignore existing specs when scaffolding features or writing code
- CLI invocations lack `--caller`, making telemetry segmentation impossible (the [`cli-telemetry`](cli-telemetry.md) Idea explicitly depends on agents setting `SPECSCORE_CALLER` or `--caller`)
- Each new project requires re-authoring the same boilerplate instruction content

The `specscore init` command already scaffolds SpecScore project structure. An analogous `specscore agent setup` command would close the agent-configuration gap with a single line: `specscore agent setup claude copilot cursor`.

## Recommended Direction

Add a `specscore agent setup` subcommand that generates agent-specific instruction/rules files for the current SpecScore project. The command:

1. **Resolves the project root** via the shared `resolveSpecRoot` heuristic (requires `specscore.yaml`)
2. **Reads the project title** from `specscore.yaml` for personalized content
3. **Writes one config file per agent** at the agent's canonical path, with content teaching the agent about:
   - SpecScore spec tree structure (`spec/features/`, `spec/ideas/`, etc.)
   - Key CLI commands with `--caller <agent>` baked in
   - SpecScore conventions (spec-first workflow, lifecycle)
   - Agent-specific extras (Claude gets the `ai-plugin-specscore` plugin pointer; Cursor gets MDC frontmatter)

**Agent selection** is via positional arguments (`specscore agent setup claude copilot`) or `--all`. The command follows the `init` pattern: idempotent (skip existing files unless `--force`), documented exit codes, `--project` flag for non-cwd roots.

**MVP agent matrix** (7 agents):

| Agent | Caller ID | Config File |
|-------|-----------|------------|
| Claude Code | `claude` | `CLAUDE.md` |
| GitHub Copilot | `copilot` | `.github/copilot-instructions.md` |
| Cursor | `cursor` | `.cursor/rules/specscore.mdc` |
| Codex (OpenAI) | `codex` | `codex.md` |
| Antigravity (Google) | `antigravity.google` | `GEMINI.md` |
| Pi (StackBlitz) | `pi.dev` | `AGENTS.md` |
| OpenCode | `opencode` | `AGENTS.md` |

**Codex** uses `codex.md` (not `AGENTS.md`) to avoid clobbering the project's existing `AGENTS.md` maintainer documentation. **Pi and OpenCode** both target `AGENTS.md` (the cross-tool standard); when both are requested, the first writes and the second is skipped.

**Template content** is embedded as Go string constants, consistent with the `init` command's `specReadmeContent()` / `ideasIndexContent()` pattern. Each agent gets a render function `func(projectTitle string) string`.

**Plugin install aspect**: Rather than programmatically invoking external agent CLIs (unreliable — the agent might not be installed), the generated files contain actionable plugin install instructions. For Claude Code, `CLAUDE.md` includes `/plugin install specscore@specscore`.

## Alternatives Considered

- **Generate a single `AGENTS.md` for all agents.** Many agents read this cross-tool standard. Rejected because it cannot encode agent-specific extras (Cursor MDC frontmatter, Claude plugin pointer, per-agent `--caller` value). Each agent's config is richer than a lowest-common-denominator file.

- **Extend `specscore init` with `--agent claude` flags.** Rejected because agent configuration is a separate concern from project initialization. A user may init once but configure agents repeatedly as they adopt new tools. Keeping them as separate commands follows the existing single-responsibility pattern (cf. `idea new` vs `init`).

- **Programmatically run external CLIs** (e.g., `claude plugin install`, `cursor config add`). Rejected for MVP: adds external tool dependencies, error handling for missing binaries, and testing complexity. The generated instruction files are self-contained and include manual install steps.

- **Store templates in external files rather than Go constants.** Rejected for consistency with the `init` command pattern and to avoid runtime filesystem dependencies. Templates change at the same cadence as the CLI's command surface.

## MVP Scope

Single deliverable: `specscore agent setup` with the 7-agent matrix above. Implementation:
- `internal/cli/agent.go` — command definition, agent registry, template functions
- `internal/cli/agent_test.go` — 100% statement coverage
- Feature specs at `spec/features/cli/agent/` and `spec/features/cli/agent/setup/`
- Registration in `internal/cli/root.go`

## Not Doing (and Why)

- `specscore agent status` — show which agents are configured. Useful but additive; defer to follow-on.
- `specscore agent remove` — clean up generated files. Inverse scaffolding is rarely needed; `rm` suffices.
- `--dry-run` — preview without writing. Low demand for a scaffolding command; defer.
- Agents without project-level config files (Aider, Devin, Roo, Continue, Zed, Amazon Q, Tabnine) — no standardized config mechanism to target. Add as the ecosystem matures.
- `.gitignore` generation for agent-specific directories — teams differ on whether to track these files. Leave to user preference.

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | Developers who use `specscore agent setup` will have agents that pass `--caller` on CLI invocations, improving telemetry segmentation. | Post-launch: compare the share of `caller=other` events for projects with vs without agent config files. |
| Should-be-true | The seven-agent MVP covers the majority of SpecScore users' agent usage. | Monitor the `caller` telemetry enum distribution; a high share of `caller=other` signals missing agents. |
| Should-be-true | Embedding templates as Go constants is maintainable at 7 agents. | If agent count grows past ~12, evaluate a template directory or code generation approach. |
| Might-be-true | Users will run `specscore agent setup` as part of their project bootstrap alongside `specscore init`. | Track the `agent.setup` command frequency relative to `init` in telemetry. If <10% of init users also run agent setup, consider prompting or integrating. |

## SpecScore Integration

This Idea promotes into a **two-feature structure** under a shared parent:

**Feature paths to scaffold:**

| Path | Holds |
|---|---|
| `spec/features/cli/agent/` | Parent feature for the `agent` command group. Future sibling verbs (`agent status`, `agent remove`) live here. |
| `spec/features/cli/agent/setup/` | The `agent setup` subcommand: agent registry, config-file generation, template content, `--all`/`--force` flags, exit-code contract. |

**Existing Features affected:** the root `cli` command surface (`root.go` gains `agentCommand()` in the `AddCommand` list); the `cli/telemetry/usage-telemetry` Feature (generated content includes `--caller` from its enum).

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/idea-specification*
