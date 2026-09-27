# specscore feature refs

Find every feature that names the target feature as a dependency — the inverse of [`feature deps`](deps.md). With `--transitive`, walks the reverse graph recursively.

**CLI reference:** [`cli/feature/refs`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/feature/refs/README.md)

## When to use

- **Impact analysis (reverse):** "If I change X, what else depends on it?"
- **Refactor safety:** Scope a rename or removal by enumerating downstream consumers
- **Documentation cross-checks:** See whether a foundational feature is actually referenced

## Command

```bash
specscore feature refs <feature_id> \
  [--transitive] \
  [--fields <names>] \
  [--format <yaml|json|text>] \
  [--project <path>]
```

## Parameters

| Name | Required | Description |
|---|---|---|
| `<feature_id>` | Yes | Feature whose referrers to find. |
| `--transitive` | No | Walk the reverse graph recursively. Same dedup + cycle-safety rules as `feature deps`. |
| `--fields` | No | Inline metadata fields per entry (e.g., `status`). |
| `--format` | No | Output format: `text` (default), `yaml`, or `json`. |
| `--project` | No | Project root path. Autodetected from `cwd` if omitted. |

## Exit codes

| Code | Meaning | What to do |
|---|---|---|
| `0` | References listed (even if empty) | Parse the output. |
| `2` | Missing `<feature_id>` or invalid flag | Check arguments. |
| `3` | `<feature_id>` not found | Verify with [`feature list`](list.md). |
| `10+` | Unexpected I/O failure | Log and escalate. |

## Examples

### Direct references

```bash
specscore feature refs cli
# cli/code
# cli/feature
# cli/idea
# cli/spec
# cli/task
# cli/version
```

### Transitive references

```bash
specscore feature refs cli --transitive
# cli/code
# cli/code/deps
# cli/feature
# cli/feature/deps
# cli/feature/info
# ...
```

### "Nothing references this" is a valid answer

```bash
specscore feature refs orphan-feature
# (empty output, exit 0)
```

A target with zero referrers exits `0` with empty output — not `3`.

### With status metadata

```bash
specscore feature refs source-references --fields=status
```

```yaml
- id: cli/code
  status: Stable
- id: cli/code/deps
  status: Stable
```

## Notes

- **Read-only** — never mutates state.
- The command scans every feature README in the project and reports those whose `## Dependencies` section names the target. Features without a `## Dependencies` section never appear.
- `--transitive` applies the same deduplication and cycle-safety guarantees as [`feature deps --transitive`](deps.md). Consumers can rely on symmetric behavior in both directions.
- An unresolvable `<feature_id>` exits `3`; an unresolved-but-no-referrers case exits `0` with empty output. Don't conflate the two.
- Pair with [`feature deps`](deps.md) for the forward edge.
