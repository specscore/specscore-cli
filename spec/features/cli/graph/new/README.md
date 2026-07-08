---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: Graph New

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/graph/new?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/graph/new?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/graph/new?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/graph/new?op=request-change) |

**Status:** Draft
**Source Ideas:** graphspec-cli-support

## Summary

`specscore graph new` scaffolds GraphSpec artifacts. The primary form is `specscore graph new <kind>`, where `<kind>` is one of the five GraphSpec kinds: `module`, `entity`, `relationship`, `command`, or `event`. Value objects and enums are ModelSpec concepts and are not scaffolded by `graph new` (GraphSpec decision 0004).

The command should produce reviewable Markdown with YAML frontmatter, using GraphSpec templates rather than hard-coded CLI-only semantics.

## Synopsis

```text
specscore graph new module --name <name> [--id <id>] [--root <path>] [--status <status>] [--summary <text>] [--project <path>]
specscore graph new entity --name <name> --module <module-id> [--id <id>] [--root <path>] [--summary <text>] [--project <path>]
specscore graph new relationship --name <name> --from <ref> --to <ref> --module <module-id> [--id <id>] [--project <path>]
specscore graph new command --name <name> --module <module-id> [--subject <ref>] [--project <path>]
specscore graph new event --name <name> --module <module-id> [--subject <ref>] [--project <path>]
```

Future structural editing commands:

```text
specscore graph rename <ref> <new-name>
specscore graph move <ref> --to-module <module-id>
specscore graph delete <ref>
specscore graph extract <ref> --as <kind>
specscore graph merge <source-ref> <target-ref>
specscore graph split <ref>
```

## Problem

GraphSpec artifacts are intended to be Markdown-first and reviewable, but manual creation still risks inconsistent IDs, paths, frontmatter, headings, and ownership metadata. A scaffolder should remove mechanical mistakes while leaving domain modelling choices visible to the author.

## Behavior

### Artifact kind dispatch

`graph new` dispatches on a required `<kind>` token.

#### REQ: kind-required

`specscore graph new` without `<kind>` MUST exit `2` (InvalidArgs) and print help that lists supported kinds.

#### REQ: supported-kinds

The first implementation MUST support `module`, `entity`, `relationship`, `command`, and `event`. Unknown kind tokens — including the retired `value-object` and `enum` — MUST exit `2`; for the retired tokens the error message SHOULD point authors to ModelSpec.

### Output location

The default same-repo root is `spec/graph/`. Within a graph root, the CLI uses plural consumer directories, not GraphSpec language-kind directory names.

Filenames are the bare local ID with no kind suffix — the plural directory and the frontmatter `kind:` already type the artifact (GraphSpec decision 0005).

| Kind | Default location |
|---|---|
| `module` | `spec/graph/modules/<module-id>/README.md` |
| `entity` | `<module-root>/entities/<id>.md` |
| `relationship` | `<module-root>/relationships/<id>.md` |
| `command` | `<module-root>/commands/<id>.md` |
| `event` | `<module-root>/events/<id>.md` |

#### REQ: plural-consumer-directories

Generated GraphSpec artifacts MUST use plural consumer directories. The CLI MUST NOT scaffold project artifacts under singular language-kind directories such as `entity/` or `relationship/`.

#### REQ: configurable-root

`--root <path>` MUST allow callers to choose a graph root other than `spec/graph/`. The path MUST be project-relative and MUST NOT escape the project root.

#### REQ: module-scaffold

`graph new module` MUST create the module README and MUST scaffold, by default, the five collection directories (`entities/`, `relationships/`, `commands/`, `events/`, `models/`), each with a README index — SpecScore's every-directory-has-a-README rule applies inside graph trees, and the first consumer pilot hit lint failures on every unscaffolded collection directory. A `--bare` flag MAY skip collection scaffolding. `models/` holds the module's ModelSpec sources per decision 0006.

### Frontmatter and body

Generated files must be intentionally minimal and reviewable.

#### REQ: frontmatter-minimum

Every generated artifact MUST include YAML frontmatter with at least `kind`, `id`, `name`, `status`, and `summary` where those fields are meaningful for the GraphSpec kind. Artifacts MUST NOT carry an `owner:` field — ownership is derived from placement under the module root (GraphSpec decision 0005). Relationship artifacts MUST also include `from` and `to` when supplied. Command and Event artifacts MUST include `subject` when supplied. Entity artifacts SHOULD include a `model:` placeholder comment pointing authors at the local `modelspec:///<module>.<Name>` reference form (SpecScore decision 0010).

#### REQ: body-sections

Generated artifacts MUST include at least:

- a level-1 title naming the GraphSpec kind and display name
- `## Description`
- `## Open Questions`

Additional sections MAY be kind-specific, such as `## Relationships`, `## Lifecycle`, `## Failure Cases`, `## Possible Triggers`, or `## Values`.

#### REQ: lint-clean-output

`graph new` MUST produce artifacts that pass `specscore graph lint` for the generated files, assuming referenced modules and concepts exist. It MUST NOT guarantee a globally clean graph when the existing graph already has unrelated violations.

### Naming and IDs

#### REQ: id-derivation

When `--id` is omitted, the CLI MUST derive the ID from `--name` per GraphSpec decision 0005: bare lowercase kebab-case, no module prefix, filename stem equal to the ID. Display case is preserved in `name`. Supplying a module-prefixed `--id` (e.g. `reservations.booking`) MUST exit `2` (InvalidArgs) with a message explaining that qualified IDs are computed, not stored.

#### REQ: collision-check

If the target file already exists, `graph new` MUST exit `1` (Conflict) unless a future `--force` flag is explicitly specified. No partial write may occur before the collision check.

### Relationship scaffolding

#### REQ: relationship-endpoints

`graph new relationship` MUST require `--from` and `--to`. The command SHOULD validate that supplied references resolve before writing. If either endpoint does not resolve, the command MUST exit `3` (NotFound) unless a future `--allow-unresolved` flag is specified.

### Structural editing commands

`rename`, `move`, `delete`, `extract`, `merge`, and `split` are intentionally specified as future design surfaces, not MVP authoring commands.

#### REQ: structural-editing-deferred

The first implementation MAY omit `rename`, `move`, `delete`, `extract`, `merge`, and `split`. Before implementation, each MUST have its own safety contract covering reference rewriting, clean-tree checks, rollback, and cross-repo link updates.

## Dependencies

- cli/graph

## Acceptance Criteria

### AC: scaffold-entity

**Requirements:** graph/new#req:supported-kinds, graph/new#req:frontmatter-minimum, graph/new#req:body-sections

Running `specscore graph new entity --name Booking --module reservations` creates `entities/booking.md` under the reservations module graph root with `kind: entity`, `id: booking`, `name: Booking`, no `owner:` field (ownership is derived from placement), and an `## Open Questions` section.

### AC: relationship-requires-endpoints

**Requirements:** graph/new#req:relationship-endpoints

Running `specscore graph new relationship --name reserves --module reservations` without `--from` and `--to` exits `2` and writes no file.

### AC: no-overwrite-by-default

**Requirements:** graph/new#req:collision-check

Running the same `graph new event` command twice exits `1` on the second invocation and leaves the first file unchanged.

## Open Questions

- Should `graph new` support interactive mode like `idea new -i`?
- Should `--allow-unresolved` exist for architecture sketching before all referenced concepts are created?
- Should structural editing commands require a clean git working tree, following the stricter lifecycle mutation conventions?
- Should `graph extract` be able to extract GraphSpec artifacts from existing FeatureSpec prose?

---
*This document follows the https://specscore.md/feature-specification*
