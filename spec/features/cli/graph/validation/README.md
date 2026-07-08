---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: Graph Validation

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/graph/validation?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/graph/validation?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/graph/validation?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/graph/validation?op=request-change) |

**Status:** Draft
**Source Ideas:** graphspec-cli-support

## Summary

Graph validation commands check GraphSpec structure, YAML, references, IDs, names, ownership, dependencies, lifecycle, and graph consistency.

`graph lint` is the v0.2 command and the only one implemented first. `graph validate` and `graph check-refs` are rule-subset conveniences of `lint` (`--rules graph-schema-*` and `--rules graph-ref-*` respectively) and MAY ship as aliases later; `graph doctor` is staged as Later.

The v0.2 core rule set follows GraphSpec decisions 0004 and 0005: id-equals-filename-stem, id-kebab-case, no-module-prefix-in-id, reference-resolves (including `modelspec://` references once ModelSpec validation is available), ownership-derivable (artifact under a module root, no `owner:` field), dependency-direction (qualified references covered by the owning module's `dependsOn`), and relationship-owner-depends-on-endpoints.

## Synopsis

```text
specscore graph lint [--fix] [--severity <error|warning|info>] [--rules <list>] [--ignore <list>] [--format <text|yaml|json>] [--project <path>]
specscore graph validate [--schema-version <version>] [--format <text|yaml|json>] [--project <path>]
specscore graph doctor [--module <id>] [--format <text|yaml|json|md>] [--project <path>]
specscore graph check-refs [--local-only] [--include-external] [--format <text|yaml|json>] [--project <path>]
```

## Problem

GraphSpec will become useful only if authors can trust graph references and ownership boundaries. Generic Markdown lint is not enough: GraphSpec needs semantic checks for IDs, references, modules, lifecycle, dependency direction, and graph consistency.

## Behavior

### Command roles

| Command | Role |
|---|---|
| `graph lint` | CI-oriented validation of GraphSpec rules. |
| `graph validate` | Schema-oriented validation of GraphSpec artifact shape. |
| `graph doctor` | Human diagnostic report with suggested fixes and improvement opportunities. |
| `graph check-refs` | Focused reference validation, including cross-module and cross-repo links. |

#### REQ: lint-is-ci-entrypoint

`graph lint` MUST be the command intended for CI. It exits `1` when violations at or above `--severity` are found, matching `specscore spec lint`.

#### REQ: validate-is-schema-focused

`graph validate` MUST focus on GraphSpec artifact schema and YAML shape. It MUST NOT perform speculative architectural critique beyond the GraphSpec-defined validation rules.

#### REQ: doctor-is-diagnostic

`graph doctor` MAY include warnings, suggestions, unresolved modelling pressure points, and remediation hints. It MUST clearly distinguish normative validation errors from advisory recommendations.

#### REQ: check-refs-is-reference-focused

`graph check-refs` MUST validate reference resolution without requiring all other graph validation rules to pass. It is the fast path for authors asking "what links are broken?".

### Validation categories

Graph validation should eventually include these categories:

| Category | Examples |
|---|---|
| Structure | expected directories, file naming, required sections |
| YAML | parse errors, required frontmatter fields, unknown kind tokens |
| Identity | duplicate IDs, duplicate names within scope, id/file mismatch |
| References | broken local refs, broken cross-repo refs, unresolved module refs |
| Ownership | artifact placed under a module root, stray `owner:` field present, relationship ownership |
| Dependencies | qualified reference to a module missing from `dependsOn`, cycles |
| Lifecycle | empty or duplicate `lifecycle.states`, unknown state refs, illegal transitions if declared |
| Graph consistency | dangling relationships, inconsistent cardinality, command/event subject mismatch |
| Module consistency | missing module indexes, artifacts outside owning module, distributed-root conflicts |

#### REQ: graph-rule-prefix

GraphSpec lint rules MUST use a `graph-` prefix or a more specific `graph-<kind>-` prefix. They MUST be selectable through `--rules` and suppressible through `--ignore` using the same semantics as `spec lint`.

#### REQ: duplicate-id-detection

Graph validation MUST detect duplicate GraphSpec IDs across discovered graph roots. When duplicates occur across repositories, diagnostics MUST include both project identity and path.

#### REQ: duplicate-name-detection

Graph validation SHOULD detect duplicate display names within the same module and kind. Duplicate names across modules are allowed unless GraphSpec later forbids them.

#### REQ: ownership-validation

Ownership is derived from placement (GraphSpec decision 0005). Graph validation MUST verify that every artifact resides under a resolvable module root; artifacts outside any module root are errors. An `owner:` frontmatter field on any artifact is itself a lint violation, since it duplicates derived ownership. A module that owns zero graph artifacts (a pure structural provider whose surface is only `models/`) is legal and MUST NOT be a violation.

#### REQ: model-ref-resolution

`modelspec://` references in graph artifacts and module-qualified names inside ModelSpec sources MUST resolve per SpecScore decision 0007: local graph root (placement per decision 0006), then configured `specscore.yaml` `projects:` local paths, then an explicit `@{host}/{org}/{repo}` suffix — with no implicit network fetch. Diagnostics MUST distinguish unknown module, unknown concept within a resolved module, and unavailable suffixed repository. Model-level cross-module references MUST be covered by the owning module's `dependsOn`, under the same dependency-direction rule as graph-level references.

#### REQ: dependency-validation

Graph validation MUST verify that every qualified reference from an artifact targets a module listed in the owning module's `dependsOn`, and that a relationship's owning module covers both endpoint modules in its `dependsOn` closure. The CLI MUST NOT invent project-specific architecture rules beyond what GraphSpec defines unless they are configured outside the core GraphSpec validator.

#### REQ: lifecycle-validation

When an entity declares `lifecycle.states`, validation MUST check the list is non-empty and free of duplicates, and that any state reference elsewhere in the artifact resolves to a declared state. Deeper transition validation is deferred until GraphSpec defines lifecycle transition semantics.

### Integration with `specscore spec lint`

Graph validation may be called directly through `specscore graph lint` and may later be included in `specscore spec lint`.

#### REQ: graph-lint-standalone-first

The first GraphSpec validation implementation SHOULD ship as `specscore graph lint` before enabling GraphSpec rules by default in `specscore spec lint`. This avoids breaking existing repos that experiment with `spec/graph/` before GraphSpec is stable.

#### REQ: spec-lint-integration-gated

When GraphSpec rules become part of `specscore spec lint`, that integration MUST be gated by GraphSpec maturity or configuration and MUST be documented in [spec lint](../../spec/lint/README.md).

### Output

Validation output should reuse the existing lint violation shape where possible.

#### REQ: violation-shape-compatible

`graph lint` structured output SHOULD use the same violation fields as `spec lint`: path, line when available, severity, rule, message, and optional fix target. Graph-specific diagnostics MAY add stable keys such as `id`, `kind`, `module`, and `ref`.

## Dependencies

- cli/graph
- cli/spec/lint

## Acceptance Criteria

### AC: broken-reference-reported

**Requirements:** graph/validation#req:check-refs-is-reference-focused

Given a Booking relationship whose `to` reference does not resolve, `specscore graph check-refs` exits `1` and reports the artifact path, reference field, and unresolved target.

### AC: duplicate-id-reported

**Requirements:** graph/validation#req:duplicate-id-detection

Given two GraphSpec artifacts with the same `id`, `specscore graph lint` exits `1` and reports both paths.

### AC: doctor-separates-errors-from-advice

**Requirements:** graph/validation#req:doctor-is-diagnostic

Given a graph with one broken reference and one advisory modelling concern, `specscore graph doctor --format yaml` returns separate `errors` and `recommendations` sections.

## Open Questions

- Should GraphSpec expose machine-readable JSON Schemas for CLI validation?
- Should GraphSpec support semantic validation beyond structural validation in v0.1?
- Should the CLI detect architectural cycles by default or only behind `graph doctor`?
- Should command-event flow validation be structural, semantic, or advisory?
- How should GraphSpec version migrations be supported?
- Should repository-wide architectural health reports be part of `graph doctor` or a separate `graph report` command?

---
*This document follows the https://specscore.md/feature-specification*
