---
format: https://specscore.md/idea-specification
status: Implemented
---

# Idea: Agent Setup Skill Bundles

**Status:** Implemented
**Date:** 2026-06-06
**Owner:** alexander.trakhimenok
**Promotes To:** cli/agent/setup
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we let `specscore agent setup` install SpecScore's invokable skill bundles into each AI agent's skills directory — not just the single instruction file it writes today — so agents on platforms without a skill marketplace (Cursor, Codex, Copilot, Gemini) can run the same /ideate, /specify, /plan, /implement workflows that Claude Code installs from its plugin marketplace?

## Context

The shipped `ai-agent-configuration-cli` Idea gave us `specscore agent setup <agent>` (internal/cli/agent.go), which writes ONE instruction/rules file per agent (CLAUDE.md, .cursor/rules/specscore.mdc, .github/copilot-instructions.md, codex.md, GEMINI.md). That teaches an agent ABOUT SpecScore, but it does not give the agent the actual invokable skills (the specstudio set: ideate, specify, plan, implement, sidekick, verify, recap, ship, …). Claude Code installs those from a plugin marketplace. Cursor and most other agents have no marketplace, so the skills cannot be installed there at all. The user wants `agent setup` to also copy the skill bundles into each agent's skills directory (for Cursor, the repo's skills dir such as .cursor/skills/).

## Recommended Direction

Extend the existing `specscore agent setup <agent>` command — do not add a new command group or a top-level `setup` alias — so that, in addition to writing the per-agent instruction file, it copies SpecScore's skill bundle into the agent's skills directory when that agent has a known skills-dir convention. Keep the `init`/`agent setup` ergonomics: agents are named as positional args (`agent setup claude codex`), as a comma-separated list (`agent setup claude,codex`), or via `--all`; idempotent (skip existing files unless `--force`), `--project` for non-cwd roots, documented exit codes, additive/non-destructive writes. Copying is on by default (a command named `setup` should fully configure the agent); `--no-skills` restores the instruction-file-only behavior. Skills are copied as raw markdown directories with at most minimal per-agent path placement — no per-agent transpilation in the MVP. The command reports every change it made, one line per path with an explicit action verb (e.g. `added ./.cursor/skills/specscore-ideate`, `modified ./AGENTS.md`, `skipped ./CLAUDE.md (exists, use --force)`), so the user sees exactly what was written, overwritten, or left untouched. The skill source-of-truth is fetched the way `template_fetch.go` already fetches gallery templates (network-first with an embedded offline fallback), so the binary need not vendor every skill byte.

## Alternatives Considered

- **A new `specscore skills install <agent>` command group.** Cleanest separation of concerns (config vs skills) and room for `list`/`remove` siblings. Lost because configuring an agent then means running two commands, and the user explicitly chose to keep one entry point: `agent setup` should leave the agent fully ready.

- **A top-level `specscore setup <agent>` alias.** Shorter to type than `agent setup`. Lost on convention: every other group in this CLI is noun-group + verb (`idea new`, `plan list`, `feature …`, `spec lint`, `agent setup`). A verb-first top-level command is the odd one out and would give two paths to the same behavior.

- **Embed every skill's bytes in the Go binary (the `init` template-constant pattern).** Self-contained, no network. Lost because the skill set is large and evolves on a different cadence than the CLI release; the existing `template_fetch.go` already solves "fetch latest from the gallery, fall back offline" and skills should ride that same path rather than bloat the binary and go stale between releases.

- **Make skill-copying opt-in behind `--skills` instead of on-by-default.** Zero behavior change for current `agent setup` users. Lost (narrowly) because the writes are additive and idempotent, so default-on is low-risk, and a command literally named `setup` should fully configure the agent; the opt-out (`--no-skills`) covers the rare caller who wants only the instruction file.

## MVP Scope

A two-week increment: `specscore agent setup cursor,claude` (space- or comma-separated agents) writes the existing instruction file AND copies the SpecScore skill bundle into each agent's skills dir (.cursor/skills/ for Cursor, .claude/skills/ for Claude). Idempotent, additive, `--no-skills` opt-out, a per-path change report (`added`/`modified`/`skipped` one line each), 100% statement coverage on the new code. Scope the MVP to the agents with a CONFIRMED skills-dir convention; the other three named agents (Codex, Copilot, Gemini) keep instruction-file-only behavior until their convention is confirmed.

## Not Doing (and Why)

- A separate `specscore skills install` command group — the user chose to extend `agent setup`; one place to configure an agent
- A top-level `setup <agent>` alias — breaks the CLI's noun-group + verb convention (idea new, plan list, agent setup) and creates two ways to do one thing
- Per-agent skill transpilation/reformatting — MVP copies raw skill markdown; format adaptation per agent is a follow-on once we see what each agent actually parses
- Copying skills to agents with no skills-dir convention — those agents keep the existing instruction-file-only behavior until a stable convention exists
- agent skills list / status / remove subcommands — additive management verbs; defer until the copy path proves out

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | At least one non-Claude agent (Cursor) actually reads skills from a stable repo skills directory (e.g. `.cursor/skills/`), so copied files are discoverable and invokable. | Copy the bundle into a real Cursor project, confirm Cursor surfaces/invokes the skills; check Cursor docs for the documented skills path. If no stable path exists, the MVP collapses to Claude-only and the premise weakens. |
| Must-be-true | The SpecScore skill bundle can be obtained at runtime (gallery fetch with offline fallback) without depending on the Claude-only plugin marketplace. | Prototype the fetch via the `template_fetch.go` path against the gallery; confirm an embedded fallback set exists and copies cleanly when offline. |
| Should-be-true | Copying raw skill markdown (no per-agent transpilation) is enough for non-Claude agents to use the skills usefully. | Have a Cursor user run a copied `/specify` skill end-to-end; if the agent can't parse the frontmatter/triggers, transpilation moves from "Not Doing" into scope. |
| Should-be-true | Making skill-copying default-on does not surprise or break existing `agent setup` users, since writes are additive and idempotent. | Run `agent setup` twice on a configured project; confirm no overwrites without `--force` and no destructive changes; confirm `--no-skills` reproduces the old output exactly. |
| Might-be-true | Codex, Copilot, and Gemini will gain (or already have) a skills-dir convention worth targeting. | Watch each agent's docs/releases; add to the matrix when a stable path is documented. |


## SpecScore Integration

- **New Features this would create:** likely a child Feature under `spec/features/cli/agent/setup/` for the skill-bundle copy path (target-dir matrix, `--no-skills`/`--force`, fetch + offline fallback, exit-code contract), rather than a new top-level command tree.
- **Existing Features affected:** `cli/agent/setup` (gains skill-copy behavior and the `--no-skills` flag); the fetch path shares logic with whatever Feature owns `template_fetch.go` (gallery fetch + offline fallback).
- **Dependencies:** builds directly on `ai-agent-configuration-cli` (Implemented). Reuses the gallery-fetch mechanism from the new-artefact template fetcher.

## Open Questions

- Which agents besides Cursor and Claude have a documented, stable repo-level skills directory today? This sets the real MVP agent list.
- What is the authoritative source-of-truth for the SpecScore skill bundle the CLI copies — the specstudio plugin repo, the gallery, or an embedded snapshot — and how is its version pinned per CLI release?
- Should default-on skill-copying ship as a behavior change to `agent setup`, or land first behind `--skills` for one release before flipping the default?
- For Claude specifically, does copying into `.claude/skills/` duplicate or conflict with a marketplace-installed plugin, and should Claude be excluded from copying when the plugin is detected?
