# Idea: Canonical Lint-Rule Catalog

**Status:** Approved
**Date:** 2026-06-03
**Owner:** alexander.trakhimenok
**Promotes To:** —
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we give SpecScore users and maintainers one authoritative, always-accurate catalog of every lint rule and what it enforces, without a hand-maintained list that drifts from the 113-rule code registry?

## Context

SpecScore ships ~113 lint rules across families (idea-*, D-*, DI-*, I-*, plan/feature/misc). The de-facto source of truth is the `allRuleNames` map in `pkg/lint/lint.go` plus per-family ID slices (e.g. `decisionRuleIDs`, `decisionsIndexRuleIDs`). Human documentation is fragmented: idea rules are partially tabled in `spec/features/cli/spec/lint/README.md`, issue rules in `issue-rules/README.md`, and decision rules (D-*/DI-*) are documented nowhere. Newly added rules (three landed this session in #31/#32) get registered in code with no doc home. Users running `specscore spec lint` have no single place to discover what a rule enforces or what to `--ignore`. Related: the queued seed `linter-to-output-supported-prefixes-for-related-ideas` asks for a discoverable surface for supported values — the same discoverability theme.

## Recommended Direction

Make the code registry the single source of truth and derive everything else from it. (1) Enrich the rule registry so each rule carries a one-line description and family/severity metadata (replace the bare `map[string]bool` with a structured registry). (2) Expose it at runtime via a `specscore rules` command (and/or `spec lint --list-rules`) so CLI users can discover and filter rules. (3) Generate a committed markdown catalog from the registry. (4) Guarantee zero drift with a meta-lint rule (dogfooding SpecScore on itself) that fails CI if the generated catalog and the registry diverge. This serves both audiences — runtime discovery for end users, a maintained doc + CI enforcement for contributors — and makes silent drift impossible.

## Alternatives Considered

- **Hand-maintained markdown table.** Simplest to start, but a static table of 113 rules drifts from the registry the moment a rule is added (it already happened three times this session). A catalog that silently lies is worse than none. Rejected.
- **CLI command only, no committed doc.** A `specscore rules` command gives end users runtime discovery, but leaves no reviewable artifact for maintainers and no CI gate — failing the "both audiences" and "zero-drift" goals. Loses to the recommended direction, which adds a generated, drift-guarded doc on top of the command.
- **Doc + drift-guard without a structured registry.** Keep descriptions in markdown and reconcile against the bare `allRuleNames` keys. This guards existence but not meaning (descriptions still live only in prose, not in code), so it doesn't establish a single source of truth. Rejected in favor of enriching the registry itself.

## MVP Scope

A two-to-three-week slice: structured registry with descriptions for all current rules, a `specscore rules` command that prints them (filterable by family), a generated markdown catalog committed under spec/, and a drift-guard check (lint rule or `rules --check`) wired into CI. Done = adding a rule without updating its description fails CI.

## Not Doing (and Why)

- Hand-maintained rule table — drifts immediately at 113 rules; the whole point is to derive from code
- Per-rule long-form docs/rationale pages — MVP is one-line descriptions; deep docs can follow
- Rule deprecation/versioning lifecycle — out of scope; this is a catalog, not a rule-lifecycle system
- Changing any rule's behavior or severity — purely additive: metadata, surfacing, and drift-guard only

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | The complete rule set can be reconciled into ONE structured in-code registry without losing or duplicating IDs (today they live in `allRuleNames` plus per-family slices like `decisionRuleIDs`). | Audit that every rule ID emitted by a checker appears in the unified registry and vice versa; a partial coverage test already exists to extend. |
| Should-be-true | A single one-line description per rule is enough for both runtime discovery and the maintainer doc (no long-form pages needed for MVP). | Draft one-liners for a dozen rules across families; check they make a violation actionable without further reading. |
| Might-be-true | One `specscore rules` command plus a generated doc covers both audiences without per-family doc pages or a web surface. | Prototype the command + generated catalog; get maintainer feedback before investing in richer surfaces. |


## SpecScore Integration

- **New Features this would create:** likely two — (a) a structured rule registry + `specscore rules` command (runtime discovery/filtering); (b) a generated rule-catalog doc + meta-lint drift-guard (maintainer doc + CI enforcement). Split decided at spec time.
- **Existing Features affected:** `cli/spec/lint` (registry refactor from `map[string]bool` to structured metadata); the partial idea-rules table in `spec/features/cli/spec/lint/README.md` would be superseded by the generated catalog.
- **Dependencies:** synergy with the queued seed `linter-to-output-supported-prefixes-for-related-ideas` (same discoverability theme — could be folded in or kept adjacent).

## Open Questions

- Should the generated catalog live under `spec/` (dogfooded, lint-checked) or `docs/`?
- Is the drift-guard a self-referential lint rule, or a `specscore rules --check` step invoked in CI?
- One surface or two: keep both `specscore rules` and `spec lint --list-rules`, or pick one?
- What happens to the existing partial idea-rules table in the lint README — migrate into the generated catalog and remove, or leave during transition?

---
*This document follows the https://specscore.md/idea-specification*
