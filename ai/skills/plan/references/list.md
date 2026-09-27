# specscore plan list

List the project's plan slugs, optionally filtered by status and projected to selected metadata fields. Default output is the `text` slug list; `--format yaml|json` adds structure for tooling.

**CLI reference:** [`cli/plan/list`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/plan/README.md)

## When to use

- **What plans exist?** Get the full plan list as slugs or structured data
- **Triage by status:** Filter to `--status Approved` or `--status Executing`
- **Project fields:** Use `--fields` to pull just the metadata columns you need
- **Audit before scaffolding:** Dump the list before [`plan new`](new.md) so you can diff after

## Command

```bash
specscore plan list \
  [--status <status>] \
  [--fields <comma-separated>] \
  [--format <text|yaml|json>] \
  [--project <path>]
```

## Parameters

| Flag | Required | Description |
|---|---|---|
| `--status` | No | Filter by status (case-insensitive exact match), e.g. `Approved`, `Executing`. |
| `--fields` | No | Comma-separated metadata fields to include: `status`, `source-feature`, `mode`, `date`, `owner`. |
| `--format` | No | Output format: `text` (default — slugs, one per line), `yaml`, or `json`. |
| `--project` | No | Project root path. Autodetected from `cwd` if omitted. |

## Exit codes

| Code | Meaning | What to do |
|---|---|---|
| `0` | Listing printed (even if there are no plans) | Parse the output. |
| `2` | Invalid `--status`, `--fields`, or `--format` value | Check arguments. |
| `3` | No project found, or the project has no `spec/plans/` | Verify the project; create a plan with [`plan new`](new.md). |
| `10+` | Unexpected I/O failure | Log and escalate. |

## Examples

### Default — slug list

```bash
specscore plan list
```

### Filter by status

```bash
specscore plan list --status Executing
```

### Selected fields as YAML

```bash
specscore plan list --fields status,owner,date --format yaml
```

### JSON for tooling

```bash
specscore plan list --format json | jq -r '.[] | select(.status=="Approved") | .slug'
```

## Notes

- **Read-only** — never mutates state.
- Unknown `--status` / `--fields` / `--format` values exit `2` rather than returning empty output — failure is loud.
- For a single plan's full detail (metadata plus a task-status rollup) use [`plan info`](info.md).
