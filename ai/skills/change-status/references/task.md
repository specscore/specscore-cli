# Change a Task's status

Wraps `specscore task change-status` — transitions a Task's `**Status:**` field, validates the transition against the Task legal-transition matrix, and runs `specscore spec lint --fix` to keep the matching index/inline row in sync. Optionally stamps implementation-commit provenance on completion.

**Authoritative CLI reference:** [`cli/task/change-status`](https://github.com/specscore/specscore-cli/blob/main/spec/features/cli/task/change-status/README.md)

## When to use

- **Walk a Task through its lifecycle** — `planning → queued → in_progress → blocked → complete` (or `failed` / `aborted`).
- **Complete a task** when its implementation has landed — optionally stamping the implementation commit that satisfied it.
- **Mark a task blocked** when it cannot proceed.
- **Re-stamp provenance** on an already-`complete` task whose commit/branch metadata was wrong or missing (`--amend-provenance`), without a status transition.

## Command

```bash
specscore task change-status <task> --to=<status> \
  [--plan <slug>] \
  [--repo <repo>] [--commit <sha>] [--branch <branch>] \
  [--amend-provenance] \
  [--project <path>]
```

## Canonical statuses

`planning`, `queued`, `in_progress`, `blocked`, `complete`, `failed`, `aborted`.

## Target resolution (two modes)

A Task lives in one of two places, and `<task>` is resolved differently per mode:

- **Board mode** (default) — the Task is a directory at `tasks/<task>/`. `<task>` is the board task identifier.
- **Plan-inline mode** (`--plan <slug>`) — the Task is a task block inside `spec/plans/<slug>.md`. `<task>` resolves against each task block's `**Id:**` field, not a filesystem path.

## Parameters

| Name / Flag | Required | Description |
|---|---|---|
| `<task>` | Yes | Task identifier. In board mode, identifies `tasks/<task>/`. In plan-inline mode (`--plan`), matched against the task block's `**Id:**`. |
| `--to` | Yes | Target status. Legal values: `planning`, `queued`, `in_progress`, `blocked`, `complete`, `failed`, `aborted` (case-insensitive). |
| `--plan` | No | Plan slug. Switches to plan-inline mode, resolving `<task>` against task-block `**Id:**` fields in `spec/plans/<slug>.md`. |
| `--repo` | No | Provenance: implementation repository. Valid **only** with `--to=complete` (or `--amend-provenance`). |
| `--commit` | No | Provenance: implementation commit sha. Valid **only** with `--to=complete` (or `--amend-provenance`). |
| `--branch` | No | Provenance: implementation branch. Valid **only** with `--to=complete` (or `--amend-provenance`). |
| `--amend-provenance` | No | Corrective re-stamp of provenance on an already-`complete` task **without** a status transition. Mutually exclusive with `--to`. |
| `--project` | No | Project root. Autodetected from `cwd` if omitted. |

## Implementation-commit provenance

The `--repo` / `--commit` / `--branch` flags are **optional** and valid **only** when completing a task (`--to=complete`) — or correcting an already-complete task via `--amend-provenance`. They record the implementation commit that satisfied the task.

- **Cross-repo:** assembled as `<repo>@<sha> (<branch>)`.
- **Same-repo:** a bare `<sha>` (no `<repo>@` prefix) when the commit lives in the spec repo itself.
- `--commit` is the anchor — `--repo` or `--branch` without `--commit` is an argument error (exit `2`).

### Corrective re-stamp (`--amend-provenance`)

`--amend-provenance` overwrites (or, with no provenance flags, clears) the provenance on a task that is **already** `complete`, without performing a status transition. It is mutually exclusive with `--to`:

- Passing both `--amend-provenance` and `--to` is an argument error (exit `2`).
- Running `--amend-provenance` against a task that is **not** `complete` is an illegal operation (exit `4`).

## Exit codes

Inherits the [SpecScore CLI shared exit-code contract](https://github.com/specscore/specscore-cli/blob/main/spec/features/cli/task/change-status/README.md).

| Code | Meaning | What to surface to the caller |
|---|---|---|
| `0` | Transition (or amend) succeeded; index/inline row synced | The single-line stdout `<task>: <from> → <to>` — relayed byte-for-byte. |
| `2` | Invalid args: missing/malformed `<task>`, missing/unrecognized `--to`, a provenance flag without `--to=complete`, a provenance flag (`--repo`/`--branch`) without `--commit`, or `--amend-provenance` combined with `--to` | The verbatim stderr. |
| `3` | Task not found — no `tasks/<task>/` (board mode) or no task block with matching `**Id:**` in `spec/plans/<slug>.md` (plan-inline mode) | The verbatim stderr. |
| `4` | Illegal transition `(current_status, --to)`, or `--amend-provenance` on a task that is not `complete` | The verbatim stderr. Check current status before retrying. |
| `10` | I/O failure during rewrite or `spec lint --fix` failed after a successful rewrite (rollback applied) | The verbatim stderr. On-disk state is pre-invocation; do **not** retry automatically. |

## Output

On exit `0`, exactly one line on stdout:

```
<task>: <from-status> → <to-status>
```

(Unicode `→`, not `->`.) stdout is empty on every non-zero exit; explanations go to stderr. The skill MUST relay this single line **byte-for-byte** to the caller — no paraphrasing, no augmentation.

## No silent fallback

If the CLI exits non-zero for any reason, surface the exit code and the verbatim stderr to the caller. Do **not**:

- Edit the `**Status:**` line of `tasks/<task>/` or a plan task block directly.
- Hand-edit provenance into a task file in place of `--repo`/`--commit`/`--branch`.
- Re-run `specscore spec lint --fix` in place of the verb.

The dedicated-skill contract is "CLI or stop" — never hand-edit task files to work around a failed verb.

## Examples

### Walk a board task to completion

```bash
specscore task change-status auth-login --to=in_progress
# auth-login: queued → in_progress

specscore task change-status auth-login --to=complete
# auth-login: in_progress → complete
```

### Complete with cross-repo provenance

```bash
specscore task change-status auth-login --to=complete \
  --repo specscore/backend --commit 9f3a1c2 --branch feat/login
# auth-login: in_progress → complete
#   provenance: specscore/backend@9f3a1c2 (feat/login)
```

### Complete a plan-inline task (same-repo provenance)

```bash
specscore task change-status T-003 --plan auth-rework --to=complete \
  --commit 9f3a1c2 --branch feat/login
# T-003: in_progress → complete
#   provenance: 9f3a1c2
```

### Correct provenance on an already-complete task

```bash
specscore task change-status auth-login --amend-provenance \
  --commit 1234abc --branch feat/login-fix
# auth-login: provenance re-stamped
```

### Illegal: provenance flag without --to=complete

```bash
specscore task change-status auth-login --to=blocked --commit 9f3a1c2
# Exit 2: --commit is valid only with --to=complete
```
