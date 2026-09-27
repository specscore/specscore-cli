# specscore plan info

Return detailed metadata for a single plan plus a rollup of its tasks' statuses. Metadata comes from the plan's body header; the rollup is computed from the plan's `## Tasks` section.

**CLI reference:** [`cli/plan/info`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/plan/README.md)

## When to use

- **Picking up a plan:** Get the full plan context and where its tasks stand in one call
- **Status check:** See the task-status rollup without opening the file
- **Token efficiency:** Structured YAML/JSON instead of parsing the plan README by hand

## Command

```bash
specscore plan info <slug> \
  [--format <yaml|json|text>] \
  [--project <path>]
```

## Parameters

| Flag | Required | Description |
|---|---|---|
| `<slug>` | Yes | Plan slug to inspect (positional — the file name under `spec/plans/`). |
| `--format` | No | Output format: `yaml` (default), `json`, or `text`. |
| `--project` | No | Project root path. Autodetected from `cwd` if omitted. |

## Exit codes

| Code | Meaning | What to do |
|---|---|---|
| `0` | Plan found | Parse the output. |
| `2` | Missing `<slug>` or invalid `--format` | Supply the slug; check the format value. |
| `3` | No project found, or `spec/plans/<slug>.md` missing | Verify with [`plan list`](list.md). |
| `10+` | Unexpected I/O failure | Log and escalate. |

## Examples

### Inspect a plan

```bash
specscore plan info cli-idea-promote
```

### Single-field extraction

```bash
specscore plan info cli-idea-promote --format json | jq -r .status
```

### Task-status rollup at a glance

```bash
specscore plan info cli-idea-promote --format yaml
# → status, source feature/idea, and a per-status count of the plan's tasks
```

## Notes

- **Read-only** — never mutates state.
- The task-status rollup reflects the plan's `## Tasks` section as authored; `spec lint` is responsible for flagging structural problems.
- For the full list of plans use [`plan list`](list.md). To create a plan use [`plan new`](new.md).
