# specscore feature list

List all features in a project to get an overview of what capabilities exist.

**CLI reference:** [`cli/feature/list`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/feature/list/README.md)

## When to use

- **Surveying a project:** Get a complete enumeration of every feature ID
- **Finding a feature ID:** Look up the exact ID before using `info`, `deps`, or `refs`
- **Status overview:** Use `--fields=status` to see the state of every feature at once
- **Pipe targets:** Default text output is one ID per line, ready for `grep`, `fzf`, `xargs`

## Command

```bash
specscore feature list \
  [--fields <names>] \
  [--format <yaml|json|text>] \
  [--project <path>]
```

## Parameters

| Flag | Required | Description |
|---|---|---|
| `--fields` | No | Comma-separated metadata fields to include per entry. Recognized: `title`, `status`, `oq` (count), `questions` (full text), `deps`, `refs`, `children`, `plans`. Forces structured output. |
| `--format` | No | Output format: `yaml`, `json`, or `text`. Default `text` without `--fields`; auto-upgrades to `yaml` when `--fields` is set. |
| `--project` | No | Project root path. Autodetected by walking up from `cwd` to find `specscore-spec-repo.yaml` if omitted. |

## Exit codes

| Code | Meaning | What to do |
|---|---|---|
| `0` | Success | Parse the output — one feature ID per line (text), or a YAML/JSON list. |
| `2` | Invalid arguments | Check `--fields` names and `--format` value. |
| `3` | No project found | Verify you're inside a SpecScore project, or pass `--project`. |
| `10+` | Unexpected error | Log and escalate. |

## Examples

### List all feature IDs

```bash
specscore feature list
# cli
# cli/code
# cli/code/deps
# cli/feature
# cli/feature/deps
# cli/feature/info
# ...
```

### List with status and outstanding-questions count

```bash
specscore feature list --fields=status,oq
```

```yaml
- id: cli
  status: In Progress
  oq: 3
- id: cli/feature
  status: Stable
  oq: 2
- id: cli/feature/list
  status: Stable
  oq: 1
```

### Pipe to other tools

```bash
# Count features
specscore feature list | wc -l

# Find features matching a pattern
specscore feature list | grep '^cli/'

# Pick a feature interactively
specscore feature list | fzf
```

## Notes

- This is a **read-only** command — it never mutates state.
- Without `--fields`: plain text, one feature ID per line, sorted alphabetically, no headers, no trailing blank line.
- With `--fields`: output auto-switches to YAML with the requested metadata inline. `--format text --fields …` is auto-upgraded to YAML rather than rejected.
- Both parent features (`cli`) and their children (`cli/feature`) appear as separate entries. Use [`feature tree`](tree.md) for a hierarchical view.
- The MVP does not support server-side filtering. Filter with `grep` on text output or `yq` on structured output.
