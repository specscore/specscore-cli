# Change a Feature's status

Wraps `specscore feature change-status` — transitions a Feature artifact's `**Status:**` field, validates the transition against the Feature legal-transition matrix, and runs `specscore spec lint --fix` to keep the features-index row in sync. No file relocation side effects.

**Authoritative CLI reference:** [`cli/feature/change-status`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/feature/change-status/README.md)

**Sibling reference inside the navigation-style skill:** [`skills/feature/references/change-status.md`](../../feature/references/change-status.md) — identical CLI contract, more navigation-oriented framing.

## When to use

- **Walk a Feature through its lifecycle** — `Draft → Under Review → Approved → Implementing → Stable → Deprecated`.
- **Skip review** when a Feature is simple enough to go `Draft → Approved` directly.
- **Mark a Feature as Stable** when its implementation has landed and is no longer changing materially.
- **Deprecate a Feature** that is being superseded or removed.

## Command

```bash
specscore feature change-status <feature_id> --to=<status> \
  [--project <path>]
```

## Parameters

| Name / Flag | Required | Description |
|---|---|---|
| `<feature_id>` | Yes | Feature identifier — identifies `spec/features/<feature_id>/README.md`. Slashes allowed for nested features (e.g., `cli/idea/change-status`). |
| `--to` | Yes | Target status. Legal values: `under review`, `approved`, `implementing`, `stable`, `deprecated` (case-insensitive). `draft` is **not** a legal target. Multi-word values must be shell-quoted: `--to="under review"`. |
| `--project` | No | Project root. Autodetected from `cwd` if omitted. |

## Legal transitions

| From | To | Side effects |
|---|---|---|
| `Draft` | `Under Review` | Status rewrite + features-index sync |
| `Draft` | `Approved` | Status rewrite + features-index sync |
| `Under Review` | `Approved` | Status rewrite + features-index sync |
| `Approved` | `Implementing` | Status rewrite + features-index sync |
| `Implementing` | `Stable` | Status rewrite + features-index sync |
| `Stable` | `Deprecated` | Status rewrite + features-index sync |

Any other `(from, to)` pair exits `4` (InvalidTransition). The state machine is strict, not idempotent — `Draft → Approved` is the only legal skip-step path.

## Exit codes

| Code | Meaning | What to surface to the caller |
|---|---|---|
| `0` | Transition succeeded; features-index synced | The single-line stdout `<feature_id>: <from> → <to>` — relayed byte-for-byte. |
| `2` | Missing/malformed `<feature_id>`, missing `--to`, or unrecognized `--to` value | The verbatim stderr from the CLI. Legal `--to` set: `{under review, approved, implementing, stable, deprecated}`. |
| `3` | No Feature directory or README at `spec/features/<feature_id>/` | The verbatim stderr. Suggest `specscore feature list` to confirm the ID. |
| `4` | `(current_status, --to)` is not a legal transition | The verbatim stderr. Cite the matrix above; check current status via `specscore feature info <feature_id>`. |
| `10` | I/O failure or `spec lint --fix` failed after a successful rewrite (rollback applied) | The verbatim stderr. On-disk state is pre-invocation; do **not** retry automatically. |

## Output

On exit `0`, exactly one line on stdout:

```
<feature_id>: <from-status> → <to-status>
```

(Unicode `→`, not `->`.) stdout is empty on every non-zero exit; explanations go to stderr. The skill MUST relay this single line **byte-for-byte** to the caller — no paraphrasing, no augmentation.

## No silent fallback

If the CLI exits non-zero for any reason, surface the exit code and the verbatim stderr to the caller. Do **not**:

- Edit the `**Status:**` line in `spec/features/<feature_id>/README.md` directly.
- Re-run `specscore spec lint --fix` in place of the verb.
- Drop the failure and retry with a different `--to`.

The dedicated-skill contract is "CLI or stop" — the entire point of this skill is to keep that contract.

## Examples

### Walk a Feature through review

```bash
specscore feature change-status auth --to="under review"
# auth: Draft → Under Review

specscore feature change-status auth --to=approved
# auth: Under Review → Approved

specscore feature change-status auth --to=implementing
# auth: Approved → Implementing

specscore feature change-status auth --to=stable
# auth: Implementing → Stable
```

### Skip review

```bash
specscore feature change-status simple-toggle --to=approved
# simple-toggle: Draft → Approved
```

### Illegal transition

```bash
specscore feature change-status auth --to=stable
# Exit 4: invalid transition Draft → Stable
#   (legal targets from Draft: Under Review, Approved)
```
