# specscore task info

Return detailed metadata for a single task: title, status, description, dependencies, and summary. Status comes from the board (`tasks/README.md`); the rest comes from `tasks/<slug>/README.md`.

**CLI reference:** [`cli/task/info`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/task/info/README.md)

## When to use

- **Picking up work:** Get the full task context in one call before starting
- **Triage:** Inspect description and dependencies without opening multiple files
- **Token efficiency:** Structured YAML/JSON instead of two README parses (board row + task body)

## Command

```bash
specscore task info \
  --task <slug> \
  [--format <yaml|json>] \
  [--project <path>]
```

## Parameters

| Flag | Required | Description |
|---|---|---|
| `--task` | Yes | Task slug to inspect. |
| `--format` | No | Output format: `yaml` (default) or `json`. No text format — task info is structured data. |
| `--project` | No | Project root path. Autodetected from `cwd` if omitted. |

## Exit codes

| Code | Meaning | What to do |
|---|---|---|
| `0` | Task found | Parse the YAML/JSON output. |
| `2` | Missing `--task` or invalid `--format` | Supply the slug; check format value. |
| `3` | No project found, slug not on the board, or `tasks/<slug>/README.md` missing | Verify with [`task list`](list.md). |
| `10+` | Unexpected I/O failure | Log and escalate. |

## Examples

### Inspect a task

```bash
specscore task info --task ship-onboarding
```

```yaml
slug: ship-onboarding
title: "Ship onboarding flow"
status: in_progress
description: "Set up the welcome screen and first-run survey."
dependencies:
  - design-mocks-approved
summary: |
  Done = first-run survey lands in analytics with the
  `onboarding_complete` event firing for >95% of new sessions.
```

### Single-field extraction

```bash
specscore task info --task ship-onboarding --format json | jq -r .status
# in_progress
```

### Use info → editor jump

```bash
slug=ship-onboarding
specscore task info --task "$slug" >/dev/null 2>&1 || exit 1
${EDITOR:-vi} "tasks/$slug/README.md"
```

## Notes

- **Read-only** — never mutates state.
- **Two data sources** are merged: the board row (authoritative `status`) and `tasks/<slug>/README.md` (everything else). If the slug exists as a directory but is *not* on the board, the command exits `3` — the board is the source of truth.
- Missing optional sections in the README render as empty fields, not as errors. `spec lint` is responsible for flagging absent required sections.
- `--task` is currently a flag (not positional). Tracked as an open question on the [parent group](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/task/README.md#outstanding-questions) for parity with `feature info`.
- For the full board view use [`task list`](list.md).
