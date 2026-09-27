# specscore task new

Create a new task: writes `tasks/<slug>/README.md` with required sections **and** appends a row to the board (`tasks/README.md`) atomically. Always created in `planning` status — there is no `--status` flag.

**CLI reference:** [`cli/task/new`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/task/new/README.md)

## When to use

- **Seeding the board** with a new task
- **Bulk import** (loop over a list of titles in a script)
- **Plan → Task** step in an agent workflow when the planner has decided a chunk of work is ready to track

This command is **not** for transitioning state. Status changes (claim/release/complete) are out of scope for the MVP — see the [parent group's scope rule](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/task/README.md#req-no-lifecycle-in-mvp).

## Command

```bash
specscore task new \
  --task <slug> \
  --title <text> \
  [--description <text>] \
  [--depends-on <slugs>] \
  [--format <yaml|json>] \
  [--project <path>]
```

## Parameters

| Flag | Required | Description |
|---|---|---|
| `--task` | Yes | Task slug — lowercase, hyphen-separated, URL-safe. Becomes the directory name. |
| `--title` | Yes | Human-readable title shown on the board and in `tasks/<slug>/README.md`. |
| `--description` | No | Short description placed on the board row and in the README. |
| `--depends-on` | No | Comma-separated list of existing task slugs the new task depends on. Each MUST resolve. |
| `--format` | No | Output format: `yaml` (default) or `json`. |
| `--project` | No | Project root path. Autodetected from `cwd` if omitted. |

## Exit codes

| Code | Meaning | What to do |
|---|---|---|
| `0` | Task created (file + board row written atomically) | Parse the output for the new path. |
| `1` | Slug collision — `tasks/<slug>/` directory or board row already exists | Pick a different slug; there is no `--force`. |
| `2` | Missing `--task`/`--title`, invalid flag value, or bad slug format | Check arguments. |
| `3` | `--depends-on` names a non-existent task | Verify with [`task list`](list.md); no file is written on validation failure. |
| `10+` | Unexpected I/O failure | If a partial write occurred the command's atomicity guarantee triggers a rollback. Inspect the working tree before retrying. |

## Examples

### Minimal create

```bash
specscore task new \
  --task ship-onboarding \
  --title "Ship onboarding flow"
```

### With description and dependencies

```bash
specscore task new \
  --task ship-onboarding \
  --title "Ship onboarding flow" \
  --description "Set up the welcome screen and first-run survey." \
  --depends-on "design-mocks-approved"
```

### Catch a collision

```bash
specscore task new --task ship-onboarding --title "Duplicate"
# Exit 1: slug collision — `tasks/ship-onboarding/` already exists.
# (no state mutated)
```

## Notes

- **Status is fixed to `planning`.** No flag overrides this. Other statuses are out of scope for this command — see the parent group's [no-lifecycle-in-mvp](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/task/README.md#req-no-lifecycle-in-mvp) rule.
- **Atomic write:** the new README and the new board row are written as a pair. A failure on either rolls the working tree back to the pre-call state — no orphan files, no orphan rows.
- **No `--force`:** collisions exit `1` rather than overwriting. Pick a different slug.
- **Pre-write validation:** invalid slugs, missing required flags, and unresolvable `--depends-on` IDs all exit before any file is touched.
- **Slug format:** lowercase, hyphen-separated, URL-safe (`[a-z0-9-]`). Underscores and uppercase characters exit `2`.
- **No `--commit`/`--push` parity with `feature new`** in MVP — tracked in [open questions](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/task/new/README.md#outstanding-questions).
