---
format: https://specscore.md/idea-specification
status: Specified
---

# Idea: ModelSpec CLI Validation

**Status:** Specified
**Date:** 2026-07-08
**Owner:** codex
**Promotes To:** cli/spec/lint
**Supersedes:** —
**Related Ideas:** alternative_to:graphspec-cli-support

## Problem Statement

How might the SpecScore CLI validate ModelSpec as a first-class independent
specification while preserving current lint conventions and avoiding a new command
architecture?

## Context

ModelSpec is an independent specification language for application data models. It is
maintained in the SpecScore GitHub organization, but SpecScore does not own ModelSpec
semantics.

The CLI should support ModelSpec linting, structural validation, and semantic checking
without making ModelSpec a sub-language of GraphSpec or disrupting existing
`specscore spec lint` workflows.

## Recommended Direction

Reuse existing CLI conventions. The current implementation surface remains
`specscore spec lint`. Shorter root-level aliases and a ModelSpec-specific target can
be added later if they fit CLI naming rules and compatibility requirements.

Examples under consideration:

```text
specscore spec lint
specscore lint
specscore lint modelspec
specscore validate
```

ModelSpec rules should use the existing lint selection machinery and remain gated
until ModelSpec publishes stable validation contracts.

## Alternatives Considered

### Add a new `specscore modelspec` command group immediately

Rejected for the first pass. It risks creating a parallel command architecture before
the generic lint and validate aliases are settled.

### Put ModelSpec under `specscore graph`

Rejected. ModelSpec is not a GraphSpec sub-language.

### Make OpenVaultDB depend on SpecScore validation

Rejected. OpenVaultDB consumes ModelSpec directly; SpecScore is a validator, not the
owner of ModelSpec semantics.

## MVP Scope

Document the validation boundary first. When implementation starts, register
ModelSpec rules through the existing lint selection machinery and keep them gated
until ModelSpec publishes stable validation contracts.

## Not Doing (and Why)

- Defining ModelSpec semantics — ModelSpec owns its language.
- Implementing OpenVaultDB schema loading — OpenVaultDB consumes ModelSpec directly.
- Changing GraphSpec commands — ModelSpec is independent from GraphSpec.
- Adding a new CLI architecture — reuse existing lint and validate conventions.

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | ModelSpec can expose stable validation rules independent of SpecScore. | Reference the ModelSpec repo as the semantic source of truth. |
| Must-be-true | Existing lint rule-selection machinery can host ModelSpec rules. | Specify registry compatibility before implementation. |
| Should-be-true | Root-level `specscore lint` and `specscore validate` aliases can coexist with `specscore spec lint`. | Resolve through the CLI command-naming open question. |

## SpecScore Integration

- **New Features this would create:** none immediately; this updates `cli/spec/lint`.
- **Existing Features affected:** cli/spec/lint, cli/spec, cli.
- **Dependencies:** ModelSpec language specification and stable validation contracts.

## Open Questions

- Should ModelSpec validation be enabled by default in repo-wide lint or require an explicit target?
- Should the first explicit target be `specscore lint modelspec` or `specscore spec lint --rules modelspec-*`?
- Which ModelSpec serialization formats should be validated first?

---
*This document follows the https://specscore.md/idea-specification*
