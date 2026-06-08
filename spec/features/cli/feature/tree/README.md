---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: Feature Tree

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/feature/tree?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/feature/tree?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/feature/tree?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/feature/tree?op=request-change) |
>
> **AI skill:** [GitHub](https://github.com/specscore/ai-plugin-specscore/blob/main/skills/feature/references/tree.md) · [local](../../../../../../ai-plugin-specscore/skills/feature/references/tree.md) — if this command's CLI signature or behavior changes, update the linked skill to keep agents in sync.

**Status:** Stable
**Source Ideas:** —

## Summary

`specscore feature tree` renders the project's feature hierarchy as an indented tree. Without an argument, it shows the full tree. With a `<feature_id>`, it focuses on that feature — ancestors plus subtree by default, or narrowed with `--direction`.

## Synopsis

```
specscore feature tree [<feature_id>] [--direction <up|down>] [--fields <names>] [--format <yaml|json|text>] [--project <path>]
```

## Problem

The flat ID list returned by `feature list` loses hierarchy. To reason about where a feature sits in the tree, callers want a visual (text) rendering for humans and a structured rendering (YAML/JSON) for tools. A single command that switches output shapes keeps the mental model consistent.

## Behavior

### Default scope

Without a `<feature_id>`, the command renders the full feature tree starting from the project root.

With a `<feature_id>`, the default view includes:

- Every ancestor from the project root down to the target feature (the **path to root**).
- The target feature.
- Every descendant of the target (its **subtree**).

#### REQ: default-shows-context

When a `<feature_id>` is supplied, the default output MUST include both ancestors and subtree so the feature is shown in context. Unfocused siblings of ancestors MUST be omitted.

### Direction narrowing

`--direction` MUST be used only in conjunction with a `<feature_id>`.

#### REQ: direction-requires-id

If `--direction` is supplied without a `<feature_id>`, the command MUST exit `2` (InvalidArgs) with a message naming the conflict.

#### REQ: direction-values

`--direction up` MUST render only the path from the project root down to the target (ancestors + target, no subtree). `--direction down` MUST render only the target and its subtree (no ancestors). Any other value MUST exit `2`.

### Output formats

Text output (default) uses indentation and the Unicode character `*` to mark the focused feature when a `<feature_id>` is supplied; root and non-focused nodes are rendered plain. YAML / JSON output represents the tree as nested objects.

#### REQ: focus-marker

In text output, exactly one node — the focused feature — MUST be marked with a `*` prefix. The full-tree view (no `<feature_id>`) has no focus marker.

### Open-question surfacing

In addition to the count exposed by `--fields oq`, the tree command MUST accept `--fields questions` to surface the text of each item in a feature's `## Open Questions` section, and `--fields title` to surface the feature's H1 (with the conventional `Feature: ` prefix stripped). These fields compose with the rest of the registry, so `--fields title,oq,questions` renders a question-aware project tree.

#### REQ: questions-field

`--fields questions` MUST return one string per top-level `- ` item under `## Open Questions`, in document order. When the section is absent or contains no items, the field MUST be omitted (YAML/JSON) so callers can distinguish "no questions" from "field not requested". The count returned by `--fields oq` MUST equal the length of the list returned by `--fields questions` for the same feature.

#### REQ: title-field

`--fields title` MUST return the text of the README's first H1, with a leading `Feature:` (and optional whitespace) stripped to match the scaffolding template. When no H1 is present, the field MUST be omitted.

## Parameters

| Name | Required | Description |
|---|---|---|
| `feature_id` | No | Feature to focus on. If omitted, the full tree is rendered. |

## Exit codes

| Code | Condition |
|---|---|
| `0` | Tree rendered |
| `2` | `--direction` without `<feature_id>`, or invalid `--direction` / `--format` value |
| `3` | Supplied `<feature_id>` not found |
| `10` | Unexpected I/O failure |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [feature](../../../feature/README.md) | Source of the parent/child relationships rendered by this command. |
| [CLI Feature group](../README.md) | Inherits `--fields`, `--format`, `--project`. |

## Acceptance Criteria

### AC: full-tree-renders

**Requirements:** cli/feature/tree#req:default-shows-context

`specscore feature tree` (no argument) prints every feature in the project with nesting that reflects directory structure.

### AC: focused-tree-shows-context

**Requirements:** cli/feature/tree#req:default-shows-context, cli/feature/tree#req:focus-marker

`specscore feature tree cli/version` prints `cli` (ancestor) and `cli/version` (focused, marked with `*`), plus any children of `cli/version`. Sibling ancestors (other top-level features) are not printed.

### AC: direction-up-down

**Requirements:** cli/feature/tree#req:direction-requires-id, cli/feature/tree#req:direction-values

`tree cli/version --direction up` prints only the path from the root to `cli/version`. `tree cli/version --direction down` prints only `cli/version` and its subtree. `tree --direction up` (no ID) exits `2`.

### AC: questions-and-title-fields

**Requirements:** cli/feature/tree#req:questions-field, cli/feature/tree#req:title-field

Given a feature `a` whose README starts with `# Feature: Alpha` and contains an `## Open Questions` section with two `- ` items (`Q1?` and `Q2?`), `specscore feature tree a --fields title,oq,questions --format yaml` returns an entry with `title: Alpha`, `oq: 2`, and a `questions` list `[Q1?, Q2?]`. For a feature with no `## Open Questions` section, the same command omits both `oq` and `questions` (or returns `oq: 0` and omits `questions`).

## Open Questions

- Should the focus marker be configurable (e.g., `--focus-marker '>'`) for environments where Unicode `*` clashes with other output — or is one stable marker sufficient?

---
*This document follows the https://specscore.md/feature-specification*
