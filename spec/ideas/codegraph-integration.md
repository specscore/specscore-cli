---
format: https://specscore.md/idea-specification
status: Draft
---

# Idea: codegraph integration: trace, --where, and drift in the specscore CLI

**Status:** Draft
**Date:** 2026-06-10
**Owner:** trakhimenok
**Promotes To:** —
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might the specscore CLI consume a codegraph index to answer spec-to-code questions (trace, entry points, drift) without owning an indexer?

## Context

CLI-side companion to specscore/specscore spec/ideas/spec-code-linking-via-codegraph.md (the concept: REQ/AC bindings, orphans report, drift flags, and agent-first one-call orientation). codegraph (github.com/colbymchenry/codegraph - MIT, tree-sitter-based, ~46k stars) already indexes symbols/edges/files into a local .codegraph/ SQLite store and is the hard dependency of cover100, so SpecScore-family repos increasingly have an index sitting there. This idea decides how specscore-cli talks to it and what new verbs ship.

## Recommended Direction

Coupling REVISED (2026-06-10): with the Go port of codegraph planned as github.com/specscore/codegrapher (library-first design), specscore-cli EMBEDS codegrapher as a Go library - typed structs, no subprocess. Shell-out to the CLI remains the stable contract for NON-Go consumers (cover100 agents), and reading the index SQLite schema directly stays forbidden for everyone - the library API and CLI surface are the two sanctioned interfaces. specstudio-skills consume this indirectly: their SKILL.md instructions drive specscore CLI verbs, so trace/--where capabilities reach every skill platform (Claude Code, Gemini, Codex) with zero skill-side changes. Link-marker capture (comments like '// implements: <feature>#req:<slug>' and test-name conventions) is specscore-cli's OWN lightweight scan; codegraph resolves the symbols those markers attach to and keeps positions current. New verbs: 'specscore trace req:<slug>' (implementing symbols + verifying tests, current file:line), 'specscore trace --orphans' (REQs with no bindings; bound code whose spec is gone), 'specscore feature <slug> --where' (one-call agent orientation: summary + REQs + bindings + drift flags, machine-readable). Drift = linked symbol's content changed (git) since the spec file's last revision touching that REQ. GRACEFUL DEGRADATION is mandatory: unlike cover100, specscore-cli must keep working without codegraph - trace verbs report 'codegraph not installed' with install guidance; nothing else breaks.

## Alternatives Considered

<!-- 2–3 directions that lost, and why each lost. -->

## MVP Scope

Spike on this repo itself (dogfood): define the link-marker convention, implement 'specscore trace req:<slug>' and 'specscore feature <slug> --where' for Go, produce one honest orphans report for specscore-cli's own spec tree. Drift detection second iteration.

## Not Doing (and Why)

- Owning an indexer or parsing source ourselves beyond link-marker scanning - codegraph is the index
- Reading codegraph's SQLite schema directly - unversioned internals; CLI surface only
- Hard codegraph requirement - trace features degrade gracefully when absent
- Lint gates on orphans - reports first; gates only after the convention proves itself
- Cross-repo tracing - single repo first

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | placeholder dealbreaker assumption | describe how to validate |
| Should-be-true | … | … |
| Might-be-true | … | … |


## SpecScore Integration

- **New Features this would create:** TBD at design time
- **Existing Features affected:** none
- **Dependencies:** none

## Open Questions

None at this time.
