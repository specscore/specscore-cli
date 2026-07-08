---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: Graph Documentation

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/graph/documentation?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/graph/documentation?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/graph/documentation?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/graph/documentation?op=request-change) |

**Status:** Draft
**Source Ideas:** graphspec-cli-support

## Summary

Graph documentation commands render GraphSpec into diagrams, Markdown, HTML, and architecture reports. The proposed commands are `graph render` and `graph export`.

## Synopsis

```text
specscore graph render [--module <id>] [--kind <kind>] [--format <mermaid|dot|md|html>] [--output <path>] [--project <path>]
specscore graph export [--format <json|yaml|mermaid|dot|md|html>] [--module <id>] [--output <path>] [--project <path>]
```

## Problem

GraphSpec is authored for humans and machines. Teams need derived views for pull requests, architecture reviews, documentation sites, and AI review packages. Those views should come from the graph artifacts rather than hand-maintained diagrams.

## Behavior

### Render

`graph render` creates human-facing derived views.

#### REQ: render-formats

`graph render` MUST support `mermaid` and `md` as initial target formats. `dot` and `html` SHOULD be supported when rendering dependencies are available.

#### REQ: render-module-scope

`--module <id>` MUST scope output to one module and its directly referenced external concepts unless `--include-external` is later specified.

#### REQ: render-output-path

When `--output <path>` is supplied, the command writes to that path. When omitted, it writes to stdout. The command MUST refuse to overwrite an existing file unless a future `--force` flag is specified.

### Export

`graph export` creates machine-readable or interchange output.

#### REQ: export-index-compatible

`graph export --format json|yaml` SHOULD use the same graph index model as `graph index`, adding only export metadata such as generation time, project identity, and GraphSpec version.

#### REQ: export-diagram-compatible

`graph export --format mermaid|dot` MUST produce valid diagram source without Markdown wrapping. `graph render --format md` MAY wrap Mermaid or GraphViz source in documentation prose.

### Reports

Documentation generation should include architecture-focused reports.

#### REQ: architecture-report

`graph render --format md` SHOULD support architecture reports that include modules, owned concepts, cross-module references, unresolved questions, and validation summary.

#### REQ: module-report

When scoped with `--module`, Markdown/HTML output SHOULD include a module report listing owned entities, relationships, commands, events, enums, value objects, inbound references, outbound references, and open modelling questions.

## Dependencies

- cli/graph

## Acceptance Criteria

### AC: render-mermaid

**Requirements:** graph/documentation#req:render-formats

Given a graph with Booking reserving Asset, `specscore graph render --format mermaid` emits Mermaid source containing both concepts and the relationship edge.

### AC: export-json-index

**Requirements:** graph/documentation#req:export-index-compatible

`specscore graph export --format json` emits a JSON object with graph artifacts, references, modules, and metadata.

### AC: module-report

**Requirements:** graph/documentation#req:module-report

`specscore graph render --module bookius --format md` emits a Markdown report containing Bookius-owned artifacts and cross-module references.

## Open Questions

- Should Mermaid output be generated directly by the CLI, or by a shared renderer library consumed by the website and CLI?
- Should GraphViz be required or optional?
- Should `graph render --format html` use the site generator, or produce standalone HTML?
- Should architecture reports include advisory diagnostics from `graph doctor`?
- How should generated documentation be marked to avoid hand-edit drift?

---
*This document follows the https://specscore.md/feature-specification*
