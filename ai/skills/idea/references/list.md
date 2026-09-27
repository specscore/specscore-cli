# specscore idea list

Lists all Idea artifacts in the current project.

## Usage

```bash
specscore idea list [--status=<filter>] [--all] [--format=<fmt>] [--project=<path>] [--caller=claude]
```

## Flags

| Flag | Description |
|------|-------------|
| `--status` | Filter by lifecycle status (case-insensitive). E.g. `--status=Draft`, `--status=Approved`. Omit to show all non-archived ideas. |
| `--all` | Include archived ideas in the output. |
| `--format` | Output format: `text` (default), `yaml`, `json`. |
| `--project` | Project root (autodetected from cwd if omitted). |
| `--caller` | Always pass `--caller=claude` when invoking from an AI agent. |

## Default output (text)

One slug per line, sorted alphabetically:

```
map-skill
retrofit-evaluation-skill
retrofit-skill
synchestra-removal
```

## Enriched output (yaml/json)

With `--format=yaml` or `--format=json`, each idea includes:

```yaml
- slug: map-skill
  status: Draft
  path: spec/ideas/map-skill.md
  archived: false
```

## Examples

```bash
# List all non-archived ideas
specscore idea list --caller=claude

# List only Draft ideas
specscore idea list --status=Draft --caller=claude

# List all ideas including archived, as JSON
specscore idea list --all --format=json --caller=claude

# List Approved ideas ready for specification
specscore idea list --status=Approved --caller=claude
```

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success (even if no ideas match the filter) |
| 1 | Unexpected error (I/O failure, etc.) |
| 10 | spec/ directory not found in project |
