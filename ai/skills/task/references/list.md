# specscore task list

List tasks from the project task board, optionally filtered by status. Default output is YAML; `--format md` re-emits the board table verbatim for round-tripping.

**CLI reference:** [`cli/task/list`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/task/list/README.md)

## When to use

- **What's on the board?** Get the full task list as structured data
- **Pick what to work on:** Filter to `--status planning` or `--status in_progress`
- **Round-trip the table:** Use `--format md` to read the board exactly as it appears on disk
- **Audit before mutation:** Dump the board before [`task new`](new.md) so you can diff after

## Command

```bash
specscore task list \
  [--status <status>] \
  [--format <yaml|json|md>] \
  [--project <path>]
```

## Parameters

| Flag | Required | Description |
|---|---|---|
| `--status` | No | Filter to one status value (e.g., `planning`, `in_progress`). Empty (`--status ""`) is treated as unset. |
| `--format` | No | Output format: `yaml` (default), `json`, or `md` (re-emits the board table). |
| `--project` | No | Project root path. Autodetected from `cwd` if omitted. |

## Exit codes

| Code | Meaning | What to do |
|---|---|---|
| `0` | Listing printed (even if board is empty) | Parse the output. |
| `2` | Invalid `--status` or `--format` | Check arguments. |
| `3` | No project found, or project has no `tasks/README.md` | Verify the project; create the board with [`task new`](new.md) if missing. |
| `10+` | Unexpected I/O failure | Log and escalate. |

## Examples

### Default — YAML list

```bash
specscore task list
```

```yaml
- slug: ship-onboarding
  title: "Ship onboarding flow"
  status: in_progress
  description: "Set up the welcome screen and first-run survey."
- slug: cleanup-flag
  title: "Remove legacy feature flag"
  status: planning
```

### Filter by status

```bash
specscore task list --status planning
```

### Round-trip the board (Markdown out)

```bash
specscore task list --format md > /tmp/board.md
diff tasks/README.md /tmp/board.md
# (no diff — byte-for-byte equivalent modulo trailing newline)
```

### JSON for tooling

```bash
specscore task list --format json | jq -r '.[] | select(.status=="planning") | .slug'
```

## Notes

- **Read-only** — never mutates state.
- The output reflects the **board file** (`tasks/README.md`) — not the union of `tasks/<slug>/` directories. A task whose folder exists but isn't on the board will not appear in this output. That mismatch is a [`spec lint`](../../spec/references/lint.md) concern, not a `task list` concern.
- `--format md` produces the original board table; useful for stable diffs and CI gating.
- Unknown `--status` values exit `2` rather than returning empty output — failure is loud.
- For a single task's full detail (description, dependencies, summary) use [`task info`](info.md).
