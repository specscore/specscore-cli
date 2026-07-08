---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: Graph AI Assistance

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/graph/ai-assistance?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/graph/ai-assistance?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/graph/ai-assistance?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/graph/ai-assistance?op=request-change) |

**Status:** Draft
**Source Ideas:** graphspec-cli-support

## Summary

Graph AI assistance commands prepare GraphSpec context for AI agents and frontier reasoning models. Proposed commands include `graph review`, `graph explain`, `graph suggest`, `graph scaffold`, and `graph summarize`.

These commands are future-facing. They should be specified before implementation so AI workflows use stable, auditable CLI surfaces rather than private prompt glue.

## Synopsis

```text
specscore graph review [--module <id>] [--scope <local|cross-repo>] [--format <md|yaml|json>] [--project <path>]
specscore graph explain <ref> [--format <text|md|yaml|json>] [--project <path>]
specscore graph suggest [--module <id>] [--kind <kind>] [--format <md|yaml|json>] [--project <path>]
specscore graph scaffold <brief-file> [--kind <kind>] [--dry-run] [--format <md|yaml|json>] [--project <path>]
specscore graph summarize [--module <id>] [--format <text|md|yaml|json>] [--project <path>]
```

## Problem

AI agents will use GraphSpec for reasoning, planning, migration analysis, and implementation task generation. Without a CLI-supported context surface, every agent will assemble its own partial graph context, leading to inconsistent reviews and hard-to-audit recommendations.

## Behavior

### Review packages

#### REQ: graph-review-package

`graph review` MUST produce a deterministic review package containing relevant artifacts, validation summary, cross-module references, open questions, and modelling pressure points. It SHOULD be suitable for handoff to a frontier reasoning model.

#### REQ: review-no-mutation

`graph review` MUST be read-only. It MUST NOT rewrite GraphSpec artifacts or accept model-generated changes directly.

### Explain

#### REQ: graph-explain

`graph explain <ref>` MUST summarize what the artifact is, who owns it, what it references, who references it, known lifecycle/permission notes, and open questions. Structured output MUST expose those sections as stable keys.

### Suggest

#### REQ: graph-suggest

`graph suggest` MAY identify missing relationships, duplicate concepts, ambiguous ownership, suspicious command-event flows, lifecycle gaps, or possible refactorings. Suggestions MUST be labelled advisory and MUST NOT be treated as lint violations unless GraphSpec later defines a normative rule.

### Scaffold

#### REQ: graph-ai-scaffold-dry-run-first

`graph scaffold` MUST default to dry-run or require `--apply` before writing files. AI-generated scaffolds need a review gate before mutation.

#### REQ: graph-ai-scaffold-uses-graph-new

When `graph scaffold` writes files, it MUST delegate artifact creation semantics to `graph new` templates and safety checks rather than inventing a separate write path.

### Summarize

#### REQ: graph-summarize

`graph summarize` MUST produce concise summaries of modules or graph roots for humans and agents. Summaries SHOULD include counts, important concepts, cross-module references, unresolved questions, and validation status.

### Downstream generation

AI commands may prepare downstream specs, but GraphSpec remains the source vocabulary.

#### REQ: downstream-generation-is-proposal

Commands that generate FeatureSpecs, implementation tasks, migration plans, or API sketches from GraphSpec MUST emit proposals or draft artifacts. They MUST NOT imply that GraphSpec itself depends on those downstream specs.

## Dependencies

- cli/graph

## Acceptance Criteria

### AC: review-package-is-deterministic

**Requirements:** graph/ai-assistance#req:graph-review-package

Running `specscore graph review --module bookius --format md` twice on the same graph emits equivalent content in deterministic order.

### AC: explain-shows-references

**Requirements:** graph/ai-assistance#req:graph-explain

`specscore graph explain bookius.Booking --format yaml` includes owner, kind, outbound references, inbound references, and open questions.

### AC: suggestions-are-advisory

**Requirements:** graph/ai-assistance#req:graph-suggest

`specscore graph suggest --module bookius` labels findings as advisory recommendations, not lint errors.

## Open Questions

- How should AI review workflows integrate with GraphSpec and Consilium?
- Should `graph review` prepare one package per module, per graph root, or per pull request diff?
- Should AI commands call local models, remote models, or only prepare context for external tools?
- Should generated FeatureSpecs from GraphSpec be emitted by `graph suggest`, `graph scaffold`, or a future cross-spec command?
- How should the CLI record provenance for AI-generated graph changes?

---
*This document follows the https://specscore.md/feature-specification*
