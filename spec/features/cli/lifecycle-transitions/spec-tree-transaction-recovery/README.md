---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: Spec tree transaction recovery

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lifecycle-transitions/spec-tree-transaction-recovery?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lifecycle-transitions/spec-tree-transaction-recovery?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lifecycle-transitions/spec-tree-transaction-recovery?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lifecycle-transitions/spec-tree-transaction-recovery?op=request-change) |
**Status:** Draft
**Source Ideas:** —

## Summary

Crash-safe copy-on-write publication and explicit recovery for lifecycle mutations spanning a declared Spec tree.

## Problem

Some lifecycle commands derive changes to indexes, links, or other artifacts
after changing their primary artifact. A process crash between those writes can
leave a valid primary mutation beside stale derived state, while a late
"rollback" can be worse: it can overwrite a concurrent writer with an old tree.

PR #100 explored a copy-on-write transaction for the entire `spec/` tree. That
work contains valuable platform, crash-boundary, and recovery machinery, but it
predates the current lifecycle contract: transaction-profile commands retain a
committed primary mutation and report recovery required; they never restore a
stale preimage. This Feature reconciles those designs before code is ported so
SpecScore has one transaction identity and one recovery truth instead of two
competing systems.

## Behavior

### Declared transaction boundary

#### REQ: declared-write-set

A command using a Spec-tree transaction MUST declare the artifact paths it can
publish before mutation begins. Whole-tree staging MAY be used as an
implementation boundary, but it MUST NOT silently broaden the command's product
contract: a staged change outside the declared write set is a conflict and
MUST NOT be published.

The transaction engine is initially available only to commands that explicitly
opt in because they publish multiple related artifacts. It MUST NOT replace the
single-artifact CAS path for every lifecycle verb merely because the engine
exists.

#### REQ: no-null-action-artifacts

When validation rejects a command before it mutates any artifact, the live
`spec/` tree remains byte-identical and no transaction receipt, staging tree,
predecessor tree, or adjacent untracked lock file is left behind. Persistent
cooperating-writer lock identities are operational state: initialized projects
MUST ignore `**/.*.lifecycle-transaction.lock` so successful scaffolding and
lifecycle commands do not dirty source control.

### Staging and publication

#### REQ: descriptor-anchored-staging

On supported platforms, the engine MUST open the project and live `spec/` tree
without following symbolic links, create a sibling staging tree, and bind each
security-sensitive operation to the opened filesystem identity rather than
re-resolving an attacker-replaceable path. If those guarantees are unavailable,
the engine fails closed before invoking the mutation callback.

#### REQ: baseline-and-stage-validation

Before publication, the engine MUST prove both that the live tree still matches
the captured baseline and that the staging tree still matches the mutation
result it validated. A non-cooperating writer, replaced directory, changed
mount, or modified stage is a conflict. The command MUST leave the writer's live
bytes untouched and retain enough evidence to diagnose any uncertain state.

#### REQ: atomic-tree-publication

Publication MUST use an atomic directory exchange between the live tree and the
validated stage. The predecessor tree remains held under its transaction
identity until the committed receipt is durable. A platform without a proven
atomic exchange primitive MUST reject the operation rather than emulate it with
a sequence of renames that exposes a partial tree.

#### REQ: no-late-stale-rollback

Once the new tree is atomically published, a later derived-work or event-delivery
failure MUST NOT automatically exchange the predecessor back into place. The
command retains the committed tree and reports the current lifecycle contract's
typed recovery-required outcome. Any future operator-requested restore is a new,
explicit, concurrency-checked operation; it is not an error handler.

### Receipt and event identity

#### REQ: one-transaction-identity

The tree transaction, lifecycle mutation, event/outbox record, and recovery
receipt MUST share one immutable transaction UUID. The recovery subsystem MUST
extend the existing event/outbox transaction record or link to it bijectively;
it MUST NOT create a second ledger that can independently claim a different
outcome for the same mutation.

#### REQ: durable-receipt-state-machine

Before each irreversible boundary, the engine durably advances a receipt
through `prepared`, `publishing`, and `committed`. If a crash prevents the engine
from proving which side of publication completed, the receipt is `uncertain`.
Receipts include the opened tree identities and content/metadata manifests
needed to distinguish live, predecessor, and tampered trees after restart.
State transitions are monotonic and replaying recovery inspection is read-only.

### Fidelity and recovery

#### REQ: metadata-fidelity

The staged tree and retained predecessor MUST preserve each supported entry's
type, bytes, permission bits, modification time, and extended attributes. The
implementation MUST define and test the contract for platform-specific metadata
before enabling publication on that platform. In particular, macOS file flags
(`st_flags`) are not silently omitted. The Darwin implementation captures them,
accepts an identical inherited value, otherwise applies them through the held
descriptor, and re-verifies the exact value before publication. If the platform
refuses that preservation, the whole-tree publisher fails closed.

#### REQ: read-only-recovery-first

The first recovery CLI surface is read-only: list receipts, inspect their state,
and diff live/staged/predecessor trees. Inspection MUST NOT mutate trees, advance
receipt state, delete evidence, or infer that an ambiguous publication is safe
to reverse. Destructive `apply`, `restore`, or `reclaim` verbs require a separate
approved contract.

