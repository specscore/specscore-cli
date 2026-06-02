# Idea: Entity and Property CLI Support

**Status:** Implemented
**Date:** 2026-05-18
**Owner:** alexander.trakhimenok
**Promotes To:** cli/entity, cli/property
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we make the `specscore` CLI **aware of** and **authoritative over** the two new Document-Kind Features — [`entity`](https://github.com/specscore/specscore/blob/main/spec/features/entity/README.md) and [`property`](https://github.com/specscore/specscore/blob/main/spec/features/property/README.md) — so `spec lint` enforces their YAML-frontmatter contracts, `spec lint --fix` renders their managed body sections, and authors can navigate the entity/property graph from the CLI without hand-tracing references through Markdown?

## Context

The SpecScore meta-spec just landed two new Document-Kind Features that introduce a **typed business-data layer** on top of the existing prose-heavy Doc-Kinds: entity files (`*.entity.md`) describe business objects, property files (`*.property.md`) describe reusable single-field definitions, and the two cross-reference via a `ref:` key inside the entity's frontmatter `properties` list. The full contract — including additive-only inheritance, managed `## Properties` and `## Referenced by` sections, and a relaxed comma-separated `Consumer Path` form on the [document-types-registry](https://github.com/specscore/specscore/blob/main/spec/features/document-types-registry/README.md) — is already lint-clean and approved in the [`entity-and-property-doc-kinds` plan](https://github.com/specscore/specscore/blob/main/spec/plans/entity-and-property-doc-kinds/README.md).

The CLI is the **enforcement gap**. Today, running `specscore spec lint` against a malformed `user.entity.md` reports `0 violations` — the lint walker is blind to the new globs. The `specscore feature refs` verb does not know about entity-to-feature references. There are no `specscore entity` or `specscore property` subcommands. The new Doc-Kinds are fully documented and exemplified by [smoke-test fixtures](https://github.com/specscore/specscore/blob/main/spec/features/idea/user.entity.md) but cannot yet be **validated**, **rendered**, or **queried** by the canonical tool.

This Idea closes that gap. It is purely an implementation-side Idea against three already-approved meta-spec Features — there is no open design question on the **what**; the open work is the **how** inside `specscore-cli`.

## Recommended Direction

Ship CLI awareness of the entity and property Doc-Kinds as **three loosely-coupled work streams**, each landable independently:

**1. Lint enforcement** — `pkg/lint/property.go` and `pkg/lint/entity.go`, each a dispatch checker that runs every `property-*` or `entity-*` rule in one pass per file (mirroring the existing `pkg/lint/idea.go` pattern). Two new domain packages — `pkg/property/` and `pkg/entity/` — own discovery (glob walks under `spec/features/**`), YAML-frontmatter parsing via `gopkg.in/yaml.v3` (the version already used by `pkg/projectdef/`), and body-section parsing (titles, managed-section markers, adherence footer matching). The full rule set is enumerated in each Feature's `## Behavior` section; the lint package translates each `#### REQ:` into one rule name (e.g., `property-frontmatter-required-fields`, `entity-inherits-additive-only`).

**2. Managed-section rendering** — extends `spec lint --fix` to rewrite the body of every `*.entity.md` and `*.property.md` file on every fix pass:
- `## Properties` table in entities is rendered from the frontmatter `properties` list, in frontmatter order, with inherited properties prepended in the parent's order. Inline properties cite their `data_type`; `ref:`-referenced properties cite the linked Property file (`string *(via [email](./email.property.md))*`).
- `## Referenced by` in both entities and properties is rewritten from a fresh repo scan: every entity's `inherits:` and `properties[].ref:` populate the respective `## Referenced by` sections on parent entities and property files. Hand-edits inside the canonical `<!-- managed-by: specscore lint --fix -->` / `<!-- end-managed -->` markers are a hard error per the meta-spec contract.

**3. Navigation verbs** — new subcommands wired through `internal/cli/`:
- `specscore entity list` — flat listing of every entity id with its file path.
- `specscore entity refs <id>` — every consumer of the entity (other entities via `inherits:`; features once the `consumes:`/`produces:` mechanism lands).
- `specscore entity tree` — hierarchical view of `inherits:` chains.
- `specscore property list` — flat listing.
- `specscore property refs <id>` — every entity (and eventually feature) that references the property.

**Registry awareness.** The [document-types-registry amendment](https://github.com/specscore/specscore/blob/main/spec/features/document-types-registry/README.md#req-consumer-path-per-kind) relaxes the `Consumer Path` column to accept a comma-separated list of globs. The existing `index-entries` / `every-feature-registered` checker MUST treat that cell as the union of the listed globs. This is a small one-glob-to-list extension on the existing parser, not a redesign.

**Shared internal infrastructure.** The entity-side and property-side checks share a substantial surface — slug parsing, frontmatter delimiter detection, managed-section marker matching, adherence-footer URL matching — that overlaps with `pkg/idea/`. Where existing helpers (e.g., the slug regex, the OQ-section parser, the adherence-footer matcher) are reusable, the new packages MUST consume them rather than duplicate. A small `pkg/yamlfront/` package MAY be introduced if frontmatter parsing accumulates more than two callers; defer until the second caller exists.

**Cross-Doc-Kind lifecycle.** The `Might-be-true` assumption in the [lifecycle-verbs Idea](lifecycle-verbs-for-idea-and-feature.md#key-assumptions-to-validate) names this Idea explicitly: *"The same `pkg/lifecycle/` abstraction will later serve additional doc kinds (e.g., `proposal`, `entity`, `property` from the in-flight `entity-and-property-definitions` Idea) without refactor."* Entity and Property currently have NO author-driven status (no `**Status:**` field on instance files — entities and properties are not lifecycle-managed today). This Idea does NOT introduce lifecycle for them; it only delivers validation, rendering, and navigation. If lifecycle is ever added, the `pkg/lifecycle/` matrix gains rows for the new Doc-Kinds without restructuring.

## Alternatives Considered

**One mega-checker — `entity-and-property` combined dispatch.** A single `pkg/lint/entity_and_property.go` runs both rule sets. Rejected because the two Doc-Kinds are independently revisable: a future change to entity's inheritance semantics shouldn't touch property's rule registration, and vice versa. The existing `idea.go` / `plan_hierarchy.go` / `plan_roi.go` pattern keeps one file per Doc-Kind or concern; we match it.

**Lint-only MVP — defer `entity list/refs/tree` and `property list/refs` to a follow-on Idea.** Rejected as too thin. The navigation verbs are the user-visible payoff of "your specs are now typed"; without them, the CLI is enforcing rules but offering nothing back. Shipping all three streams in one MVP means a user who adds an entity file gets validated lint AND a working `specscore entity refs` in the same release.

**Build managed-section rendering from a template engine** (e.g., `text/template`) instead of imperative string assembly. Rejected for MVP. Imperative rendering matches the existing managed-section style in `pkg/lint/idea_index.go` and `pkg/lint/idea.go` (the `Promotes To` rewrite). A template engine adds dependency and indirection without a payoff at the current rendered-table complexity (4–6 columns, two managed sections). Revisit if a third Doc-Kind needs analogous rendering.

**Wait for the feature-level `consumes:` / `produces:` mechanism to land before shipping entity CLI support.** Rejected because the feature → entity back-reference is **deliberately deferred** by the source `entity-and-property-definitions` Idea and is not blocking. The `## Referenced by` section already has a meaningful seed (entity → entity via `inherits:`) and the meta-spec explicitly states that when no consumers exist the section renders `- _No references yet._`. The feature-link source layers on later without restructuring.

**Use `gopkg.in/yaml.v2` instead of `v3`.** Rejected — `v3` is already the in-tree choice (`pkg/projectdef/`) and supports the round-trip preservation we'll eventually need for `lint --fix` mutations to entity/property frontmatter.

## MVP Scope

One cycle. Three new feature specs under `spec/features/cli/`:
- `spec/features/cli/entity/` — `specscore entity list`, `specscore entity refs <id>`, `specscore entity tree`; declares the CLI surface for entity validation and navigation.
- `spec/features/cli/property/` — `specscore property list`, `specscore property refs <id>`; declares the CLI surface for property validation and navigation.
- Updates to `spec/features/cli/spec/lint/` (or wherever the lint feature lives in this repo) to register every new `entity-*` and `property-*` rule and to document the multi-glob `Consumer Path` resolution.

Implementation: two new domain packages (`pkg/entity/`, `pkg/property/`), two new lint checker files (`pkg/lint/entity.go`, `pkg/lint/property.go`), CLI wiring in `internal/cli/entity.go` and `internal/cli/property.go`, the managed-section rewriter inside the existing `--fix` flow, and the comma-separated `Consumer Path` extension on whichever existing checker enforces registry rows. Every `#### REQ:` in the three source Features maps to at least one rule name; every `### AC:` in those Features has at least one Go-level unit test that drives the rule end-to-end against a fixture.

The two smoke-test fixtures already in the meta-spec repo ([`spec/features/idea/email.property.md`](https://github.com/specscore/specscore/blob/main/spec/features/idea/email.property.md), [`spec/features/idea/user.entity.md`](https://github.com/specscore/specscore/blob/main/spec/features/idea/user.entity.md)) are the integration-test target: a successful `specscore spec lint` over the meta-spec repo at the head of `main` MUST report `0 violations` after this work ships, AND the two fixture files MUST be recognized and visible to `specscore entity list` / `specscore property list`.

## Not Doing (and Why)

- **Feature-level `consumes:` / `produces:` declarations** — explicitly out of scope per the source Idea's Not Doing list. The feature → entity back-reference layer waits for its own Idea. `specscore feature refs` learns to surface entity links **later**, not in this cycle.
- **Cross-repo `@import` / `@from` for entity and property references** — `ref:` resolves only inside the current repo. Supply-chain and freshness concerns belong to a future Idea.
- **Override semantics for inherited properties** — additive-only is the MVP. The CLI MUST reject any child entity that redefines a parent property; it MUST NOT attempt partial-override resolution.
- **Lifecycle verbs for entity / property** — they have no `**Status:**` field today. The `pkg/lifecycle/` package designed for Idea/Feature is forward-compatible (parameterized on kind), but this Idea does not extend the matrix.
- **Code generation from entity definitions** — datatug.io, schema-to-Go, schema-to-TS, schema-to-OpenAPI are downstream consumers of the YAML frontmatter. They depend on parseability, not on this CLI work.
- **A `specscore entity diff` / `specscore entity validate <instance>` verb** — both useful but deferred. `list`, `refs`, `tree` are the minimum that earn their weight at MVP; additional verbs follow real usage requests.
- **i18n in `singular` / `plural` / `description`** — English-only per the source Idea.
- **A standalone `pkg/yamlfront/` package** — defer until the second non-trivial caller exists. For MVP, frontmatter parsing lives inside `pkg/entity/` and `pkg/property/`.
- **Plugin (`ai-plugin-specscore`) skill updates** — deferred. Once `entity list/refs/tree` and `property list/refs` ship, the plugin SHOULD expose them as skill commands. Separate follow-on Idea or a docs-only PR.

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | The three Feature specs in the meta-spec are implementable as written — every `#### REQ:` translates to one (or a small number of) Go-level checks without forcing a spec revision. | Build a one-page mapping table (`req → check`) before writing code. Acceptance: every REQ in `entity/README.md` and `property/README.md` has a concrete `check_name` paired with a fixture; no REQ is marked "ambiguous, needs spec amendment". If 2+ REQs hit ambiguity, surface a meta-spec proposal before continuing. |
| Must-be-true | `gopkg.in/yaml.v3` round-trips entity frontmatter without losing comments, key order, or formatting when `lint --fix` rewrites a property item. | Spike: load the smoke-test `user.entity.md`, mutate one field via `yaml.Node` API, write it back, diff. Acceptance: the diff shows only the mutated field; comments and key order survive. |
| Must-be-true | Managed-section rendering produces stable, diff-friendly output on every fix pass — a clean tree at HEAD stays at HEAD after `lint --fix`. | Run `lint --fix` twice against the smoke-test fixtures; the second run MUST exit clean with no further changes. Add this as a `TestFixIsIdempotent` test. |
| Should-be-true | The full lint sweep (entity + property + existing rules) over a 200-feature consumer repo completes in under 1s on a developer laptop. | Benchmark on a synthetic 200-feature repo with 50 entity files and 100 property files. Acceptance: end-to-end `specscore spec lint` under 1s wall-clock; if exceeds, the entity/property walks are scope-narrowed to changed paths. |
| Should-be-true | The `pkg/entity/` and `pkg/property/` API surfaces converge on a shared parser interface that a hypothetical `pkg/recordset/` (future Doc-Kind) could reuse. | At design review, the two packages MUST expose `Discover(specRoot) []File`, `Parse(path) (*Doc, error)`, `Walk(specRoot, fn) error` with the same signatures. Acceptance: review-time agreement that the signatures are kind-agnostic enough. |
| Might-be-true | `specscore entity tree` is the right shape for inheritance visualization at MVP — users find it useful enough not to ask immediately for `specscore entity graph` or a Mermaid export. | Ship `tree` plain-text only; collect feedback for one cycle. Acceptance: if ≥2 independent users request graph output, prioritise it in a follow-on Idea. |
| Might-be-true | The comma-separated `Consumer Path` parser handles the union-resolution case correctly for all current registry consumers (lint walkers, `feature refs`, doc renderers). | Audit every reader of the `Consumer Path` cell before changing the parser. Acceptance: a draft PR demonstrates the new parser passes against every consumer's existing tests. |

## SpecScore Integration

- **New Features this would create:**
  - `spec/features/cli/entity/` — declares `specscore entity list`, `specscore entity refs`, `specscore entity tree`, plus the lint contract (`entity-*` rule names, severities, fix behavior).
  - `spec/features/cli/property/` — declares `specscore property list`, `specscore property refs`, plus the lint contract (`property-*` rule names, severities, fix behavior).
- **Existing Features affected:**
  - [`spec/features/cli/README.md`](../features/cli/README.md) — Contents table gains `entity/` and `property/`; brief callout that these subcommands are the canonical surface for the entity and property Doc-Kinds defined in the meta-spec.
  - [`spec/features/cli/spec/lint/`](../features/cli/spec/lint/README.md) (or equivalent) — rule registry gains every `entity-*` and `property-*` rule; the `--fix` contract documents managed-section rewrites for `## Properties` and `## Referenced by`.
  - [`spec/features/cli/feature/`](../features/cli/feature/README.md) — `feature refs` Outstanding Question gains a note that entity-link surfacing waits on the feature-level `consumes:`/`produces:` mechanism (separate Idea).
  - `pkg/entity/` (new) — `Discover`, `Parse`, `Walk`, slug helpers, inheritance-graph traversal with cycle detection.
  - `pkg/property/` (new) — `Discover`, `Parse`, `Walk`, slug helpers, reverse-reference query helpers.
  - `pkg/lint/entity.go`, `pkg/lint/entity_test.go` (new) — dispatch checker covering every `entity-*` rule.
  - `pkg/lint/property.go`, `pkg/lint/property_test.go` (new) — dispatch checker covering every `property-*` rule.
  - `pkg/lint/checkers_extended.go` or the registry checker — extended to parse comma-separated `Consumer Path` cells.
  - `internal/cli/entity.go`, `internal/cli/property.go` (new) — cobra subcommand wiring.
- **Dependencies:** None blocking. Builds on existing `spec lint --fix` infrastructure, the `gopkg.in/yaml.v3` dependency already in the tree, and the three approved meta-spec Features ([`entity`](https://github.com/specscore/specscore/blob/main/spec/features/entity/README.md), [`property`](https://github.com/specscore/specscore/blob/main/spec/features/property/README.md), [`document-types-registry`](https://github.com/specscore/specscore/blob/main/spec/features/document-types-registry/README.md)). The smoke-test fixtures committed to the meta-spec repo are the canonical integration-test target.

## Open Questions

- **`Status` field on entity / property instances** — entities and properties have no lifecycle today (no `**Status:**` in frontmatter). Should the CLI silently accept that absence, or should an explicit `status: stable` be added to the frontmatter contract in a future Idea? Lean: silent for MVP; revisit if users adopt a stable-vs-draft entity workflow.
- **Where does the spec-level `pkg/yamlfront/` extraction live if a third caller arrives?** Inline parsing in `pkg/entity/` and `pkg/property/` is the MVP, but if `pkg/projectdef/` and the new packages converge on similar code, factoring becomes attractive. Decide at the second-call-site point.
- **`specscore entity refs <id>` output format** — table, JSON, plain list? Existing verbs (`feature refs`, `idea refs` via `lint`) are plain-list. Lean: plain-list with `--json` flag if a downstream consumer requests it.
- **Managed-section idempotency across `os.WriteFile` ordering** — when both an entity and a referenced property need their `## Referenced by` rewritten in the same `--fix` pass, the write order matters for predictable diffs. Lean: scan-then-write all changes in one pass (compute → apply), not iterative.
- **Behavior when an entity's `inherits:` target is a Property file (wrong target kind)** — explicit lint error, or silently treated as broken-ref? Lean: explicit `entity-inherits-target-kind` rule, error severity.
- **`specscore property refs <id>` semantics when the property is inlined inside an entity** — inline definitions have no addressable id, so they don't appear in `property refs`. Documented or surfaced as a warning? Lean: documented in the verb's help text.

---
*This document follows the https://specscore.md/idea-specification*
