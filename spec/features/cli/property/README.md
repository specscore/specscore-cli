---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: Property (CLI)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/property?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/property?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/property?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/property?op=request-change) |
>
> **AI skill:** _planned_ — `skills/property/references/*.md` references in [`ai-plugin-specscore`](https://github.com/specscore/ai-plugin-specscore) will follow shipping these verbs.

**Status:** Stable
**Source Ideas:** entity-and-property-cli-support

## Summary

The `specscore property` command group is the `specscore-cli` surface for the upstream [property Doc-Kind](https://github.com/specscore/specscore/blob/main/spec/features/property/README.md). It owns three responsibilities: **lint enforcement** (every `property-*` rule that validates `*.property.md` files), **managed-section rendering** (rewriting the `## Referenced by` section under `spec lint --fix`), and **navigation verbs** (`specscore property list`, `specscore property refs <id>`).

This Feature is the CLI's implementation contract for the meta-spec [property Feature](https://github.com/specscore/specscore/blob/main/spec/features/property/README.md) — the meta-spec defines what a property is; this Feature defines how the CLI validates, renders, and queries property files. Every `property-*` rule name listed below corresponds to one or more `REQ:` items in the upstream Feature.

## Synopsis

```
specscore property list [--project <path>] [--format <text|yaml|json>]
specscore property refs <id> [--project <path>] [--format <text|yaml|json>]
```

## Problem

Today, `specscore spec lint` is blind to `*.property.md` files — a malformed `email.property.md` (missing frontmatter, invalid `data_type`, `pattern` applied to an `integer`) reports `0 violations`. The `## Referenced by` section has no rewriter, so the cross-link from a Property back to the Entities and Features that use it must be hand-maintained. There are no `specscore property` subcommands, so authors who want to know "which entities reference `email`?" must grep markdown by hand. This Feature closes those three gaps in one MVP cycle.

## Behavior

The CLI implements three loosely-coupled surfaces backed by a single `pkg/property/` parser. Each surface is specified in its own subsection below.

### Lint enforcement

The CLI registers a family of `property-*` rules in `pkg/lint`. Each rule corresponds to one or more REQs in the upstream [property Feature](https://github.com/specscore/specscore/blob/main/spec/features/property/README.md) and is runnable via `specscore spec lint --rules <rule>`.

| Rule name | Upstream REQ | Severity | `--fix` |
|---|---|---|---|
| `property-location` | [property#req:property-location](https://github.com/specscore/specscore/blob/main/spec/features/property/README.md#req-property-location) | error | no |
| `property-slug-format` | [property#req:slug-format](https://github.com/specscore/specscore/blob/main/spec/features/property/README.md#req-slug-format) | error | no |
| `property-single-file` | [property#req:single-file](https://github.com/specscore/specscore/blob/main/spec/features/property/README.md#req-single-file) | error | no |
| `property-frontmatter-required` | [property#req:frontmatter-required](https://github.com/specscore/specscore/blob/main/spec/features/property/README.md#req-frontmatter-required) | error | no |
| `property-frontmatter-required-fields` | [property#req:frontmatter-required-fields](https://github.com/specscore/specscore/blob/main/spec/features/property/README.md#req-frontmatter-required-fields) | error | no |
| `property-id-equals-slug` | [property#req:id-equals-slug](https://github.com/specscore/specscore/blob/main/spec/features/property/README.md#req-id-equals-slug) | error | yes — rewrite `id` from filename |
| `property-data-type-values` | [property#req:data-type-values](https://github.com/specscore/specscore/blob/main/spec/features/property/README.md#req-data-type-values) | error | no |
| `property-checks-shape` | [property#req:checks-shape](https://github.com/specscore/specscore/blob/main/spec/features/property/README.md#req-checks-shape) | error (inapplicable check), warning (unknown key) | no |
| `property-title-format` | [property#req:title-format](https://github.com/specscore/specscore/blob/main/spec/features/property/README.md#req-title-format) | error | yes — rewrite from frontmatter `id` |
| `property-required-sections` | [property#req:required-sections](https://github.com/specscore/specscore/blob/main/spec/features/property/README.md#req-required-sections) | error | no |
| `property-referenced-by-managed` | [property#req:referenced-by-managed](https://github.com/specscore/specscore/blob/main/spec/features/property/README.md#req-referenced-by-managed) | error | yes — see [REQ: referenced-by-from-entities](#req-referenced-by-from-entities) |

Adherence-footer enforcement for `*.property.md` files reuses the shared `adherence-footer` rule (see [REQ: adherence-footer-target-registered](#req-adherence-footer-target-registered)); there is NO kind-specific `property-adherence-footer` rule name.

The cross-Doc-Kind rule for `ref:` resolution lives on the entity side as [`entity-ref-target-exists`](../entity/README.md#req-lint-rules-registered) — it inspects every entity's `properties[].ref:` and confirms the target `*.property.md` file exists. The property side does NOT carry a duplicate rule; a single source of truth keeps the rule registry small.

#### REQ: lint-rules-registered

Every rule name in the table above MUST be present in `pkg/lint`'s canonical `allRuleNames` map and MUST be selectable via `specscore spec lint --rules <name>`. Each rule MUST be exercised by at least one Go-level unit test that drives a fixture (positive or negative) through the linter.

#### REQ: adherence-footer-target-registered

The CLI's shared `adherence-footer` checker MUST recognise `*.property.md` consumer-path discovery and the URL `https://specscore.md/property-specification`. Missing or wrong-URL adherence footers on property files MUST be reported by the shared `adherence-footer` rule (severity follows the rest of the consumer-layer Doc-Kinds — `warn` during MVP rollout per the policy already established in `pkg/lint/adherence_footer.go`).

#### REQ: id-equals-slug-autofix

When `spec lint --fix` runs over a property file whose frontmatter `id` does not equal the filename slug, the fixer MUST rewrite `id` to match the filename. The filename is authoritative per [property#req:id-equals-slug](https://github.com/specscore/specscore/blob/main/spec/features/property/README.md#req-id-equals-slug). The rewrite MUST preserve YAML comments, key order, and surrounding formatting (via `gopkg.in/yaml.v3`'s `yaml.Node` round-trip).

#### REQ: title-format-autofix

When `spec lint --fix` runs over a property file whose `# Property: <name>` title does not match the frontmatter `id`, the fixer MUST rewrite the title line to `# Property: <id>`.

#### REQ: checks-shape-applicability

The `property-checks-shape` rule MUST validate that every key in the `checks` mapping is applicable to the declared `data_type`. The applicability matrix is defined by [property#req:checks-shape](https://github.com/specscore/specscore/blob/main/spec/features/property/README.md#req-checks-shape):

- `required`, `enum` are valid for every `data_type`.
- `min`, `max` are valid only for `integer`, `number`, `date`, `datetime`.
- `min_length`, `max_length` are valid only for `string`, `array`.
- `pattern`, `trim`, `lowercase`, `uppercase` are valid only for `string`.
- `items` is valid only for `array`.
- `json_schema` is valid only for `object`.
- `entity_ref` is valid only for `ref`.

Inapplicable check keys MUST be reported at severity `error`. Unknown check keys (anything not in the canonical vocabulary) MUST be reported at severity `warning` per the MVP forward-compatibility stance.

### Managed-section rendering

The `## Referenced by` section in every `*.property.md` file is a **managed view** over a fresh repo scan. `spec lint --fix` MUST rewrite it on every run; hand-edits inside the managed markers are a lint error.

#### REQ: referenced-by-from-entities

The `## Referenced by` managed body in every property file MUST be rewritten by `spec lint --fix` from a fresh scan of every `*.entity.md` file in the project whose frontmatter `properties[].ref:` resolves to this property. Each match MUST be rendered as `- Entity: [<entity-id>](<relative-path>)`, where `<entity-id>` is the entity's frontmatter `id` and `<relative-path>` is the path from this property file to the entity file. Each consumer entity MUST appear at most once regardless of how many `properties[].ref:` items in that entity resolve to this property (e.g., an entity with both `home_email: { ref: email.property.md }` and `work_email: { ref: email.property.md }` contributes exactly one row to `email`'s `## Referenced by`). Entries MUST be sorted alphabetically by `<entity-id>`; ties MUST be broken by `<relative-path>`.

The feature-to-property back-reference source (a forthcoming `**Consumes:**` / `**Produces:**` declaration on Feature READMEs) is **out of scope** for this Feature per the Idea's Not Doing list — until that mechanism lands, only entity → property references are wired.

#### REQ: referenced-by-no-references-fallback

When no entity references this property and no other back-reference source is wired, the managed body MUST be exactly the single line `- _No references yet._`. The managed body MUST NOT be empty.

#### REQ: managed-section-fix-is-idempotent

Running `specscore spec lint --fix` twice consecutively over a tree containing `*.property.md` files MUST yield no further changes on the second run. This is verified by `TestFixIsIdempotent` against the smoke-test fixtures.

#### REQ: fix-write-ordering

When a single `spec lint --fix` pass mutates managed sections in multiple files (e.g., adding a property and an entity that references it in the same edit set means both the property's `## Referenced by` AND the entity's `## Properties` table need rewriting), the fixer MUST compute every change before writing any file. Per-file writes MUST happen after the full repo scan completes — this is the same write-ordering contract declared in [cli/entity#req:fix-write-ordering](../entity/README.md#req-fix-write-ordering) and the two surfaces share one implementation.

### Navigation verbs

The `specscore property` command group provides two read-only navigation verbs over the property graph.

#### REQ: property-list

`specscore property list` MUST print every property id discovered under the project's `spec/features/**/*.property.md`, one per line, sorted alphabetically by id. The default output is plain text (one id per line) — matching the established pattern of `feature list` (text-default unless `--format` is supplied). With `--format yaml` or `--format json`, the output MUST be a structured list whose items carry `id` and `path` fields (path is project-relative). Exit `0` always; an empty project produces empty stdout.

#### REQ: property-refs

`specscore property refs <id>` MUST print every consumer of the property identified by `<id>`. In MVP, a "consumer" is every `*.entity.md` file whose frontmatter `properties[].ref:` resolves to `<id>`. The default output is one consumer id per line in plain text — matching the [`feature refs`](../feature/refs/README.md) exemplar (bare ids, no prefix). With `--format yaml` or `--format json`, the output MUST be a structured list under the key `consumers`. When `<id>` resolves but has no consumers, the command MUST exit `0` and print empty stdout (text) or `consumers: []` (yaml/json). The bare-id form is sufficient in MVP because the only consumer kind is `entity`; when a feature-level back-reference source lands (separate Idea), the output shape gains a kind prefix to disambiguate — non-breaking because `--format yaml` already groups under the typed `consumers` list.

Inline property definitions (those embedded directly in an entity's frontmatter without a `ref:`) have no addressable id and therefore MUST NOT appear in `property refs` output — they are not reachable from a Property file. The verb's `--help` text MUST document this.

#### REQ: verb-exit-codes

The `specscore property` verbs follow the shared CLI exit-code contract from [CLI#req:standard-exit-codes](../README.md#req-standard-exit-codes):

| Code | Condition |
|---|---|
| `0` | Success — including the empty-result case |
| `2` | Invalid `--format` value, missing `<id>` for `property refs` |
| `3` | Project root not found (no `specscore.yaml` in any ancestor) OR (`property refs` only) `<id>` does not resolve to a discovered property |
| `10` | Unexpected I/O error during discovery or parsing |

### Discovery scope

#### REQ: discovery-scope

Property discovery MUST walk `spec/features/**/*.property.md` from the project root. Files at other locations (e.g., `spec/properties/`, `docs/`) MUST NOT be discovered — they instead trigger the `property-location` rule on lint. Hidden directories (any path segment starting with `.`) and reserved underscore-prefixed directories (e.g., `_tests/`) MUST be skipped during the walk.

## Parameters

| Verb | Name | Required | Description |
|---|---|---|---|
| `property refs` | `id` | Yes | Property id — must resolve to a discovered `<id>.property.md` file under `spec/features/`. |

## Flags

| Flag | Verbs | Description |
|---|---|---|
| `--project` | all | Project root. Autodetected per [CLI#req:project-autodetect](../README.md#req-project-autodetect) when omitted. |
| `--format` | `list`, `refs` | Output format: `text` (default), `yaml`, `json`. |

## Exit codes

See [REQ: verb-exit-codes](#req-verb-exit-codes). The lint surface obeys the lint feature's contract; see [spec lint#exit-codes](../spec/lint/README.md#exit-codes).

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [Property (meta-spec)](https://github.com/specscore/specscore/blob/main/spec/features/property/README.md) | Authoritative source of the property Doc-Kind contract. Every `property-*` rule here implements one or more REQs in that spec. |
| [Entity (CLI)](../entity/README.md) | Sibling Feature. The cross-Doc-Kind `entity-ref-target-exists` rule (on the entity side) resolves `properties[].ref:` against discovered `*.property.md` files. The property side does NOT carry a duplicate rule. The `fix-write-ordering` contract is shared. |
| [spec lint](../spec/lint/README.md) | Hosts the rule registry and `--fix` flow. Adding the `property-*` rule family is the principal contract change in this cycle; see [cli/spec/lint#req:entity-and-property-rules-registered](../spec/lint/README.md#req-entity-and-property-rules-registered). |
| [adherence-footer](../../adherence-footer/README.md) | Defines the shared adherence-footer mechanism. The property URL `https://specscore.md/property-specification` is registered against the `*.property.md` consumer path so the shared `adherence-footer` rule covers property files. |
| [CLI](../README.md) | Inherits the exit-code contract, `--project` autodetection, and the `text|yaml|json` `--format` convention. |
| Source Idea: [entity-and-property-cli-support](../../../ideas/entity-and-property-cli-support.md) | The Idea that promotes to this Feature. |

## Dependencies

- cli/spec/lint
- adherence-footer
- cli/entity

## Acceptance Criteria

### AC: lint-rejects-malformed-property-file

**Requirements:** [cli/property#req:lint-rules-registered](#req-lint-rules-registered)

Given a property file at `spec/features/shared/email.property.md` that violates any `property-*` rule (e.g., missing frontmatter, invalid `data_type: blob`, `pattern` applied to `data_type: integer`), `specscore spec lint` exits `1` with at least one violation whose `Rule` field starts with `property-`.

### AC: smoke-test-fixture-passes-lint

**Requirements:** [cli/property#req:lint-rules-registered](#req-lint-rules-registered), [cli/property#req:adherence-footer-target-registered](#req-adherence-footer-target-registered)

Given the meta-spec smoke-test fixture at `spec/features/idea/email.property.md`, `specscore spec lint` exits `0` (no `property-*` violations) when run against the meta-spec repository at HEAD.

### AC: inapplicable-check-rejected

**Requirements:** [cli/property#req:checks-shape-applicability](#req-checks-shape-applicability)

Given a property file with `data_type: integer` and `checks: { pattern: "^[0-9]+$" }`, `specscore spec lint` reports a `property-checks-shape` violation at severity `error` with a message naming both the inapplicable key (`pattern`) and the declared `data_type` (`integer`).

### AC: unknown-check-key-warning

**Requirements:** [cli/property#req:checks-shape-applicability](#req-checks-shape-applicability)

Given a property file with `data_type: string` and `checks: { custom_validator: "foo" }`, `specscore spec lint --severity warning` reports a `property-checks-shape` violation at severity `warning` naming `custom_validator` as an unknown key. The default `--severity error` run does NOT surface this.

### AC: fix-renders-referenced-by-from-entities

**Requirements:** [cli/property#req:referenced-by-from-entities](#req-referenced-by-from-entities), [cli/property#req:managed-section-fix-is-idempotent](#req-managed-section-fix-is-idempotent)

Given a property file `email.property.md` and a sibling entity `user.entity.md` whose frontmatter declares `properties[].ref: ./email.property.md`, running `specscore spec lint --fix` rewrites the property's `## Referenced by` managed body to `- Entity: [user](user.entity.md)`. Running the fix twice yields no further changes.

### AC: fix-no-references-fallback

**Requirements:** [cli/property#req:referenced-by-no-references-fallback](#req-referenced-by-no-references-fallback)

Given a property that no entity references and no other back-reference source is wired, running `specscore spec lint --fix` rewrites its `## Referenced by` managed body to exactly the single line `- _No references yet._`.

### AC: id-equals-slug-autofix-rewrites-id

**Requirements:** [cli/property#req:id-equals-slug-autofix](#req-id-equals-slug-autofix)

Given a property file at `spec/features/shared/email.property.md` with frontmatter `id: emai` (does not match filename slug `email`), running `specscore spec lint --fix` rewrites the frontmatter to `id: email`, preserves comments and key order in the rest of the frontmatter, and removes the `property-id-equals-slug` violation from subsequent lint runs.

### AC: property-list-discovers-fixtures

**Requirements:** [cli/property#req:property-list](#req-property-list), [cli/property#req:discovery-scope](#req-discovery-scope)

Given a project containing `spec/features/idea/email.property.md`, running `specscore property list` exits `0` and prints `email` on stdout (text format). Running with `--format yaml` exits `0` and prints a YAML list with one item carrying `id: email` and `path: spec/features/idea/email.property.md`.

### AC: property-refs-no-consumers-exits-0

**Requirements:** [cli/property#req:property-refs](#req-property-refs)

Given a project containing one property with no entity references, running `specscore property refs <id>` exits `0` with empty stdout (text format) or `consumers: []` (yaml/json).

### AC: property-refs-unknown-id-exits-3

**Requirements:** [cli/property#req:property-refs](#req-property-refs), [cli/property#req:verb-exit-codes](#req-verb-exit-codes)

Given no property matches the supplied `<id>`, running `specscore property refs <id>` exits `3` with an explanatory stderr message; stdout remains empty.

### AC: property-refs-ignores-inline-definitions

**Requirements:** [cli/property#req:property-refs](#req-property-refs)

Given a property file `email.property.md` and an entity that has an INLINE `email` property (with `data_type` + `checks`, NOT a `ref:`), running `specscore property refs email` MUST NOT list the inline-property-bearing entity as a consumer — only `ref:`-style references count.

### AC: missing-project-exits-3

**Requirements:** [cli/property#req:verb-exit-codes](#req-verb-exit-codes)

Running any `specscore property` verb in a directory tree with no `specscore.yaml` in any ancestor exits `3` with an explanatory stderr message that names `specscore.yaml` as the project anchor.

## Open Questions

- **Severity escalation for `property-adherence-footer`** (via the shared `adherence-footer` rule) — same posture as the entity side: ships at `warn` during MVP rollout, escalates to `error` after a clean release cycle. Lock in via a follow-on Idea.
- **`property refs <id>` shape when a future feature → property link source lands** — the verb currently shows only entity consumers. When the feature-level `consumes:` / `produces:` mechanism lands, should `property refs` start showing feature consumers transparently (one merged list) or behind a `--include-features` flag? Lean: merged list, with the `entity:` / `feature:` prefix disambiguating.
- **Should `property-checks-shape` allow an `--ignore-unknown-checks` flag** for projects that extend the check vocabulary? Today unknown keys are a fixed-severity warning. Lean: defer; revisit if a real project's extension demands silencing.

---
*This document follows the https://specscore.md/feature-specification*
