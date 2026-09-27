# specscore feature change-status

Transition a Feature artifact from its current `**Status:**` to the target named by `--to`. The verb validates the transition against the Feature legal-transition matrix, rewrites the status line, and runs `specscore spec lint --fix` to keep the features-index row in sync.

Unlike the Idea-kind sibling, this verb has no file-relocation side effects — every transition is a pure status rewrite plus index sync.

**CLI reference:** [`cli/feature/change-status`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/feature/change-status/README.md)

## Synchestra-presence pre-flight

This verb mutates the Feature kind's `**Status:**` field. The [lifecycle-transitions contract](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/lifecycle-transitions/README.md) requires that when both `specscore` and a corresponding Synchestra command are installed on the user's machine, this skill SHOULD prefer the Synchestra command for that doc kind. Today **no Synchestra equivalent exists for Feature lifecycle**, so `specscore` is unconditionally the canonical path. If `synchestra feature change-status` appears in a future Synchestra release, update this pre-flight block to route to it when present.

## When to use

- **Walk a Feature through its full lifecycle** — `Draft → Under Review → Approved → Implementing → Stable → Deprecated`
- **Skip the review phase** when a Feature is simple enough to go `Draft → Approved` directly
- **Mark a Feature as Stable** when its implementation has landed and is no longer changing materially
- **Deprecate a Feature** that is being superseded or removed from the platform
- **Replace a hand-edit** of the `**Status:**` line that previously required a follow-up `specscore spec lint --fix` to keep the index row consistent

## Command

```bash
specscore feature change-status <feature_id> --to=<status> \
  [--project <path>]
```

## Parameters

| Name / Flag | Required | Description |
|---|---|---|
| `<feature_id>` | Yes | Feature identifier — identifies `spec/features/<feature_id>/README.md`. Slashes are allowed for nested features (e.g., `cli/idea/change-status`). |
| `--to` | Yes | Target status. Legal values: `under review`, `approved`, `implementing`, `stable`, `deprecated` (case-insensitive). `draft` is NOT a legal target — no transition lands INTO `Draft`. Multi-word values must be shell-quoted: `--to="under review"`. |
| `--project` | No | Project root path. Autodetected from `cwd` if omitted. |

## Legal transitions

| From | To | Side effects |
|---|---|---|
| `Draft` | `Under Review` | Status rewrite + features-index sync |
| `Draft` | `Approved` | Status rewrite + features-index sync |
| `Under Review` | `Approved` | Status rewrite + features-index sync |
| `Approved` | `Implementing` | Status rewrite + features-index sync |
| `Implementing` | `Stable` | Status rewrite + features-index sync |
| `Stable` | `Deprecated` | Status rewrite + features-index sync |

The `Draft → Approved` direct path is permitted — not every Feature requires a review phase. Any other `(from, to)` pair (reverse transitions, skip-step transitions, or same-target re-runs) exits `4` (InvalidTransition). The state machine is strict, not idempotent.

`feature undeprecate` (`Deprecated → Stable`) and `Approved → Draft` rollback paths are **deferred** until a real reuse pattern surfaces.

## Exit codes

| Code | Meaning | What to do |
|---|---|---|
| `0` | Transition succeeded; features-index synced | Parse the single-line stdout `<feature_id>: <from> → <to>`. |
| `2` | Missing or malformed `<feature_id>`, missing `--to`, or unrecognized `--to` value | Check arguments; the legal `--to` set is `{under review, approved, implementing, stable, deprecated}`. |
| `3` | No Feature directory or README at `spec/features/<feature_id>/` | Verify the ID with [`feature list`](list.md). |
| `4` | `(current_status, --to)` is not a legal transition | Read the matrix above; check the current status with [`feature info`](info.md). |
| `10` | I/O failure or `spec lint --fix` failed after a successful rewrite (rollback applied) | Inspect stderr for the underlying lint or I/O error. The on-disk state is the pre-invocation state. |

## Output

On exit `0`, exactly one line is written to stdout:

```
<feature_id>: <from-status> → <to-status>
```

(Unicode arrow `→`, not `->`.) stdout is empty on every non-zero exit; explanations go to stderr.

## Examples

### Walk a Feature through review

```bash
specscore feature change-status auth --to="under review"
# auth: Draft → Under Review

specscore feature change-status auth --to=approved
# auth: Under Review → Approved
```

### Direct Draft → Approved (no review phase)

```bash
specscore feature change-status auth --to=approved
# auth: Draft → Approved
```

### Mark a Feature as Stable

```bash
specscore feature change-status auth --to=implementing
# auth: Approved → Implementing

# ...later, after implementation lands:
specscore feature change-status auth --to=stable
# auth: Implementing → Stable
```

### Nested feature id

```bash
specscore feature change-status cli/idea/change-status --to=approved
# cli/idea/change-status: Draft → Approved
```

Slashes in `<feature_id>` resolve as a path within `spec/features/`.

### Deprecate a Feature

```bash
specscore feature change-status legacy-auth --to=deprecated
# legacy-auth: Stable → Deprecated
```

### Illegal skip-step (rejected)

```bash
specscore feature change-status auth --to=stable
# Current status is Draft.
# Exit 4: invalid transition Draft → Stable (legal targets from Draft: Under Review, Approved)
```

## Notes

- **Strict state machine, not idempotent.** Re-running with the current status as `--to` exits `4`. Idempotent callers should read the status first via [`feature info`](info.md).
- **No file relocation.** Unlike Idea archival, every Feature transition is a pure rewrite — the Feature's directory location does not change.
- **No `--reason` / `--message` flag in MVP.** Particularly for `--to=deprecated`, where a successor reference would aid migration, capture the rationale in the git commit body. A future revision may add structured rationale capture.
- **Lint sync is full-tree.** Any pre-existing error-severity lint violation elsewhere in the spec tree will trigger rollback, not just violations introduced by this transition. Run `specscore spec lint` first if the tree is in an unknown state.
- **Index row format.** The new row is in the 4-cell `Feature | Status | Kind | Description` shape consumed by the `feature-index-row-sync` lint rule. Kind is hand-maintained — the verb only syncs the Status cell. If Kind for your feature needs adjustment, hand-edit `spec/features/README.md` directly.
- **`--to=draft` is rejected.** No transition lands INTO `Draft`. Calls naming `--to=draft` exit `2` (InvalidArgs) at the flag-validation layer, BEFORE the state-machine check.
