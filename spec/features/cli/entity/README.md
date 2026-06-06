---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: Entity (CLI)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/entity?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/entity?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/entity?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/entity?op=request-change) |
>
> **AI skill:** _planned_ — `skills/entity/references/*.md` references in [`ai-plugin-specscore`](https://github.com/specscore/ai-plugin-specscore) will follow shipping these verbs.

**Status:** Stable
**Source Ideas:** entity-and-property-cli-support

## Summary

The `specscore entity` command group is the `specscore-cli` surface for the upstream [entity Doc-Kind](https://github.com/specscore/specscore/blob/main/spec/features/entity/README.md). It owns three responsibilities: **lint enforcement** (every `entity-*` rule that validates `*.entity.md` files), **managed-section rendering** (rewriting the `## Properties` table and `## Referenced by` section under `spec lint --fix`), and **navigation verbs** (`specscore entity list`, `specscore entity refs <id>`, `specscore entity tree`).

This Feature is the CLI's implementation contract for the meta-spec [entity Feature](https://github.com/specscore/specscore/blob/main/spec/features/entity/README.md) — the meta-spec defines what an entity is; this Feature defines how the CLI validates, renders, and queries entity files. Every `entity-*` rule name listed below corresponds to one or more `REQ:` items in the upstream Feature.

## Synopsis

```
specscore entity list [--project <path>] [--format <text|yaml|json>]
specscore entity refs <id> [--project <path>] [--format <text|yaml|json>]
specscore entity tree [--project <path>]
```

## Problem

Today, `specscore spec lint` is blind to `*.entity.md` files — a malformed `user.entity.md` (missing frontmatter, broken `inherits:`, duplicate property names) reports `0 violations`. The `## Properties` table and `## Referenced by` section have no rewriter. There are no `specscore entity` subcommands, so authors who want to know "which entities inherit from `User`?" have to grep markdown by hand. This Feature closes those three gaps in one MVP cycle.

## Behavior

The CLI implements three loosely-coupled surfaces backed by a single `pkg/entity/` parser. Each surface is specified in its own subsection below.

### Lint enforcement

The CLI registers a family of `entity-*` rules in `pkg/lint`. Each rule corresponds to one or more REQs in the upstream [entity Feature](https://github.com/specscore/specscore/blob/main/spec/features/entity/README.md) and is runnable via `specscore spec lint --rules <rule>`.

| Rule name | Upstream REQ | Severity | `--fix` |
|---|---|---|---|
| `entity-location` | [entity#req:entity-location](https://github.com/specscore/specscore/blob/main/spec/features/entity/README.md#req-entity-location) | error | no |
| `entity-slug-format` | [entity#req:slug-format](https://github.com/specscore/specscore/blob/main/spec/features/entity/README.md#req-slug-format) | error | no |
| `entity-single-file` | [entity#req:single-file](https://github.com/specscore/specscore/blob/main/spec/features/entity/README.md#req-single-file) | error | no |
| `entity-frontmatter-required` | [entity#req:frontmatter-required](https://github.com/specscore/specscore/blob/main/spec/features/entity/README.md#req-frontmatter-required) | error | no |
| `entity-frontmatter-required-fields` | [entity#req:frontmatter-required-fields](https://github.com/specscore/specscore/blob/main/spec/features/entity/README.md#req-frontmatter-required-fields) | error | no |
| `entity-id-equals-slug` | [entity#req:id-equals-slug](https://github.com/specscore/specscore/blob/main/spec/features/entity/README.md#req-id-equals-slug) | error | yes — rewrite `id` from filename |
| `entity-properties-list-shape` | [entity#req:properties-list-shape](https://github.com/specscore/specscore/blob/main/spec/features/entity/README.md#req-properties-list-shape) | error | no |
| `entity-ref-target-exists` | [entity#req:ref-target-exists](https://github.com/specscore/specscore/blob/main/spec/features/entity/README.md#req-ref-target-exists) | error | no |
| `entity-inherits-additive-only` | [entity#req:inherits-additive-only](https://github.com/specscore/specscore/blob/main/spec/features/entity/README.md#req-inherits-additive-only) | error | no |
| `entity-inherits-target-exists` | [entity#req:inherits-target-exists](https://github.com/specscore/specscore/blob/main/spec/features/entity/README.md#req-inherits-target-exists) | error | no |
| `entity-inherits-acyclic` | [entity#req:inherits-acyclic](https://github.com/specscore/specscore/blob/main/spec/features/entity/README.md#req-inherits-acyclic) | error | no |
| `entity-title-format` | [entity#req:title-format](https://github.com/specscore/specscore/blob/main/spec/features/entity/README.md#req-title-format) | error | yes — rewrite from frontmatter `singular` |
| `entity-required-sections` | [entity#req:required-sections](https://github.com/specscore/specscore/blob/main/spec/features/entity/README.md#req-required-sections) | error | no |
| `entity-properties-table-managed` | [entity#req:properties-table-managed](https://github.com/specscore/specscore/blob/main/spec/features/entity/README.md#req-properties-table-managed) | error | yes — see [REQ: properties-table-rendered](#req-properties-table-rendered) |
| `entity-referenced-by-managed` | [entity#req:referenced-by-managed](https://github.com/specscore/specscore/blob/main/spec/features/entity/README.md#req-referenced-by-managed) | error | yes — see [REQ: referenced-by-from-inheritance](#req-referenced-by-from-inheritance) |

Adherence-footer enforcement for `*.entity.md` files reuses the shared `adherence-footer` rule (see [REQ: adherence-footer-target-registered](#req-adherence-footer-target-registered)); it does NOT get a kind-specific `entity-adherence-footer` rule name.

#### REQ: lint-rules-registered

Every rule name in the table above MUST be present in `pkg/lint`'s canonical `allRuleNames` map and MUST be selectable via `specscore spec lint --rules <name>`. Each rule MUST be exercised by at least one Go-level unit test that drives a fixture (positive or negative) through the linter.

#### REQ: adherence-footer-target-registered

The CLI's shared `adherence-footer` checker MUST recognise `*.entity.md` consumer-path discovery and the URL `https://specscore.md/entity-specification`. Missing or wrong-URL adherence footers on entity files MUST be reported by the shared `adherence-footer` rule (severity follows the rest of the consumer-layer Doc-Kinds — `warn` during MVP rollout per the policy already established in `pkg/lint/adherence_footer.go`).

#### REQ: id-equals-slug-autofix

When `spec lint --fix` runs over an entity file whose frontmatter `id` does not equal the filename slug, the fixer MUST rewrite `id` to match the filename. The filename is authoritative — renaming a file is more visible than editing frontmatter, per [entity#req:id-equals-slug](https://github.com/specscore/specscore/blob/main/spec/features/entity/README.md#req-id-equals-slug). The rewrite MUST preserve YAML comments, key order, and surrounding formatting (via `gopkg.in/yaml.v3`'s `yaml.Node` round-trip).

#### REQ: title-format-autofix

When `spec lint --fix` runs over an entity file whose `# Entity: <name>` title does not match the frontmatter `singular`, the fixer MUST rewrite the title line to `# Entity: <singular>`.

### Managed-section rendering

The `## Properties` table and the `## Referenced by` section in every `*.entity.md` file are **managed views** over the frontmatter and a fresh repo scan respectively. `spec lint --fix` MUST rewrite both sections on every run; hand-edits inside the managed markers are a lint error.

#### REQ: properties-table-rendered

For each entity file, the body between `<!-- managed-by: specscore lint --fix -->` and `<!-- end-managed -->` markers inside the `## Properties` section MUST be rewritten by `spec lint --fix` to a markdown pipe-table with the columns `Name`, `Type`, `Required`, `Description`, in that order.

Rows MUST appear in the order they appear in the frontmatter `properties` list, with inherited properties (when `inherits:` is set) prepended in the parent's order. For each row:

- `Name` is the property `name`, rendered as backtick-quoted code (`` `email` ``).
- `Type` is the resolved `data_type`. For inline items, this is the inline `data_type` literally. For `ref:` items, the cell is rendered as `<data_type> *(via [<id>](<relative-path>))*`, where `<data_type>` is read from the referenced `*.property.md` file's frontmatter, `<id>` is the property's frontmatter `id`, and `<relative-path>` is the path from the file being rendered to the referenced property file. Relative paths MUST always be computed against the file being rendered — including for inherited rows, where the path resolves from the child entity's location even though the `ref:` was declared in the parent's frontmatter.
- `Required` is `yes` when `checks.required` is true (or inherited as true from the referenced property's `checks`), `no` otherwise.
- `Description` is the property `description` if present, otherwise `—`.

The fixer MUST be deterministic — running it twice on the same input produces the same output.

#### REQ: properties-table-hand-edit-error

When the managed body inside the `## Properties` markers does not match the canonical rendering for the file's frontmatter, the CLI MUST report this as an `entity-properties-table-managed` lint error. `spec lint --fix` MUST repair the violation by overwriting the managed body. The repair MUST NOT preserve hand-edited content — managed sections are not a second source of truth.

#### REQ: referenced-by-from-inheritance

The `## Referenced by` managed body in every entity file MUST be rewritten by `spec lint --fix` from a fresh scan of every other `*.entity.md` file in the project whose `inherits:` resolves to this entity. Each match MUST be rendered as `- Entity: [<child-id>](<relative-path>) *(inherits)*`, where `<child-id>` is the child's frontmatter `id` and `<relative-path>` is the path from the parent file to the child file. Entries MUST be sorted alphabetically by `<child-id>`; ties (which the `entity-frontmatter-required-fields` rule already forbids in a clean tree, but can transiently occur during edits) MUST be broken by `<relative-path>`.

The feature-to-entity back-reference source (a forthcoming `**Consumes:**` / `**Produces:**` declaration on Feature READMEs) is **out of scope** for this Feature per the Idea's Not Doing list — until that mechanism lands, only `inherits:` is wired.

#### REQ: referenced-by-no-references-fallback

When no entity inherits from this entity and no other back-reference source is wired, the managed body MUST be exactly the single line `- _No references yet._`. The managed body MUST NOT be empty (an empty managed body is itself a lint error per [entity#req:referenced-by-managed](https://github.com/specscore/specscore/blob/main/spec/features/entity/README.md#req-referenced-by-managed)).

#### REQ: managed-section-fix-is-idempotent

Running `specscore spec lint --fix` twice consecutively over a tree containing `*.entity.md` files MUST yield no further changes on the second run. This applies independently to `## Properties` and `## Referenced by` and is verified by `TestFixIsIdempotent` against the smoke-test fixtures.

#### REQ: fix-write-ordering

When a single `spec lint --fix` pass mutates managed sections in multiple entity files (e.g., adding a `child.entity.md` whose `inherits:` references `parent.entity.md` means both the child's `## Properties` table AND the parent's `## Referenced by` section need rewriting), the fixer MUST compute every change before writing any file. Per-file writes MUST happen after the full repo scan completes, so the second-pass idempotency contract is satisfied regardless of file-iteration order.

### Navigation verbs

The `specscore entity` command group provides three read-only navigation verbs over the entity graph.

#### REQ: entity-list

`specscore entity list` MUST print every entity id discovered under the project's `spec/features/**/*.entity.md`, one per line, sorted alphabetically by id. The default output is plain text (one id per line) — matching the established pattern of `feature list` (text-default unless `--format` is supplied), and acknowledging that the CLI parent's [REQ: yaml-default-for-structured](../README.md#req-yaml-default-for-structured) applies once `--fields` (or, for `entity list`, an explicit `--format`) is supplied. With `--format yaml` or `--format json`, the output MUST be a structured list whose items carry `id` and `path` fields (path is project-relative). Exit `0` always; an empty project produces empty stdout.

#### REQ: entity-refs

`specscore entity refs <id>` MUST print every consumer of the entity identified by `<id>`. In MVP, a "consumer" is every other `*.entity.md` file whose `inherits:` resolves to `<id>`. The default output is one consumer id per line in plain text — matching the [`feature refs`](../feature/refs/README.md) exemplar (bare ids, no prefix). With `--format yaml` or `--format json`, the output MUST be a structured list under the key `consumers`. When `<id>` resolves but has no consumers, the command MUST exit `0` and print empty stdout (text) or `consumers: []` (yaml/json). The bare-id form is sufficient in MVP because the only consumer kind is `entity`; when a feature-level back-reference source lands (separate Idea), the output shape gains a `entity:` / `feature:` prefix to disambiguate — that's a non-breaking addition because `--format yaml` already groups under the typed `consumers` list.

#### REQ: entity-tree

`specscore entity tree` MUST print the inheritance forest as indented plain text from the project root. Each entity that does not declare `inherits:` is a root; each descendant entity appears under its parent at one extra level of indentation (two spaces per level). Sibling entities at the same level MUST be sorted alphabetically by id. `--format` is NOT supported in MVP — the verb is text-only. Cycles in the inheritance graph (already a lint error via `entity-inherits-acyclic`) MUST be rendered with a `(cycle)` suffix on the first edge that would close the cycle (the descendant whose `inherits:` points back to an ancestor already on the printed path), then the recursion MUST stop for that subtree rather than continue infinitely.

#### REQ: verb-exit-codes

The `specscore entity` verbs follow the shared CLI exit-code contract from [CLI#req:standard-exit-codes](../README.md#req-standard-exit-codes):

| Code | Condition |
|---|---|
| `0` | Success — including the empty-result case |
| `2` | Invalid `--format` value, missing `<id>` for `entity refs` |
| `3` | Project root not found (no `specscore.yaml` in any ancestor) OR (`entity refs` only) `<id>` does not resolve to a discovered entity |
| `10` | Unexpected I/O error during discovery or parsing |

### Discovery scope

#### REQ: discovery-scope

Entity discovery MUST walk `spec/features/**/*.entity.md` from the project root. Files at other locations (e.g., `spec/entities/`, `docs/`) MUST NOT be discovered — they instead trigger the `entity-location` rule on lint. Hidden directories (any path segment starting with `.`) and reserved underscore-prefixed directories (e.g., `_tests/`) MUST be skipped during the walk, matching the convention in `pkg/lint/adherence_footer.go`.

## Parameters

| Verb | Name | Required | Description |
|---|---|---|---|
| `entity refs` | `id` | Yes | Entity id — must resolve to a discovered `<id>.entity.md` file under `spec/features/`. |

## Flags

| Flag | Verbs | Description |
|---|---|---|
| `--project` | all | Project root. Autodetected per [CLI#req:project-autodetect](../README.md#req-project-autodetect) when omitted. |
| `--format` | `list`, `refs` | Output format: `text` (default), `yaml`, `json`. Not supported on `tree`. |

## Exit codes

See [REQ: verb-exit-codes](#req-verb-exit-codes). The lint surface obeys the lint feature's contract; see [spec lint#exit-codes](../spec/lint/README.md#exit-codes).

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [Entity (meta-spec)](https://github.com/specscore/specscore/blob/main/spec/features/entity/README.md) | Authoritative source of the entity Doc-Kind contract. Every `entity-*` rule here implements one or more REQs in that spec. |
| [Property (CLI)](../property/README.md) | Sibling Feature. The `entity-ref-target-exists` rule resolves `ref:` targets against discovered `*.property.md` files. The `## Properties` rendered table reads each referenced property's `data_type` from its frontmatter. |
| [spec lint](../spec/lint/README.md) | Hosts the rule registry and `--fix` flow. Adding the `entity-*` rule family is the principal contract change in this cycle; see [cli/spec/lint#req:entity-and-property-rules-registered](../spec/lint/README.md#req-entity-and-property-rules-registered). |
| [adherence-footer](../../adherence-footer/README.md) | Defines the shared adherence-footer mechanism. The entity URL `https://specscore.md/entity-specification` is registered against the `*.entity.md` consumer path so the shared `adherence-footer` rule covers entity files. |
| [CLI](../README.md) | Inherits the exit-code contract, `--project` autodetection, and the `text|yaml|json` `--format` convention. |
| Source Idea: [entity-and-property-cli-support](../../../ideas/entity-and-property-cli-support.md) | The Idea that promotes to this Feature. |

## Dependencies

- cli/spec/lint
- adherence-footer

## Acceptance Criteria

### AC: lint-rejects-malformed-entity-file

**Requirements:** [cli/entity#req:lint-rules-registered](#req-lint-rules-registered)

Given an entity file at `spec/features/user/user.entity.md` that violates any `entity-*` rule (e.g., missing frontmatter, `id` ≠ slug, duplicate property `name`, broken `ref:` target, `inherits:` cycle), `specscore spec lint` exits `1` with at least one violation whose `Rule` field starts with `entity-`.

### AC: smoke-test-fixture-passes-lint

**Requirements:** [cli/entity#req:lint-rules-registered](#req-lint-rules-registered), [cli/entity#req:adherence-footer-target-registered](#req-adherence-footer-target-registered)

Given the meta-spec smoke-test fixture at `spec/features/idea/user.entity.md`, `specscore spec lint` exits `0` (no `entity-*` violations) when run against the meta-spec repository at HEAD.

### AC: fix-renders-properties-table-from-frontmatter

**Requirements:** [cli/entity#req:properties-table-rendered](#req-properties-table-rendered), [cli/entity#req:managed-section-fix-is-idempotent](#req-managed-section-fix-is-idempotent)

Given an entity file whose frontmatter `properties` list has three items but whose `## Properties` managed body is empty or stale, running `specscore spec lint --fix` rewrites the managed body to a four-column markdown pipe-table (`Name`, `Type`, `Required`, `Description`) with one row per property in frontmatter order. Running `spec lint --fix` a second time produces zero further changes.

### AC: fix-resolves-ref-property-type

**Requirements:** [cli/entity#req:properties-table-rendered](#req-properties-table-rendered)

Given an entity file with a property item `- name: email\n  ref: ../shared/email.property.md` and a sibling property file whose frontmatter declares `data_type: string`, running `specscore spec lint --fix` renders the `Type` cell of the `email` row as `string *(via [email](../shared/email.property.md))*`.

### AC: fix-renders-referenced-by-from-inheritance

**Requirements:** [cli/entity#req:referenced-by-from-inheritance](#req-referenced-by-from-inheritance), [cli/entity#req:fix-write-ordering](#req-fix-write-ordering)

Given a parent entity `parent.entity.md` and a sibling `child.entity.md` whose frontmatter declares `inherits: ./parent.entity.md`, running `specscore spec lint --fix` rewrites the parent's `## Referenced by` managed body to `- Entity: [child](child.entity.md) *(inherits)*` in a single fix pass. Running the fix twice yields no further changes.

### AC: fix-no-references-fallback

**Requirements:** [cli/entity#req:referenced-by-no-references-fallback](#req-referenced-by-no-references-fallback)

Given an entity that no other entity inherits from and no other back-reference source is wired, running `specscore spec lint --fix` rewrites its `## Referenced by` managed body to exactly the single line `- _No references yet._`. An empty managed body is rejected (would be an `entity-referenced-by-managed` lint error before `--fix`).

### AC: id-equals-slug-autofix-rewrites-id

**Requirements:** [cli/entity#req:id-equals-slug-autofix](#req-id-equals-slug-autofix)

Given an entity file at `spec/features/user/user.entity.md` with frontmatter `id: usr` (does not match filename slug `user`), running `specscore spec lint --fix` rewrites the frontmatter to `id: user`, preserves comments and key order in the rest of the frontmatter, and removes the `entity-id-equals-slug` violation from subsequent lint runs.

### AC: entity-list-discovers-fixtures

**Requirements:** [cli/entity#req:entity-list](#req-entity-list), [cli/entity#req:discovery-scope](#req-discovery-scope)

Given a project containing `spec/features/idea/user.entity.md`, running `specscore entity list` exits `0` and prints `user` on stdout (text format). Running with `--format yaml` exits `0` and prints a YAML list with one item carrying `id: user` and `path: spec/features/idea/user.entity.md`.

### AC: entity-refs-no-consumers-exits-0

**Requirements:** [cli/entity#req:entity-refs](#req-entity-refs)

Given a project containing one entity with no inheriting children, running `specscore entity refs <id>` exits `0` with empty stdout (text format) or `consumers: []` (yaml/json).

### AC: entity-refs-unknown-id-exits-3

**Requirements:** [cli/entity#req:entity-refs](#req-entity-refs), [cli/entity#req:verb-exit-codes](#req-verb-exit-codes)

Given no entity matches the supplied `<id>`, running `specscore entity refs <id>` exits `3` with an explanatory stderr message; stdout remains empty.

### AC: entity-tree-shows-inheritance

**Requirements:** [cli/entity#req:entity-tree](#req-entity-tree)

Given `parent.entity.md` and `child.entity.md` where the child inherits from the parent, running `specscore entity tree` exits `0` and prints

```
parent
  child
```

(parent at column 0, child at column 2, trailing newline).

### AC: missing-project-exits-3

**Requirements:** [cli/entity#req:verb-exit-codes](#req-verb-exit-codes)

Running any `specscore entity` verb in a directory tree with no `specscore.yaml` in any ancestor exits `3` with an explanatory stderr message that names `specscore.yaml` as the project anchor.

## Open Questions

- **Should `entity refs <id>` surface entity ↔ entity references beyond `inherits:` in MVP** — e.g., when entity A's `properties` list references a property whose `## Referenced by` lists entity B, does `entity refs A` mention B? The MVP says no (only direct `inherits:` consumers); revisit when the back-reference graph proves too thin in practice.
- **`entity tree --format json` for downstream tooling consumption** — deferred per [REQ: entity-tree](#req-entity-tree). Lock in if any downstream tool (datatug.io, SpecStudio) requests structured tree output.
- **What happens when `inherits:` resolves to a path outside `spec/features/`** (e.g., a typo or pre-meta-spec layout)? Today `entity-inherits-target-exists` would fire; should there be a distinct rule that points at the location? Lean: keep the broader `entity-inherits-target-exists` rule and let the error message disambiguate.
- **Severity escalation for `entity-adherence-footer`** — the shared `adherence-footer` rule ships at `warn` for new consumer-layer Kinds during the MVP rollout (per `pkg/lint/adherence_footer.go` policy). When does entity escalate to `error`? Lock in with a follow-on Idea once a release cycle of clean runs is observed.

---
*This document follows the https://specscore.md/feature-specification*
