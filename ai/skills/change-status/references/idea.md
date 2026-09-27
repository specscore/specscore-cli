# Change an Idea's status

Wraps `specscore idea change-status` — transitions an Idea artifact's `**Status:**` field, and (for `--to=archived`) moves the file from `spec/ideas/<slug>.md` to `spec/ideas/archived/<slug>.md`. The post-mutation `specscore spec lint --fix` keeps the active-index and archived-index rows in sync.

**Authoritative CLI reference:** [`cli/idea/change-status`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/idea/change-status/README.md)

**Sibling reference inside the navigation-style skill:** [`skills/idea/references/change-status.md`](../../idea/references/change-status.md) — identical CLI contract, more navigation-oriented framing.

## When to use

- **Promote a Draft Idea to Approved** after a review pass.
- **Archive an Idea** that has been superseded or abandoned — legal from `Draft`, `Under Review`, `Approved`, `Specifying`, `Specified`, `Implementing`, or `Implemented`.

`Specifying`, `Specified`, `Implementing`, and `Implemented` are **both auto-derived AND legal manual `--to` targets**. The idea status is auto-derived from referenced Features, and `specscore spec lint --fix` reconciles drift. The derivation rules are:

- no Feature references → `Approved`
- 1+ refs, any referenced Feature at `Draft`/`Under Review` → `Specifying`
- 1+ refs, every referenced Feature at `Approved` → `Specified`
- 1+ refs, any referenced Feature at `Implementing` → `Implementing`
- 1+ refs, every referenced Feature at `Stable` → `Implemented`

See the Idea Feature spec for the full derivation rules.

## Command

```bash
specscore idea change-status <slug> --to=<status> \
  [--project <path>]
```

## Parameters

| Name / Flag | Required | Description |
|---|---|---|
| `<slug>` | Yes | Idea slug — identifies the active file at `spec/ideas/<slug>.md`. An already-archived file at `spec/ideas/archived/<slug>.md` does **not** satisfy the lookup. |
| `--to` | Yes | Target status. Legal values: `Approved`, `Archived`, `Implemented`, `Implementing`, `Specified`, `Specifying` (case-insensitive). |
| `--project` | No | Project root. Autodetected from `cwd` if omitted. |

Slugs containing slashes are **not** Ideas — those are Features. Use [references/feature.md](feature.md) instead.

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

## Exit codes

| Code | Meaning | What to surface to the caller |
|---|---|---|
| `0` | Transition succeeded; indexes synced | The single-line stdout `<slug>: <from> → <to>` — relayed byte-for-byte. |
| `1` | Archive collision: `spec/ideas/archived/<slug>.md` already exists | The verbatim stderr. The active file was rolled back to its original status. Choose a different slug or move the stale archived file aside. |
| `2` | Missing/malformed `<slug>`, missing `--to`, or unrecognized `--to` value | The verbatim stderr. Legal `--to` set: `{Approved, Archived, Implemented, Implementing, Specified, Specifying}`. |
| `3` | No Idea file at `spec/ideas/<slug>.md` | The verbatim stderr. Verify the slug; try `/specscore:idea` → `new` if it doesn't exist yet. |
| `4` | `(current_status, --to)` is not a legal transition | The verbatim stderr. Cite the matrix above. |
| `10` | I/O failure during rewrite, file move, or `spec lint --fix` failed after a successful rewrite (rollback applied) | The verbatim stderr. On-disk state is pre-invocation; do **not** retry automatically. |

## Output

On exit `0`, exactly one line on stdout:

```
<slug>: <from-status> → <to-status>
```

(Unicode `→`, not `->`.) The skill MUST relay this line **byte-for-byte** to the caller — no paraphrasing.

## No silent fallback

If the CLI exits non-zero for any reason, surface the exit code and the verbatim stderr to the caller. Do **not**:

- Edit the `**Status:**` line in `spec/ideas/<slug>.md` directly.
- Move the file by hand to `spec/ideas/archived/<slug>.md`.
- Re-run `specscore spec lint --fix` in place of the verb.

Direct edits bypass the legal-transition check, the archive-collision check, and the active/archived index reconciliation — the very guarantees this skill exists to enforce.

## Examples

### Approve a Draft Idea

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

### Illegal transition

```bash
specscore idea change-status offline-mode --to=approved
# Already at Approved.
# Exit 4: invalid transition Approved → Approved
#   (legal targets from Approved: Specifying, Archived)
```
