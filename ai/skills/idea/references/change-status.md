# specscore idea change-status

Transition an Idea artifact from its current `**Status:**` to the target named by `--to`. The verb validates the transition against the Idea legal-transition matrix, rewrites the status line, and (for `--to=archived`) moves the file to `spec/ideas/archived/<slug>.md`. The post-mutation `specscore spec lint --fix` keeps the active and archived index rows in sync.

**CLI reference:** [`cli/idea/change-status`](https://github.com/specscore/specscore-cli/blob/main/spec/features/cli/idea/change-status/README.md)

## Lifecycle-transitions contract

This verb mutates the Idea kind's `**Status:**` field. The [lifecycle-transitions contract](https://github.com/specscore/specscore-cli/blob/main/spec/features/cli/lifecycle-transitions/README.md) governs which transitions are valid. `specscore` is the canonical path for Idea lifecycle transitions.

## When to use

- **Promote an Idea from `Draft` to `Approved`** after a successful review pass
- **Archive an Idea** that has been superseded, abandoned, or otherwise removed from active consideration — covers `Draft`, `Under Review`, `Approved`, `Specifying`, `Specified`, `Implementing`, and `Implemented` source states
- **Replace a hand-edit** of the `**Status:**` line that previously required a follow-up `specscore spec lint --fix` to keep the index row consistent

## Command

```bash
specscore idea change-status <slug> --to=<status> \
  [--project <path>]
```

## Parameters

| Name / Flag | Required | Description |
|---|---|---|
| `<slug>` | Yes | Idea slug — identifies the active file at `spec/ideas/<slug>.md`. An already-archived file at `spec/ideas/archived/<slug>.md` does NOT satisfy the lookup. |
| `--to` | Yes | Target status. Legal values: `Approved`, `Archived`, `Implemented`, `Implementing`, `Specified`, `Specifying` (case-insensitive). |
| `--project` | No | Project root path. Autodetected from `cwd` if omitted. |

## Legal transitions

| From | To | Side effects |
|---|---|---|
| `Draft` | `Approved`, `Archived` | Status rewrite + ideas-index sync (`→ Archived`: + file move + active-index + archived-index sync) |
| `Under Review` | `Archived` | Status rewrite + file move + active-index + archived-index sync |
| `Approved` | `Specifying`, `Archived` | Status rewrite + ideas-index sync (`→ Archived`: + file move + active-index + archived-index sync) |
| `Specifying` | `Specified`, `Archived` | Status rewrite + ideas-index sync (`→ Archived`: + file move + active-index + archived-index sync) |
| `Specified` | `Implementing`, `Archived` | Status rewrite + ideas-index sync (`→ Archived`: + file move + active-index + archived-index sync) |
| `Implementing` | `Implemented`, `Archived` | Status rewrite + ideas-index sync (`→ Archived`: + file move + active-index + archived-index sync) |
| `Implemented` | `Archived` | Status rewrite + file move + active-index + archived-index sync |

Any other `(from, to)` pair exits `4` (InvalidTransition). The state machine is strict — re-running with the current status as `--to` is rejected, not silently skipped.

`Specifying`, `Specified`, `Implementing`, and `Implemented` are **both auto-derived AND legal manual `--to` targets**. The idea status is auto-derived from referenced Features (and `specscore spec lint --fix` reconciles drift):

- no Feature references → `Approved`
- 1+ refs, any referenced Feature at `Draft`/`Under Review` → `Specifying`
- 1+ refs, every referenced Feature at `Approved` → `Specified`
- 1+ refs, any referenced Feature at `Implementing` → `Implementing`
- 1+ refs, every referenced Feature at `Stable` → `Implemented`

## Exit codes

| Code | Meaning | What to do |
|---|---|---|
| `0` | Transition succeeded; indexes synced | Parse the single-line stdout `<slug>: <from> → <to>`. |
| `1` | Archive collision: `spec/ideas/archived/<slug>.md` already exists | Choose a different slug, or move the stale archived file aside. The active file was rolled back to its original status. |
| `2` | Missing or malformed `<slug>`, missing `--to`, or unrecognized `--to` value | Check arguments; the legal `--to` set is `{Approved, Archived, Implemented, Implementing, Specified, Specifying}`. |
| `3` | No Idea file at `spec/ideas/<slug>.md` | Verify the slug; try [`idea new`](new.md) if it doesn't exist yet. |
| `4` | `(current_status, --to)` is not a legal transition | Read the matrix above; check the current status with `head spec/ideas/<slug>.md`. |
| `10` | I/O failure during rewrite, file move, or `spec lint --fix` failed after a successful rewrite (rollback applied) | Inspect stderr for the underlying lint or I/O error. The on-disk state is the pre-invocation state. |

## Output

On exit `0`, exactly one line is written to stdout:

```
<slug>: <from-status> → <to-status>
```

(Unicode arrow `→`, not `->`.) stdout is empty on every non-zero exit; explanations go to stderr.

## Examples

### Promote Draft to Approved

```bash
specscore idea change-status offline-mode --to=approved
# offline-mode: Draft → Approved
```

### Archive an abandoned Idea

```bash
specscore idea change-status offline-mode --to=archived
# offline-mode: Approved → Archived
# spec/ideas/offline-mode.md → spec/ideas/archived/offline-mode.md
```

### Case-insensitive flag

```bash
specscore idea change-status offline-mode --to=APPROVED
# Behaves identically to --to=approved; the file is written with
# canonical title-case ("**Status:** Approved").
```

### Re-running on the target state (illegal)

```bash
specscore idea change-status offline-mode --to=approved
# Already at Approved.
# Exit 4: invalid transition Approved → Approved (legal targets from Approved: Specifying, Archived)
```

## Notes

- **Strict state machine, not idempotent.** Re-running with the current status as `--to` exits `4`, not `0`. Idempotent callers should read the status first (e.g., parse `head -20 spec/ideas/<slug>.md` for the `**Status:**` line, or use [`spec lint`](../../spec/references/lint.md) output).
- **Archive side effect (`--to=archived`).** The file is moved from `spec/ideas/<slug>.md` to `spec/ideas/archived/<slug>.md`. The `archived/` directory is created if absent, and (if missing) a minimal lint-clean `archived/README.md` stub is materialized on first archive. Rollback covers status, relocation, AND the stub.
- **No `--reason` / `--message` flag in MVP.** Capture the rationale in the git commit body or in the Idea body itself; the Idea spec's `## Outstanding Questions` may evolve to accept structured rationale in a future revision.
- **Lint sync is full-tree.** Any pre-existing error-severity lint violation elsewhere in the spec tree will trigger rollback, not just violations introduced by this transition. Run `specscore spec lint` first if the tree is in an unknown state.
- **Slugs containing slashes are not Ideas.** Ideas live at the flat path `spec/ideas/<slug>.md`. Slash-bearing identifiers refer to Features; use [`feature change-status`](../../feature/references/change-status.md) instead.
