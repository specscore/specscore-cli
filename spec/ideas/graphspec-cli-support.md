---
format: https://specscore.md/idea-specification
status: Specifying
---

# Idea: GraphSpec CLI Support

**Status:** Specifying
**Date:** 2026-07-08
**Owner:** codex
**Promotes To:** cli/graph, cli/graph/ai-assistance, cli/graph/documentation, cli/graph/navigation, cli/graph/new, cli/graph/validation
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we make GraphSpec a first-class SpecScore CLI capability for authoring, validation, navigation, documentation, and AI-assisted review while keeping GraphSpec implementation-independent?

## Context

GraphSpec now exists as a bootstrap language in the SpecScore family. The CLI should consume GraphSpec and provide tooling around it without redefining the language or disrupting existing idea, feature, entity, property, and spec command groups.

## Recommended Direction

Add a specscore graph command group that follows existing CLI resource/action conventions. Specify authoring, validation, navigation, documentation, and AI-assistance subcommands as CLI contracts only; defer production implementation.

## Alternatives Considered

- Extend the existing `specscore entity` and `specscore property` groups. Rejected because those groups already have stable contracts for legacy SpecScore Doc-Kinds under `spec/features/**/*.entity.md` and `spec/features/**/*.property.md`; GraphSpec entities need a separate surface.
- Put GraphSpec validation only under `specscore spec lint`. Rejected for the first implementation because GraphSpec is still bootstrap-stage and experimental `spec/graph/` trees should not be globally gated until the language stabilizes.
- Name the command group `graphspec`. Kept as an alternative because it is precise, but `graph` is shorter, matches `spec/graph/`, and follows existing singular command naming.
- Build AI review commands first. Rejected because authoring, validation, navigation, and documentation surfaces need stable contracts before AI wrappers can rely on them.

## MVP Scope

Specify specscore graph, graph new, graph lint/validate/doctor/check-refs, graph list/search/index/stats/tree, graph render/export, and future graph review/explain/suggest/scaffold/summarize commands. Document interactions with existing CLI groups and open architecture questions.

## Not Doing (and Why)

- Implement production code — this task is specification-only.
- Redesign generic CLI architecture — reuse existing CLI command, output, exit-code, and project autodetection conventions.
- Define GraphSpec itself — GraphSpec language semantics remain owned by the GraphSpec specification.

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | A root `specscore graph` command can cover GraphSpec without colliding with legacy `entity` and `property` command groups. | Specify command boundaries and verify existing entity/property contracts remain unchanged. |
| Must-be-true | GraphSpec roots may be distributed across repos using SpecScore's unified cross-repo linking model. | Include local and cross-repo discovery requirements in the graph command specs before implementation. |
| Should-be-true | GraphSpec validation should start as `graph lint` before being enabled by default in `spec lint`. | Keep GraphSpec lint integration gated and revisit after GraphSpec schemas stabilize. |
| Might-be-true | AI-oriented graph review commands can prepare useful packages without invoking models directly. | Specify deterministic `graph review` and `graph explain` output before deciding model integration. |


## SpecScore Integration

- **New Features this would create:** cli/graph, cli/graph/new, cli/graph/validation, cli/graph/navigation, cli/graph/documentation, cli/graph/ai-assistance
- **Existing Features affected:** cli, cli/spec/lint, cli/entity, cli/property
- **Dependencies:** GraphSpec language specification, SpecScore cross-repo linking model

## Open Questions

- Should `graph` remain the long-term command name, or should `graphspec` become the explicit command before implementation?
- When should `graph-*` validation rules become part of default `specscore spec lint`?
- How much cross-repo traversal should be enabled in the first implementation?
