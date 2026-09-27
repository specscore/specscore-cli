# specscore feature tree

Render the feature hierarchy as an indented tree. With no argument, shows the full project tree. With a `<feature_id>`, focuses on that feature in context — ancestors plus subtree by default, or narrowed with `--direction`.

**CLI reference:** [`cli/feature/tree`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/feature/tree/README.md)

## When to use

- **Visualizing structure:** See where a feature sits in the project hierarchy
- **Onboarding:** Get a one-page mental model of a project's feature graph
- **Scoping a change:** `--direction down` shows everything under a node; `--direction up` shows the path to root
- **Replaces multi-grep:** Avoid stitching `feature list` output back into a tree by hand

## Command

```bash
specscore feature tree [<feature_id>] \
  [--direction <up|down>] \
  [--fields <names>] \
  [--format <yaml|json|text>] \
  [--project <path>]
```

## Parameters

| Name | Required | Description |
|---|---|---|
| `<feature_id>` | No | Feature to focus on. If omitted, the full tree is rendered. |
| `--direction` | No | `up` shows ancestors only; `down` shows subtree only. Requires `<feature_id>`. |
| `--fields` | No | Inline metadata appended to each entry. Recognized: `title`, `status`, `oq` (count), `questions` (full text), `deps`, `refs`, `children`, `plans`. |
| `--format` | No | Output format: `text` (default, indented), `yaml`, or `json` (nested objects). |
| `--project` | No | Project root path. Autodetected from `cwd` if omitted. |

## Exit codes

| Code | Meaning | What to do |
|---|---|---|
| `0` | Tree rendered | Read the output. |
| `2` | `--direction` without `<feature_id>`, or invalid flag value | Supply a feature ID, or check `--direction` / `--format`. |
| `3` | `<feature_id>` not found | Verify with [`feature list`](list.md). |
| `10+` | Unexpected I/O failure | Log and escalate. |

## Examples

### Full tree

```bash
specscore feature tree
```

```
cli
├── code
│   └── deps
├── feature
│   ├── deps
│   ├── info
│   ├── list
│   ├── new
│   ├── refs
│   └── tree
├── idea
│   └── new
├── spec
│   └── lint
├── task
│   ├── info
│   ├── list
│   └── new
└── version
```

### Focused tree (default — context view)

```bash
specscore feature tree cli/feature/info
```

The output includes every ancestor down to `cli/feature/info` plus its subtree. The focused node is marked with `*`. Sibling ancestors (e.g., other top-level features) are omitted.

### Direction up — path to root only

```bash
specscore feature tree cli/feature/info --direction up
# cli
# └── feature
#     └── * info
```

### Direction down — subtree only

```bash
specscore feature tree cli/feature --direction down
# * feature
# ├── deps
# ├── info
# ├── list
# ├── new
# ├── refs
# └── tree
```

### Structured output for tooling

```bash
specscore feature tree --format json | jq '.children[].id'
```

### Outstanding-question dashboard

```bash
specscore feature tree --fields title,oq,questions
```

```yaml
- path: cli/feature
  title: Feature (CLI)
  oq: 2
  questions:
    - Should field names move to a shared registry, or stay documented per-command?
    - Should `feature new` accept a `--from-idea <slug>` flag?
  child_nodes:
    - path: cli/feature/tree
      title: Feature Tree
      oq: 1
      questions:
        - Should the focus marker be configurable?
```

When researching a feature before changing it or implementing against it, run `tree --fields=oq,questions` first. A non-zero `oq` means the spec author flagged unresolved decisions — read those before assuming the spec is settled. Implementing on top of an open question often means picking a side the human hasn't picked yet.

`oq` is the count; `questions` is the full text of each top-level `- ` item under the README's `## Outstanding Questions` section, in document order. The two stay in lockstep — `len(questions) == oq`. When a feature has no `## Outstanding Questions` section, `oq` is `0` and `questions` is omitted.

## Notes

- **Read-only** — never mutates state.
- Default text output uses indentation and a `*` prefix to mark the focused feature when `<feature_id>` is supplied.
- The full-tree view (no argument) has no focus marker.
- `--direction` is rejected without `<feature_id>` (exits `2`) — direction has no meaning on the full tree.
- For a flat ID list use [`feature list`](list.md); for dependency edges use [`feature deps`](deps.md) or [`feature refs`](refs.md).
