# specscore feature deps

List the features that a given feature depends on. With `--transitive`, walks the dependency chain recursively.

**CLI reference:** [`cli/feature/deps`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/feature/deps/README.md)

## When to use

- **Reading order:** Before reading a feature, see what it builds on
- **Impact analysis (forward):** "If I change X, what does X assume about Y?"
- **Dependency graph build:** Feed the output into a graph tool or planner
- **Cycle smoke-test:** `--transitive` is cycle-safe; if your graph is sound, the result terminates without surprise

## Command

```bash
specscore feature deps <feature_id> \
  [--transitive] \
  [--fields <names>] \
  [--format <yaml|json|text>] \
  [--project <path>]
```

## Parameters

| Name | Required | Description |
|---|---|---|
| `<feature_id>` | Yes | Feature whose dependencies to resolve. |
| `--transitive` | No | Walk the dependency chain recursively. Output is deduplicated and cycle-safe. |
| `--fields` | No | Inline metadata fields per entry (e.g., `status,oq`). |
| `--format` | No | Output format: `text` (default, one ID per line), `yaml`, or `json`. |
| `--project` | No | Project root path. Autodetected from `cwd` if omitted. |

## Exit codes

| Code | Meaning | What to do |
|---|---|---|
| `0` | Dependencies listed (even if empty) | Parse the output. |
| `2` | Missing `<feature_id>` or invalid flag | Check arguments. |
| `3` | `<feature_id>` not found | Verify with [`feature list`](list.md). |
| `10+` | Unexpected I/O failure | Log and escalate. |

## Examples

### Direct dependencies

```bash
specscore feature deps cli/feature/refs
# feature
# cli/feature/deps
```

### Transitive dependencies

```bash
specscore feature deps cli/feature/refs --transitive
# feature
# cli/feature/deps
# cli
```

### With metadata

```bash
specscore feature deps cli/feature/new --fields=status
```

```yaml
- id: feature
  status: Stable
- id: adherence-footer
  status: Stable
- id: cli
  status: In Progress
```

### Zero deps is a valid `0`

```bash
specscore feature deps cli/version
# (empty output, exit 0)
```

A feature without a `## Dependencies` section returns an empty list — not an error.

## Notes

- **Read-only** — never mutates state.
- Dependencies come from the `## Dependencies` section of the target feature's README. Features without that section return an empty list (exit `0`).
- `--transitive` deduplicates: a feature reachable via multiple paths appears exactly once.
- `--transitive` is cycle-safe — graph cycles terminate the walk without error. Cycles are a spec smell that [`spec lint`](../../spec/references/lint.md) flags.
- For the inverse query ("what depends on this?") use [`feature refs`](refs.md).
- For non-empty results, the output line count matches what an editor/planner can chain into; pipe through `wc -l` if you need a count.
