# Plan: Agent Setup Skill Bundles

**Status:** Implemented
**Source Feature:** cli/agent/setup
**Date:** 2026-06-06
**Owner:** alexander.trakhimenok
**Supersedes:** —

## Summary

Implements the skill-bundle and comma-separated-agent increment added to the `cli/agent/setup` Feature by the `agent-setup-skill-bundles` Idea. The existing instruction-file behavior is already shipped and Stable, so this Plan covers only the eight new acceptance criteria; the twelve pre-existing ACs are deferred as already-implemented.

## Approach

Built bottom-up in `internal/cli/agent.go`: input parsing first (comma-split), then the skills-directory registry, then the bundle source (GitHub marketplace download, fail-clear when offline), then the copy engine that consumes both, then the `--no-skills` gate over that engine, and finally the per-path change report that spans every write. Each task is independently testable against its ACs and ordered so later tasks depend only on earlier ones.

## Tasks

### Task 1: Parse comma-separated agent lists

**Verifies:** cli/agent/setup#ac:comma-separated-agents

Split each positional agent argument on commas, trim whitespace, and drop empty fields before the existing per-name validation, so `claude,codex` is equivalent to `claude codex`. Pure input-normalization step ahead of all agent resolution.

### Task 2: Add the skills-directory registry to the agent matrix

**Verifies:** cli/agent/setup#ac:non-skilldir-agent-instruction-only

Extend `agentDef` with an optional skills-directory field and populate it only for `claude` (`.claude/skills/`) and `cursor` (`.cursor/skills/`); all other agents resolve to no skills directory and remain instruction-file only. This registry is the single source of truth the copy engine consults.

### Task 3: Download skill bundles from the GitHub marketplace registry

**Verifies:** cli/agent/setup#ac:skill-source-offline-fails

Download skill bundles in three steps — marketplace manifest (`specscore/ai-marketplace`), then each plugin repo's manifest, then the enumerated skill files — with the base location overridable via an environment variable for tests. There is no embedded fallback: when GitHub is unreachable or any manifest/skill fetch fails, write the instruction files, report a clear error naming the source, exit `10`, and leave no partial skills directory.

### Task 4: Copy skill bundles into the agent skills directory

**Verifies:** cli/agent/setup#ac:skill-copy-cursor-default, cli/agent/setup#ac:skill-copy-claude-always, cli/agent/setup#ac:skill-copy-skips-existing

For each requested agent that has a skills directory, copy the sourced bundle as raw markdown into per-skill subdirectories, creating parent directories as needed. Copying is on by default and treats Claude like any other skills-directory agent; existing skill files are preserved byte-identical and skipped unless `--force` overwrites them.

### Task 5: Gate copying behind the `--no-skills` flag

**Verifies:** cli/agent/setup#ac:no-skills-suppresses-copy

Add the `--no-skills` boolean flag that, when set, suppresses all skill copying so the command writes only instruction files — reproducing the pre-skill-copy output exactly. Default (flag absent) leaves copying on.

### Task 6: Emit the per-path change report

**Verifies:** cli/agent/setup#ac:change-report-output

Report every touched path on its own line, prefixed `added`, `modified` (overwrite under `--force`), or `skipped` (existing path, message mentions `--force`). The report spans both instruction files and copied skill files, and prints even when every path is skipped.

### Task 7: Make the marketplace ref overridable via --ref / SPECSCORE_MARKETPLACE_REF

**Verifies:** cli/agent/setup#ac:ref-override-precedence

Replace the hardcoded `main` ref with a resolved value: the `--ref` flag wins, else the `SPECSCORE_MARKETPLACE_REF` env var, else `main`. Thread the resolved ref through the marketplace-manifest and plugin-tarball downloads so callers can pin a reproducible tag or commit.

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
- Which marketplace ref (branch/tag/commit) does the CLI pin when downloading manifests and skills, and should it be configurable?

---
*This document follows the https://specscore.md/plan-specification*
