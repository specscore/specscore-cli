---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: Graph (CLI)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/graph?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/graph?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/graph?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/graph?op=request-change) |

**Status:** Draft
**Source Ideas:** graphspec-cli-support

## Summary

`specscore graph` is the CLI command group for GraphSpec. It provides tooling around GraphSpec artifacts: authoring, validation, navigation, documentation/export, and future AI-assisted review.

GraphSpec defines the language. The CLI consumes GraphSpec and must not make GraphSpec dependent on this implementation.

## Problem

GraphSpec is intended to become SpecScore's canonical domain modelling language. Without a first-class CLI surface, authors and agents would hand-author graph artifacts, grep references, and build ad hoc validators. That would repeat the drift problems already solved for Ideas, Features, Plans, Entities, and Properties.

The CLI needs a coherent GraphSpec surface that feels native to the existing command hierarchy while keeping GraphSpec implementation-independent.

## Contents

| Directory | Description |
|---|---|
| [new/](new/README.md) | Authoring commands for scaffolding GraphSpec artifacts. |
| [validation/](validation/README.md) | Validation commands for structure, references, ownership, dependencies, lifecycle, and graph consistency. |
| [navigation/](navigation/README.md) | Query commands for list, search, index, stats, tree, refs, deps, and impact analysis. |
| [documentation/](documentation/README.md) | Render and export commands for diagrams, Markdown, HTML, and reports. |
| [ai-assistance/](ai-assistance/README.md) | Future AI-oriented commands for review, explanation, suggestions, scaffolding, and summaries. |

### new

`graph new` scaffolds GraphSpec artifacts. The expected form is `specscore graph new <kind>`, where `<kind>` is one of `module`, `entity`, `relationship`, `command`, `event`, `value-object`, or `enum`.

### validation

`graph lint`, `graph validate`, `graph doctor`, and `graph check-refs` validate GraphSpec structure and graph semantics. `graph lint` is the CI-oriented entry point; `graph doctor` is the diagnostic authoring aid.

### navigation

`graph list`, `graph search`, `graph index`, `graph stats`, and `graph tree` provide read-only navigation over graph artifacts and relationships. They answer questions such as "who references this?", "what does this module own?", and "what changes if this concept moves?".

### documentation

`graph render` and `graph export` generate derived documentation and interchange formats such as Mermaid, GraphViz, Markdown, HTML, architecture reports, and module reports.

### ai-assistance

Future `graph review`, `graph explain`, `graph suggest`, `graph scaffold`, and `graph summarize` commands prepare structured context for AI agents and frontier reasoning models. These commands must remain consumers of GraphSpec, not sources of GraphSpec language semantics.

## Behavior

### Command placement

`specscore graph` is a root command group, parallel to `specscore idea`, `specscore feature`, `specscore entity`, `specscore property`, and `specscore spec`.

#### REQ: graph-root-command

The CLI MUST expose a root command group named `graph`. Running `specscore graph` without a subcommand MUST show help and exit `0`; it MUST NOT default to list, validate, or any other action.

### Naming rationale

The initial command name is `graph` because consumer projects use `spec/graph/`, the command is short, and it follows the CLI parent convention of singular resource names.

Alternatives considered:

- `graphspec`: precise but longer and less consistent with consumer directory names.
- `domain`: broader than GraphSpec and likely to collide with future non-graph domain tooling.
- `model`: too generic and easily confused with AI model commands.

The command name remains reviewable before implementation, but this feature specifies `graph` as the working contract.

### Scope boundary

The CLI supports GraphSpec authoring and analysis. It does not define GraphSpec.

#### REQ: graphspec-owned-by-language

When CLI behavior depends on GraphSpec language semantics, the CLI specification MUST reference the GraphSpec specification as the source of truth. CLI specs may define command shape, output, traversal behavior, exit codes, and diagnostics, but MUST NOT introduce new GraphSpec language concepts that are absent from GraphSpec itself.

### Discovery

GraphSpec roots may be centralised or distributed. A project may have one system-level `spec/graph/`, one graph root per module repository, or cross-repo references to graph artifacts owned elsewhere.

#### REQ: graph-root-discovery

