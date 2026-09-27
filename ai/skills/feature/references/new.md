# specscore feature new

Scaffold a new feature directory with a lint-clean README containing every required section. Optionally stages a git commit (`--commit`) or commits and pushes atomically (`--push`).

**CLI reference:** [`cli/feature/new`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/feature/new/README.md)

## When to use

- **Adding a new feature** to the spec hierarchy (top-level or under a parent)
- **You want lint-clean on first write** — the scaffolder emits the adherence footer, hub view link, OQ section, and required headings correctly
- **You want atomic git integration** — `--commit` or `--push` removes the two-step "edit + remember to commit" ritual
- **You're wiring a Plan → Feature step** in an agent workflow

## Command

```bash
specscore feature new \
  --title <text> \
  [--slug <slug>] \
  [--parent <feature_id>] \
  [--status <Draft|In Progress|Stable|Deprecated>] \
  [--description <text>] \
  [--depends-on <ids>] \
  [--commit | --push] \
  [--format <yaml|json|text>] \
  [--project <path>]
```

## Parameters

| Flag | Required | Description |
|---|---|---|
| `--title` | Yes | Human-readable feature title (e.g., `"Task Status Board"`). |
| `--slug` | No | Feature slug (directory name). Auto-derived from `--title` if omitted (lowercase, hyphen-separated, `[a-z0-9-]` only). |
| `--parent` | No | Parent feature ID for nesting (e.g., `cli/task`). |
| `--status` | No | Initial status: `Draft` (default), `In Progress`, `Stable`, or `Deprecated`. |
| `--description` | No | Short description placed in the Summary section. |
| `--depends-on` | No | Comma-separated list of existing feature IDs the new feature depends on. Each MUST resolve. |
| `--commit` | No | Stage and commit the new feature on the current branch. |
| `--push` | No | Implies `--commit`; additionally pushes to the remote. |
| `--format` | No | Output format: `yaml` (default), `json`, `text`. |
| `--project` | No | Project root path. Autodetected from `cwd` if omitted. |

## Exit codes

| Code | Meaning | What to do |
|---|---|---|
| `0` | Feature scaffolded (and optionally committed/pushed) | Parse the output for the new path; edit specific sections by line range. |
| `1` | `--push` failed because the remote has diverged | Pull, reconcile, retry. Local scaffolded files remain on disk. |
| `2` | Missing `--title`, invalid flag value, bad slug, or slug collision with an existing feature | Check arguments. |
| `3` | `--parent` or `--depends-on` names a non-existent feature | Verify with [`feature list`](list.md). |
| `10+` | Not a git repository (with `--commit`/`--push`), or unexpected I/O / git failure | Check git state; no working-tree mutation occurred on preflight failure. |

## Examples

### Top-level feature

```bash
specscore feature new \
  --title "Outbox Queue" \
  --description "Durable queue for outbound events." \
  --depends-on "state-store"
```

### Sub-feature under a parent

```bash
specscore feature new \
  --title "Claim Protocol" \
  --parent cli/task \
  --description "How agents claim tasks for exclusive work."
```

### With explicit slug

```bash
specscore feature new \
  --title "Outstanding Questions (OQ)" \
  --slug outstanding-questions \
  --status "In Progress"
```

### Atomic create + push

```bash
specscore feature new \
  --title "Sandbox Environment" \
  --description "Isolated execution environment for agent tasks." \
  --push
```

If the push hits a divergence the command exits `1` (Conflict); the scaffolded files remain so you can rebase and retry.

## Notes

- **Lint-clean guarantee:** the generated README passes [`spec lint`](../../spec/references/lint.md) immediately — including the adherence footer, hub view link (when `hub.host` is configured), OQ section, and structural conventions.
- **Slug derivation:** when `--slug` is omitted, the title is lowercased, whitespace and underscores become hyphens, characters outside `[a-z0-9-]` are dropped. Example: `"Outstanding Questions (OQ)"` → `outstanding-questions-oq`.
- **Pre-write validation:** `--depends-on` and `--parent` are validated *before* any file is written. A bad ID exits `3` with no side effects.
- **Status values are case-insensitive but spelled out:** `Draft`, `In Progress`, `Stable`, `Deprecated`. No `draft` shortcut.
- **Mutual exclusion:** `--commit` and `--push` are not contradictory (`--push` implies `--commit`), but if you want a non-pushing commit, use `--commit` alone.
- **Git preflight:** `--commit`/`--push` on a non-repo exits `10`; the working tree is left untouched.
- **Output is `feature info`-compatible:** parse `sections[].lines` for immediate follow-up edits without an extra [`feature info`](info.md) call.
