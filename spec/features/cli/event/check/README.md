---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: Event Ledger Check Verb

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/event/check?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/event/check?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/event/check?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/event/check?op=request-change) |
**Status:** Stable
**Date:** 2026-09-10
**Owner:** alexandertrakhimenok
**Source Ideas:** —
**Supersedes:** —

## Summary

`specscore event check --base <git-ref>` is a ledger-monotonicity gate: it
proves the project's configured JSONL event ledger, as it exists in the
working tree, has not lost any event UUID that was present at `<base>`. The
event ledger is documented as immutable and append-only (see the parent
[`cli/event`](../README.md) Feature and `event merge`/`event merge-driver`),
but nothing previously *verified* that property held across an arbitrary
merge, rebase, or hand edit — a bad merge that silently truncated the ledger
produced a green CI run. This verb closes that gap.

## Problem

`event merge` and `event merge-driver` are the mechanisms that are SUPPOSED
to keep the ledger append-only during a merge, but a caller can still bypass
them: a manual `git merge` with no configured driver, a force-push, a
hand-edited file, or a bug in a totally different merge path all produce a
working tree that no existing command inspects for ledger regressions. There
was no cheap, scriptable way to ask "did the ledger I'm about to commit
still contain everything the base commit's ledger had?" — which is exactly
the question a CI gate on a pull request needs answered before merge.

## Behavior

### Verb registration

#### REQ: verb-registration

The CLI MUST register `specscore event check` as a cobra subcommand under
the `event` parent command (matching the parent `cli` Feature's
REQ:verb-subcommands). `specscore event check --help` MUST exit `0` and
print the verb's usage including the `--base` flag.

### Ledger monotonicity

#### REQ: ledger-not-shorter-than-base

`specscore event check --base <git-ref>` MUST:

1. Resolve the project's configured JSONL event ledger path via the parent
   Feature's `events:` configuration (the same resolution `event merge`
   uses — see `event.ConfiguredLedgerPath`).
2. Read the set of event UUIDs present in that ledger file AS COMMITTED AT
   `<git-ref>`, via `git show <git-ref>:<repo-relative-ledger-path>` (or
   equivalent), parsed with the same JSONL parsing/validation the parent
   Feature's merge logic uses.
3. Read the set of event UUIDs present in that ledger file AS IT EXISTS IN
   THE WORKING TREE right now (the literal file on disk — not the index,
   not HEAD).
4. Compare the two sets. If any UUID present in the `<git-ref>` set is
   ABSENT from the working-tree set, the command MUST exit non-zero and MUST
   print every such missing UUID so the caller can identify exactly which
   event(s) were lost — one UUID per line, to make the output trivially
   greppable. Otherwise it MUST exit `0`.

A `<git-ref>` that does not resolve to a commit in the repository MUST exit
`2` (invalid arguments) with a stderr message naming the ref. A ledger that
does not exist yet at `<git-ref>` (e.g. the ledger file was created after
that revision) MUST be treated as the empty set of base UUIDs — NOT an
error — since there is nothing recorded at that revision to have lost.

This command is read-only: it MUST NOT write to the ledger, the git
repository, or any other file, regardless of outcome.

## Architecture & Components

| Unit | Responsibility | Used by | Depends on |
|---|---|---|---|
| `internal/cli/event.go` (`eventCheckCommand`, `runEventCheck`) | Registers the verb; resolves `--base` and the ledger path; reads both UUID sets; reports the diff. | cobra root. | `pkg/event.ConfiguredLedgerPath`, `pkg/event.LedgerUUIDs`, `pkg/event.MissingUUIDs`, `pkg/gitremote.TopLevel`, `git show`/`git rev-parse` via `os/exec`. |
| `pkg/event.LedgerUUIDs` | Parses JSONL ledger bytes (working-tree or historical) into a UUID set, reusing the same `parseLedger` helper `MergeLedgers` uses — no independent JSONL parsing logic. | `runEventCheck`. | `pkg/event` internals (`parseLedger`). |
| `pkg/event.MissingUUIDs` | Set difference: UUIDs in `base` absent from `working`, sorted. | `runEventCheck`. | none. |

## Data Flow

```
$ specscore event check --base origin/main
  │
  ├─→ resolveEventProjectRoot() / event.ConfiguredLedgerPath()
  │     no jsonl sink configured → exit 2
  │
  ├─→ gitremote.TopLevel() → repo root; ledger path made repo-relative
  │
  ├─→ git rev-parse --verify --quiet <base>^{commit}
  │     does not resolve → exit 2, stderr names <base>
  │
  ├─→ git show <base>:<repo-relative-ledger-path>
  │     path absent at <base> → treated as zero base UUIDs (not an error)
  │
  ├─→ pkg/event.LedgerUUIDs(baseBytes) → baseUUIDs
  ├─→ os.ReadFile(workingTreeLedgerPath) → pkg/event.LedgerUUIDs(...) → workingUUIDs
  │
  └─→ pkg/event.MissingUUIDs(baseUUIDs, workingUUIDs)
        empty            → stdout "ok: ...", exit 0
        non-empty        → stderr: one UUID per line, exit 4 (invalid ledger state)
```

## Error Handling & Failure Modes