Graph commands MUST discover GraphSpec roots through the same SpecScore project/module configuration and cross-repo linking machinery used by other specs. The default same-repo discovery root is `spec/graph/`. Future version constraints MUST live in the unified SpecScore link model, not in a GraphSpec-specific mechanism.

#### REQ: distributed-graph-roots

Graph commands MUST be designed so a graph can span multiple SpecScore-managed repositories. Commands that can traverse references (`refs`, `deps`, `tree`, `check-refs`, `impact`, `render`, `review`) MUST specify whether they operate on local-only roots by default or include configured cross-repo roots.

### Artifact kinds

The CLI recognizes the initial GraphSpec kinds from the GraphSpec bootstrap:

| Kind | Command token | Typical plural directory |
|---|---|---|
| ModuleSpec | `module` | `modules/` |
| EntitySpec | `entity` | `entities/` |
| RelationshipSpec | `relationship` | `relationships/` |
| CommandSpec | `command` | `commands/` |
| EventSpec | `event` | `events/` |
| ValueObjectSpec | `value-object` | `value-objects/` |
| EnumSpec | `enum` | `enums/` |

#### REQ: kind-token-vocabulary

Commands that accept a GraphSpec kind MUST accept the command tokens in the table above. Unknown kind tokens MUST exit `2` (InvalidArgs) and name the offending token.

### Output conventions

Graph read commands follow the existing CLI conventions:

- Text defaults are pipe-friendly when the command primarily lists IDs.
- Structured graph results default to YAML.
- JSON is available for automation where documented.

#### REQ: graph-output-formats

Graph read commands MUST use `--format` values from the existing CLI vocabulary: `text`, `yaml`, `json`, plus command-specific formats such as `mermaid`, `dot`, `md`, and `html` only where explicitly documented by the command feature.

### Relationship to legacy entity/property commands

`specscore entity` and `specscore property` already exist for legacy SpecScore Doc-Kinds under `spec/features/**/*.entity.md` and `spec/features/**/*.property.md`. GraphSpec entity/value-object/property questions are separate.

#### REQ: no-legacy-entity-collision

`specscore graph` MUST NOT reuse the existing `specscore entity` or `specscore property` command groups for GraphSpec artifacts. GraphSpec entities are addressed through `specscore graph` to avoid changing the existing command contracts.

## Dependencies

- cli/spec/lint

## Acceptance Criteria

### AC: graph-help-is-root-surface

**Requirements:** graph#req:graph-root-command

Running `specscore graph` exits `0` and prints help showing the `new`, `lint`, `validate`, `doctor`, `check-refs`, `list`, `search`, `index`, `stats`, `tree`, `render`, and `export` command families or their implemented subset. It does not mutate files and does not implicitly list artifacts.

### AC: graph-does-not-change-legacy-entity

**Requirements:** graph#req:no-legacy-entity-collision

Adding `specscore graph new entity` does not change the behavior or discovery scope of `specscore entity list`, which remains bound to legacy `spec/features/**/*.entity.md` files.

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [CLI](../README.md) | Inherits root command naming, shared flags, exit codes, and output conventions. |
| [spec lint](../spec/lint/README.md) | Graph validation rules may be included in repo-wide lint once GraphSpec schemas stabilize. |
| [Entity (CLI)](../entity/README.md) | Existing legacy entity command group remains separate from GraphSpec entity tooling. |
| [Property (CLI)](../property/README.md) | Existing legacy property command group remains separate from any future GraphSpec property support. |
| Source Idea: [graphspec-cli-support](../../../ideas/graphspec-cli-support.md) | Promotes to this command group and its child features. |

## Open Questions

- Should `graph` remain the long-term command name, or should it become an alias for a more explicit `graphspec` command before implementation?
- Should GraphSpec roots be discovered only from `spec/graph/` initially, or should the first implementation include configured cross-repo module roots?
- Should the CLI support a top-level `graph impact` command, or should impact analysis live under `graph refs` / `graph deps` flags?
- Should GraphSpec validation run automatically inside `specscore spec lint`, or only when `specscore graph lint` is invoked?
- How should the CLI report GraphSpec language version compatibility when GraphSpec itself is still bootstrap-stage?

---
*This document follows the https://specscore.md/feature-specification*
