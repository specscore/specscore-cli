# Plan: Agent Setup Skill Bundles

**Status:** Approved
**Source Feature:** cli/agent/setup
**Date:** 2026-06-06
**Owner:** alexander.trakhimenok
**Supersedes:** —

## Summary

Implements the skill-bundle and comma-separated-agent increment added to the `cli/agent/setup` Feature by the `agent-setup-skill-bundles` Idea. The existing instruction-file behavior is already shipped and Stable, so this Plan covers only the eight new acceptance criteria; the twelve pre-existing ACs are deferred as already-implemented.

## Approach

Built bottom-up in `internal/cli/agent.go`: input parsing first (comma-split), then the skills-directory registry, then the bundle source (gallery fetch + embedded fallback), then the copy engine that consumes both, then the `--no-skills` gate over that engine, and finally the per-path change report that spans every write. Each task is independently testable against its ACs and ordered so later tasks depend only on earlier ones.

## Tasks

### Task 1: Parse comma-separated agent lists

**Verifies:** cli/agent/setup#ac:comma-separated-agents

Split each positional agent argument on commas, trim whitespace, and drop empty fields before the existing per-name validation, so `claude,codex` is equivalent to `claude codex`. Pure input-normalization step ahead of all agent resolution.

### Task 2: Add the skills-directory registry to the agent matrix

**Verifies:** cli/agent/setup#ac:non-skilldir-agent-instruction-only

Extend `agentDef` with an optional skills-directory field and populate it only for `claude` (`.claude/skills/`) and `cursor` (`.cursor/skills/`); all other agents resolve to no skills directory and remain instruction-file only. This registry is the single source of truth the copy engine consults.

### Task 3: Source skill bundles via gallery fetch with embedded fallback

**Verifies:** cli/agent/setup#ac:skill-source-offline-fallback

Obtain the skill bundle the same way `template_fetch.go` obtains new-artefact templates: fetch from the SpecScore gallery when reachable, fall back to a binary-embedded snapshot when offline. Surface exit `10` when neither source can supply the bundle, without writing a partial skills directory.

### Task 4: Copy skill bundles into the agent skills directory

**Verifies:** cli/agent/setup#ac:skill-copy-cursor-default, cli/agent/setup#ac:skill-copy-claude-always, cli/agent/setup#ac:skill-copy-skips-existing

For each requested agent that has a skills directory, copy the sourced bundle as raw markdown into per-skill subdirectories, creating parent directories as needed. Copying is on by default and treats Claude like any other skills-directory agent; existing skill files are preserved byte-identical and skipped unless `--force` overwrites them.

### Task 5: Gate copying behind the `--no-skills` flag

**Verifies:** cli/agent/setup#ac:no-skills-suppresses-copy

Add the `--no-skills` boolean flag that, when set, suppresses all skill copying so the command writes only instruction files — reproducing the pre-skill-copy output exactly. Default (flag absent) leaves copying on.

### Task 6: Emit the per-path change report

**Verifies:** cli/agent/setup#ac:change-report-output

Report every touched path on its own line, prefixed `added`, `modified` (overwrite under `--force`), or `skipped` (existing path, message mentions `--force`). The report spans both instruction files and copied skill files, and prints even when every path is skipped.

## Deferred AC Coverage

- cli/agent/setup#ac:single-agent-claude — Already implemented and Stable in the shipped instruction-file behavior; this Plan scopes only the skill-bundle/CSV increment.
- cli/agent/setup#ac:multiple-agents — Already implemented and Stable in the shipped instruction-file behavior; out of scope for this increment.
- cli/agent/setup#ac:all-flag — Already implemented and Stable; the `--all` selection path is unchanged by this increment.
- cli/agent/setup#ac:unknown-agent — Already implemented and Stable; name validation is reused, not modified, by Task 1.
- cli/agent/setup#ac:no-args-no-all — Already implemented and Stable; argument-presence checks are unchanged.
- cli/agent/setup#ac:all-and-positional-conflict — Already implemented and Stable; mutual-exclusivity logic is unchanged.
- cli/agent/setup#ac:skips-existing — Already implemented and Stable for instruction files; Task 4 reuses the same idempotency rule for skill files.
- cli/agent/setup#ac:force-overwrites — Already implemented and Stable for instruction files; Task 4 extends the same `--force` semantics to skill files.
- cli/agent/setup#ac:not-specscore-repo — Already implemented and Stable; project-root resolution is unchanged.
- cli/agent/setup#ac:spec-features-but-no-yaml — Already implemented and Stable; project-root resolution is unchanged.
- cli/agent/setup#ac:cursor-mdc-format — Already implemented and Stable; the Cursor instruction file format is unchanged by skill copying.
- cli/agent/setup#ac:fallback-title — Already implemented and Stable; project-title fallback is unchanged.

## Open Questions

- Which agents beyond Claude and Cursor gain a stable skills directory (would extend Task 2's registry)?
- Should the embedded fallback snapshot be version-pinned per CLI release, and where does the canonical skill source-of-truth live?

---
*This document follows the https://specscore.md/plan-specification*
