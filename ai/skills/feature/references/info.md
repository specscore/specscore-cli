# specscore feature info

Get a compact, structured snapshot of a single feature — metadata, dependency edges, and a section TOC with line ranges — without parsing the README yourself.

**CLI reference:** [`cli/feature/info`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/feature/info/README.md)

## When to use

- **Before reading a spec:** See the section TOC and read only the sections you need
- **Quick metadata check:** Status, parent, dependency and reference counts
- **Triage:** Assess a feature's state without loading the full document
- **Token efficiency:** ~500 tokens of structured data instead of a 2,000–3,000 token README

## Command

```bash
specscore feature info <feature_id> \
  [--format <yaml|json|text>] \
  [--project <path>]
```

## Parameters

| Name | Required | Description |
|---|---|---|
| `<feature_id>` | Yes | Feature to inspect, using path-identification rules (e.g., `cli/version`, `cli/feature/info`). |
| `--format` | No | Output format: `yaml` (default), `json`, `text`. Text is a condensed human-readable rendering. |
| `--project` | No | Project root path. Autodetected from `cwd` if omitted. |

## Exit codes

| Code | Meaning | What to do |
|---|---|---|
| `0` | Feature found | Parse the YAML/JSON output. |
| `2` | Missing `feature_id` or invalid `--format` | Check arguments. |
| `3` | Feature not found | Verify the ID; try [`feature list`](list.md) to find the canonical form. |
| `10+` | Unexpected I/O failure | Log and escalate. |

## Examples

### Inspect a feature

```bash
specscore feature info cli/feature/info
```

```yaml
path: cli/feature/info
status: Stable
deps: []
refs:
  - cli/feature
sections:
  - title: Summary
    lines: [7, 9]
  - title: Synopsis
    lines: [11, 15]
  - title: Problem
    lines: [17, 19]
  - title: Behavior
    lines: [21, 49]
  - title: Parameters
    lines: [51, 55]
  - title: Exit codes
    lines: [57, 64]
  - title: Acceptance Criteria
    lines: [73, 85]
  - title: Outstanding Questions
    lines: [87, 89]
    items: 1
```

### Decide what to read

```bash
# 1. Get the section TOC
specscore feature info cli/feature/new

# 2. Based on the lines field, read only the relevant range
sed -n '21,49p' spec/features/cli/feature/new/README.md
```

### Pipe to yq for a single field

```bash
specscore feature info cli/feature/info --format json | jq -r .status
# Stable
```

## Notes

- **Read-only** — never mutates state.
- Output is YAML by default (agent-first); `--format text` is for quick CLI reads, not parsing.
- Consumers MUST tolerate unknown fields — additional keys may be added in later releases without breaking the contract.
- The `sections` list is a TOC with `[start, end]` line ranges — use it to read specific sections instead of the whole file.
- `items` appears on countable list sections (Outstanding Questions, Dependencies, Acceptance Criteria).
- A feature ID that doesn't resolve exits `3`; no partial output is written to stdout.