#### REQ: explicit-retention-reclamation

The engine MUST bound retained transaction data without deleting the only copy
of an uncertain or unacknowledged predecessor. The first implementation retains
at most eight transaction identities and refuses a ninth before writing a new
receipt or invoking the mutation callback. It performs no automatic deletion.
Any future reclamation is explicit, names the transaction it affects, and
requires its own approved authorization and audit contract.

## Acceptance Criteria

### AC: rejected-command-leaves-no-artifacts

**Verifies:** `no-null-action-artifacts`

Given an invalid lifecycle transition in a freshly initialized git repository,
when the command is rejected, then `git status --short` reports no receipt,
stage, predecessor, or lifecycle-lock artifact and every tracked byte is
unchanged.

### AC: declared-multi-artifact-publication-is-atomic

**Verifies:** `declared-write-set`, `descriptor-anchored-staging`,
`baseline-and-stage-validation`, `atomic-tree-publication`

Given a command declaring a primary artifact and derived index, when both staged
changes validate and the live baseline is unchanged, then another process sees
either the complete old tree or the complete new tree, never a mixed pair, and
the committed receipt identifies the new live tree and retained predecessor.

### AC: undeclared-stage-change-refuses-publication

**Verifies:** `declared-write-set`, `baseline-and-stage-validation`

Given a mutation callback that changes a path outside its declared write set,
when the engine validates the stage, then it refuses publication, preserves the
live tree, and reports the unexpected path.

### AC: concurrent-raw-writer-is-preserved

**Verifies:** `baseline-and-stage-validation`, `no-late-stale-rollback`

Given a non-cooperating writer changes the live tree after the transaction
captures its baseline, when publication is attempted, then the transaction
reports conflict and neither overwrites the writer nor restores any preimage.

### AC: crash-boundaries-have-one-diagnosable-outcome

**Verifies:** `durable-receipt-state-machine`, `one-transaction-identity`

Given process termination after each durable receipt transition and immediately
before and after directory exchange, when recovery inspection runs, then it
reports `prepared`, `publishing`, `committed`, or `uncertain` from durable
evidence, links the same transaction UUID to its event/outbox record, and makes
no mutation while inspecting it.

### AC: committed-tree-survives-derived-failure

**Verifies:** `no-late-stale-rollback`, `one-transaction-identity`

Given atomic publication succeeds and later derived work or event delivery
fails, when the command returns, then the new tree remains live, the predecessor
remains retained, the command reports recovery required, and no automatic
exchange restores stale bytes.

### AC: identity-replacement-and-symlink-attacks-fail-closed

**Verifies:** `descriptor-anchored-staging`, `baseline-and-stage-validation`

Given the live or staged directory is replaced, moved to another mount, or
substituted with a symlink after opening, when the next transaction boundary is
checked, then publication is refused and the evidence identifies which bound
filesystem identity no longer matches.

### AC: metadata-round-trip-is-exact

**Verifies:** `metadata-fidelity`

Given files carrying non-default modes, modification times, extended attributes,
and supported platform flags, when a tree is staged, published, and inspected,
then both live and retained predecessor preserve the declared metadata exactly.
On a platform or entry whose metadata cannot be preserved, publication is
rejected before exchange.

### AC: recovery-inspection-is-repeatably-read-only

**Verifies:** `read-only-recovery-first`

Given prepared, committed, and uncertain receipts, when list, inspect, and diff
commands are repeated, then their output is stable for unchanged inputs and no
tree, receipt, retention timestamp, or event/outbox record changes.

### AC: retention-never-deletes-the-only-uncertain-copy

**Verifies:** `explicit-retention-reclamation`

Given eight retained committed or uncertain transaction identities, when a
ninth transaction is requested, then it is refused before creating a receipt or
invoking the mutation callback, and every existing predecessor remains
available for inspection.

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [Lifecycle Transitions](../README.md) | This Feature extends the shared transaction contract without changing its retain-committed-artifact rule. |
| [Event and outbox rollout](../../../../plans/artifact-lifecycle-outbox-rollout.md) | Tree publication and event delivery share one transaction identity and recovery outcome. |

## Open Questions

1. **How do adjacent locks migrate into `.specscore/locks/` without splitting
   mutual exclusion between old and new SpecScore CLI processes?** The current
   bounded fix ignores the existing adjacent identity files. A private namespace
   needs a versioned dual-lock or deployment-fence design before migration.
2. **When may an operator destructively restore or reclaim a retained tree?**
   The first recovery surface is deliberately list/inspect/diff only. Restore
   and reclaim need an approved concurrency, authorization, and audit contract.
3. **How does a future Plan-reconciliation event share the tree transaction
   identity?** `plan reconcile` does not currently emit an event/outbox record,
   so the receipt is the only durable transaction identity in this opt-in path.
   Before event emission is added, the callback API must expose the transaction
   ID and link the outbox record bijectively; a second independent outcome
   ledger is forbidden.

---
*This document follows the https://specscore.md/feature-specification*