| Failure | Exit | Behavior |
|---|---|---|
| `--base` not supplied | `2` | Stderr states the flag is required. |
| `--base` does not resolve to a commit | `2` | Stderr names the ref and the repository root it was resolved against. |
| No jsonl event ledger configured (`events:` config ambiguous or absent a jsonl sink) | `2` | Delegated to the parent's `event.ConfiguredLedgerPath` contract (same as `event merge`). |
| `specscore.yaml` / project root not found | `3` | Delegated to the parent `cli` Feature's REQ:project-autodetect. |
| Ledger did not exist yet at `<base>` | `0` (success path, contributes zero base UUIDs) | NOT a failure — see REQ:ledger-not-shorter-than-base. |
| Working-tree ledger does not exist at all | `0` (success path, contributes zero working UUIDs) — UNLESS `<base>` had UUIDs, in which case every one of them is reported missing | Working-tree absence is not itself an error; it is evidence, the same as any other missing UUID. |
| One or more UUIDs present at `<base>` are absent from the working tree | `4` | Stderr lists every missing UUID, one per line, followed by a summary message. |
| Git repository or `git` binary unusable for reasons other than an unresolvable `--base` (e.g. `git show` fails on a resolved ref for an unexpected reason) | `10` | Stderr carries the underlying git error. |

## Testing Strategy

Unit tests in `internal/cli` cover flag validation (missing `--base`), an
unresolvable `--base`, a ledger absent at `--base`, a working tree that
retains every base UUID, and a working tree missing one or more base UUIDs
— using real temporary git repositories (matching the pattern in
`event_merge_driver_test.go`) since ref resolution is the entire point of
this command. `pkg/event` tests cover `LedgerUUIDs` and `MissingUUIDs`
directly, including malformed JSONL and the empty-input case.

## Rehearse Integration

| AC | Stub? | Rationale |
|---|---|---|
| `working-tree-retains-all-base-uuids-passes` | yes | Base and working-tree ledgers identical (or working tree is a superset); assert exit `0`. |
| `working-tree-missing-base-uuid-fails-nonzero` | yes | Working-tree ledger is missing a UUID present at `--base`; assert non-zero exit and that UUID appears in the output. |
| `unresolvable-base-fails-clearly` | yes | `--base` names a ref that does not exist; assert exit `2` and a stderr message naming the ref. |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [`cli/event`](../README.md) (parent) | Owns the JSONL ledger format, `events:` configuration, and the `parseLedger` validation this verb's `pkg/event.LedgerUUIDs` reuses. |
| `event merge` / `event merge-driver` (documented in [`docs/events.md`](../../../../../docs/events.md#merging-branch-ledgers) and [`docs/merge-drivers.md`](../../../../../docs/merge-drivers.md)) | The mechanisms meant to keep the ledger append-only during a merge. `event check` is the independent verification that those mechanisms (or any other path that touched the ledger) actually held — it does not call or depend on them. |
| [`cli`](../../../README.md) (root) | The `event` cobra subcommand attaches under root. Inherits the shared exit-code contract and `--project` autodetect. |
| Backstage CI (`sneat-co/backstage`) | The intended first consumer, wiring this verb into CI with the PR's base SHA. Out of scope of this Feature — this Feature specifies and implements the CLI command only. |

## Not Doing / Out of Scope

- **Wiring this command into any repository's CI.** Backstage's own CI integration is a separate, out-of-repo change.
- **Repairing a ledger found to have regressed.** This verb only detects and reports; it never rewrites the ledger, unlike `event merge`.
- **Comparing anything other than the UUID set.** A UUID present at both revisions with DIFFERENT content (a hand-edited record) is not detected by this verb — that is `event merge`'s conflict-detection territory, not a monotonicity check.
- **Non-git ledger storage.** The historical read is git-specific (`git show <ref>:<path>`); a project without a git repository at its root cannot use this verb.

## Assumption Carryover

Not applicable — this Feature was not derived from a SpecScore Idea; it was
prompted directly by `lesson:an-append-only-ledger-merge-must-prove-the-result-is-not-shorter-than-either-parent`
(recorded 2026-09-09 in `sneat-co/backstage`; see Summary).

## Acceptance Criteria

### AC: verb-registers-and-helps

**Requirements:** cli/event/check#req:verb-registration

**Given** a `specscore` binary built from this Feature's implementation
**When** `specscore event check --help` runs
**Then** exit code MUST be `0`; the output MUST document the `--base` flag.

### AC: working-tree-retains-all-base-uuids-passes

**Requirements:** cli/event/check#req:ledger-not-shorter-than-base

**Given** a git repository with a commit `<base>` whose configured JSONL
event ledger contains one or more events, and a working tree whose ledger
contains every one of those event UUIDs (byte-identical or with additional
events appended)
**When** `specscore event check --base <base>` runs
**Then** exit code MUST be `0`.

### AC: working-tree-missing-base-uuid-fails-nonzero

**Requirements:** cli/event/check#req:ledger-not-shorter-than-base

**Given** a git repository with a commit `<base>` whose configured JSONL
event ledger contains at least one event, and a working tree whose ledger
lacks at least one of those event UUIDs (the ledger going backwards — the
exact failure mode of the incident that prompted this Feature)
**When** `specscore event check --base <base>` runs
**Then** exit code MUST be non-zero; the missing event UUID MUST appear
verbatim in the command's output (stdout or stderr).

### AC: unresolvable-base-fails-clearly

**Requirements:** cli/event/check#req:ledger-not-shorter-than-base

**Given** a git repository and a `--base` value that does not resolve to any
commit in it (e.g. a ref that was never created)
**When** `specscore event check --base <bogus-ref>` runs
**Then** exit code MUST be `2`; stderr MUST contain a clear message naming
the unresolved `--base` value.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
