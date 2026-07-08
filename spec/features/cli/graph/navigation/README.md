---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: Graph Navigation

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/graph/navigation?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/graph/navigation?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/graph/navigation?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/graph/navigation?op=request-change) |

**Status:** Draft
**Source Ideas:** graphspec-cli-support

## Summary

Graph navigation commands provide structured access to GraphSpec artifacts and their relationships. They help humans, scripts, and agents answer questions about ownership, references, dependencies, impact, and module structure.

## Synopsis

```text
specscore graph list [--kind <kind>] [--module <id>] [--format <text|yaml|json>] [--project <path>]
specscore graph search <query> [--kind <kind>] [--module <id>] [--format <text|yaml|json>] [--project <path>]
specscore graph index [--format <yaml|json>] [--project <path>]
specscore graph stats [--module <id>] [--format <text|yaml|json>] [--project <path>]
specscore graph tree [<ref>] [--direction <up|down|both>] [--format <text|yaml|json>] [--project <path>]
specscore graph refs <ref> [--transitive] [--format <text|yaml|json>] [--project <path>]
specscore graph deps <ref> [--transitive] [--format <text|yaml|json>] [--project <path>]
```

## Problem

Once GraphSpec spans modules and repositories, direct file browsing is not enough. Authors need to find concepts, inspect ownership, follow references, understand module dependencies, and estimate the blast radius of changes without reimplementing a graph parser.

## Behavior

### Listing

#### REQ: graph-list

`specscore graph list` MUST print GraphSpec artifact IDs, one per line, sorted by ID by default. `--kind` filters by GraphSpec kind. `--module` filters by owning module. With `--format yaml|json`, each entry MUST include at least `id`, `kind`, `name`, `owner`, and `path`.

### Search

#### REQ: graph-search

`specscore graph search <query>` MUST search IDs, names, summaries, and optionally body text. The default output MUST be text with one result per line. Structured output MUST include match fields and paths.

### Index

#### REQ: graph-index

`specscore graph index` MUST emit the machine-readable graph index used by other commands. The index SHOULD include modules, artifacts, references, ownership edges, dependency edges, source roots, and unresolved references when present.

### Stats

#### REQ: graph-stats

`specscore graph stats` MUST report aggregate counts by kind, module, status, and reference state. With `--module`, stats are scoped to the selected module.

### Tree

#### REQ: graph-tree

`specscore graph tree` MUST render a tree view of modules and owned artifacts by default. With `<ref>`, it MUST show the selected artifact in context. `--direction up` shows inbound dependency/reference context, `--direction down` shows outbound dependency/reference context, and `--direction both` includes both.

### References and dependencies

#### REQ: graph-refs

`specscore graph refs <ref>` MUST report artifacts that reference the selected artifact. This is the GraphSpec equivalent of "who references this?".

#### REQ: graph-deps

`specscore graph deps <ref>` MUST report artifacts the selected artifact depends on or references. `--transitive` follows dependencies recursively and MUST detect cycles without infinite recursion.

### Impact analysis

Impact analysis may begin as a mode of `refs` before becoming its own command.

#### REQ: impact-as-refs-mode

Until a dedicated `graph impact` command is specified, impact analysis SHOULD be expressed through `graph refs <ref> --transitive --format yaml`, with stable output fields sufficient for agents to reason about blast radius.

### Reference resolution

#### REQ: ref-argument-resolution

Commands that accept `<ref>` MUST resolve references by GraphSpec ID first. If multiple artifacts match a shorthand or display name, the command MUST exit `5` (AmbiguousSlug) with candidate IDs and paths.

## Dependencies

- cli/graph

## Acceptance Criteria

### AC: list-by-kind

**Requirements:** graph/navigation#req:graph-list

Given a graph with one entity and one event, `specscore graph list --kind entity` prints only the entity ID.

### AC: refs-answer-who-uses-this

**Requirements:** graph/navigation#req:graph-refs

Given a Booking command and event that reference `bookius.Booking`, `specscore graph refs bookius.Booking --format yaml` includes both referencing artifacts with their kind and path.

### AC: deps-detects-cycle

**Requirements:** graph/navigation#req:graph-deps

Given a cycle in graph references, `specscore graph deps <ref> --transitive` terminates, marks the cycle in structured output, and does not recurse indefinitely.

## Open Questions

- Should `graph search` include body prose by default, or only frontmatter fields?
- Should dependency edges and reference edges be separate output concepts?
- Should cross-repo references be followed by default or require `--include-external`?
- Should impact analysis become `specscore graph impact <ref>`?
- Should `graph tree` be ownership-first, dependency-first, or configurable?

---
*This document follows the https://specscore.md/feature-specification*
