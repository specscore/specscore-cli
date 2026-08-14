---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: Lifecycle Transitions (Shared Contract)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lifecycle-transitions?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lifecycle-transitions?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lifecycle-transitions?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lifecycle-transitions?op=request-change) |

**Status:** Stable
**Source Ideas:** lifecycle-verbs-for-idea-and-feature

## Summary

This is **not a command group** — it has no CLI surface of its own. It is the shared cross-cutting contract every `specscore` verb that mutates the `Status` field of a SpecScore artifact MUST satisfy: atomic artifact mutation, declared post-commit recovery, output format, exit-code mapping, and the scope boundary between local file safety and distributed coordination. Verb features reference this feature instead of restating these rules.

## Problem

Every lifecycle/state-transition verb has the same core: under its local artifact transaction, read the current status, validate the source state against the verb's legal transitions, compose the `**Status:**` change and any same-artifact fields, commit once, then run any derived work declared by that verb. The [source Idea](../../../ideas/lifecycle-verbs-for-idea-and-feature.md) plans seven such verbs across two doc kinds. Restating the core in seven feature specs would guarantee drift; this Meta feature centralizes it while each verb still declares whether it has a derived lint/index hook.

## Behavior

A note on REQ types in this contract: some REQs declare **runtime behavior** (e.g., `status-line-rewrite`, `index-sync-on-success`, the historically named `rollback-on-lint-failure` recovery requirement, and `optional-transition-note`) and are verified by ACs in the verb specs that consume this contract. Others are **scoping or architectural** (e.g., `scope-status-mutation-only`, `no-coordination`, `scope-no-task-lifecycle`, `exit-code-fidelity`) — they constrain what this contract is and isn't, not what a verb does at runtime. Architectural REQs are verified by design adherence and code review rather than by per-verb ACs; per-verb specs MAY but need NOT cite them.

### Scope and applicability

This contract applies to every verb under `specscore <kind> <verb>` whose effect is to mutate the `Status` field of a single SpecScore artifact. It does NOT apply to creation verbs (`<kind> new`), query verbs (`<kind> info`, `<kind> list`, `<kind> tree`, `<kind> deps`, `<kind> refs`), or `spec lint`.

#### REQ: scope-status-mutation-only

A verb that mutates fields other than `Status` (e.g., a future `owner-change` verb) is OUT of scope for this contract. Owner mutation is a field overwrite, not a state-machine transition, and is governed by its own (future) feature spec.

### Architectural positioning and scope

`specscore` is a local-file mutation primitive. Every cooperating writer uses a fail-fast per-artifact advisory lock and an expected-byte pre-rename fence; those are local file-safety mechanisms, not a distributed coordination protocol. `specscore` does not provide sync policy, claim/release semantics, remote-git publication, or multi-agent terminal-state agreement. Those orchestration concerns remain external.

#### REQ: no-coordination

Lifecycle verbs MUST NOT push remote git, consult a sync policy, claim/release an actor, or assume distributed coordination. They MUST use the shared fail-fast local artifact transaction for cooperating-writer safety. A non-cooperating edit detected by the final expected-byte fence is a conflict, not a claim race. Callers needing coordinated workflows over the spec graph use an external workflow orchestrator.

#### REQ: scope-no-task-lifecycle

Lifecycle verbs governed by this contract MUST NOT target the `task` doc kind for any **distributed coordination-bearing** transition — claim/release, sync policy, remote publication, or multi-agent terminal-state agreement — which remains owned by external workflow orchestrators. The permitted [`cli/task/change-status`](../task/change-status/README.md) verb performs only a local artifact transaction, including fail-fast lock contention and a final expected-byte conflict check. It does not reopen the distributed coordination lane this contract keeps out.

### State-machine semantics

The contract enforces a strict, declared-transition state machine per verb. Idempotence is NOT carved out — re-running a verb on an artifact already in the target state is an illegal transition.

#### REQ: state-machine-strictness

Every verb MUST declare its legal source-status set and target status. Before any mutation, the verb MUST read the artifact's current `**Status:**` value and confirm it appears in the declared legal-source set. If not, the verb MUST exit `4` (InvalidTransition) per the [shared exit-code contract](../README.md#shared-exit-code-contract) and leave the artifact unchanged. The stderr message MUST name both the artifact's current status and the legal source-status set for the verb.

#### REQ: not-idempotent

Lifecycle verbs MUST NOT special-case the case where the artifact is already in the target status. An artifact in the target status is, by definition, not in any legal source status (because no verb's legal-source set includes its own target), so the strict source check rejects it with exit `4`. This is a contract invariant: per-kind transition tables for any verb MUST NOT declare the verb's target status as one of its own legal source values. Callers wanting idempotent behavior read state first (via the artifact's index row or by parsing the file).

### Atomic mutation and index sync

A lifecycle transition commits its canonical artifact in one transaction. A verb that declares derived lint/index work runs it only after releasing the artifact lock, and exits `0` only after that callback succeeds. Task/Plan transaction-profile verbs retain an already-committed artifact when the callback fails and return a typed recovery-required error; they never restore a stale preimage. Historical compensating multi-artifact verbs retain their explicitly documented behavior until they are migrated and are outside this Task/Plan transaction amendment.

#### REQ: status-line-rewrite

On valid transition, the artifact's `**Status:** <old>` line MUST be rewritten to `**Status:** <new>`. The rewrite MUST be line-targeted: every other line in the file (including ordering, indentation, and trailing whitespace) MUST remain byte-identical to its pre-mutation content. The rewrite uses the same artifact parser the lint layer uses, so format detection is shared.

#### REQ: index-sync-on-success

When a verb's feature declares derived index synchronization, after a successful artifact transaction it MUST invoke `specscore spec lint --fix` scoped to the project root (full-tree today). The verb's exit code MUST be `0` only if the artifact transaction and declared lint pass both succeed. A verb with no derived index surface, including board Task mutation, MUST say so in its own feature rather than implying an unavailable callback.

#### REQ: rollback-on-lint-failure

The identifier is retained for compatibility with existing cross-references;
for transaction-profile Task/Plan verbs the required outcome is retained
commit plus recovery, not rollback.

For Task/Plan transaction-profile verbs, if declared derived post-mutation work such as `spec lint --fix` reports an error after the artifact transaction commits, the verb exits `10` and retains the committed artifact as an explicit recovery-required state; it MUST NOT perform a later split rollback that could erase another writer. Fail-fast lifecycle-lock contention exits `1` and requires a re-read. Historical compensating verbs are not silently reclassified by this requirement; their own feature specs remain authoritative until migration.

### Argument shape and output

Every lifecycle verb takes a single positional identifier argument naming the target artifact. There is no list-of-identifiers variant; batch transitions are out of scope per the source Idea.

Throughout this contract, **"slug"** is shorthand for the kind's canonical identifier — `<slug>` for Idea (the file basename), `<feature_id>` for Feature (the directory path, possibly nested). Per-verb specs name the kind-specific token in their Synopsis and Parameters tables.

#### REQ: slug-positional

The artifact identifier MUST be supplied as a single positional argument. Missing argument MUST exit `2` (InvalidArgs) per the shared exit-code contract. Flag-form arguments (e.g., `--slug`, `--feature`) MUST NOT be accepted by lifecycle verbs; this matches the `<kind> info <id>` and `idea new <slug>` precedents.

#### REQ: slug-resolves-to-existing-artifact

The identifier MUST resolve to an existing artifact at the kind's canonical path within the project root (autodetected per [CLI#req:project-autodetect](../README.md#req-project-autodetect), or set via `--project`). If no such artifact exists, the verb MUST exit `3` (NotFound) with a message naming the expected path. Where the kind defines an archived location (e.g., `spec/ideas/archived/<slug>.md` for Idea), artifacts under that location MUST NOT be matched by the canonical lookup. Kinds without an archived-equivalent location (e.g., Feature) MUST simply consult their canonical path.

#### REQ: success-output-format

On exit `0`, the verb MUST write exactly one line to stdout: `<id>: <from-status> → <to-status>\n` (using the artifact's slug or feature-id, the unicode arrow `→`, and the two status values). Nothing else MUST be written to stdout. This format is greppable and pipeable; structured output (`--format yaml|json`) is deferred to a later iteration and MUST NOT be added without amending this contract.

#### REQ: error-to-stderr

Non-zero exits write a human-readable explanation to stderr per [CLI#req:error-on-stderr](../README.md#req-error-on-stderr). stdout MUST remain empty on non-zero exits so pipelines consuming the structured success line are not corrupted by error prose.

### Optional transition note

#### REQ: optional-transition-note

Every lifecycle verb MAY accept an optional `--note <markdown>` flag carrying free-form markdown (the actor's reasoning for the transition). When `--note` is supplied and non-empty after trimming, the verb MUST — atomically with the `**Status:**` rewrite and index sync — write the markdown into the target artifact body as a `## Resolution` section, so callers never hand-edit the file (one invocation performs both the status change and the note write):

- If a `## Resolution` H2 section already exists, the markdown is appended as a new trailing paragraph within it; the section is never relocated or reordered.
- If absent, the verb creates the `## Resolution` section immediately before the artifact's footer line (`*This document follows the …*`) when one is present, else at end-of-file.

The markdown is written verbatim except for trailing-newline normalization; the verb MUST NOT reflow, wrap, truncate, or sanitize it. An empty or whitespace-only `--note` value is treated as absent — no section is written and no error is raised (unless the transition is reason-required; see below). A transaction-profile verb composes the note and status in the same in-memory transform and publishes them with one atomic durable write. Failure before commit leaves the original bytes; failure in declared post-commit derived work retains both committed fields and reports recovery required per [REQ: rollback-on-lint-failure](#req-rollback-on-lint-failure). `--note` does not alter the single-line [success output](#req-success-output-format). Historical compensating verbs continue to follow their own explicit relocation/rollback contract until migration.

#### REQ: reason-required-transitions

A verb MAY designate a subset of its legal transitions as **reason-required** — typically negative or terminal-rejection transitions (e.g. a seed `Queued → Rejected`). For a reason-required transition, `--note` is mandatory: if `--note` is absent or empty/whitespace-only, the verb MUST exit `2` (InvalidArgs) BEFORE any mutation, with a stderr message naming the transition and stating that a reason is required. When supplied, the note is written per [REQ: optional-transition-note](#req-optional-transition-note). Transitions a verb does NOT designate reason-required keep `--note` optional; a verb that designates none is unaffected by this REQ.

### Shared exit-code mapping

| Exit code | Condition |
|---|---|
| `0` | The artifact transaction and any derived work declared by the verb succeeded. |
| `1` | Fail-fast artifact-lock contention, a final expected-byte mismatch, or a verb-specific coordination precondition conflict. |
| `2` | Missing or malformed positional slug, an unknown flag, or a missing/empty `--note` on a reason-required transition. |
| `3` | No artifact file found at the expected path. |
| `4` | Source status was not in the verb's legal-source set (illegal transition, including re-running on the target status). |
| `10` | I/O failure during read/write, or derived post-mutation work failed after a committed artifact transaction (recovery required). |

#### REQ: exit-code-fidelity

A lifecycle verb MUST map errors to the codes above per their declared meanings. Code `1` is the local transaction/precondition conflict code; it does not imply distributed claim ownership. Codes `5–9` are reserved for other standard meanings per the [CLI exit-code contract](../README.md#shared-exit-code-contract).

### Learning a transition's effect without mutating

The rules above assume a caller is willing to mutate the artifact to find out what a transition does. In practice an agent or script sometimes needs to answer "is this transition legal, and what would it touch?" WITHOUT committing to the mutation — the only prior way to learn that was to run the verb for real and inspect a git diff afterward, trusting its own attention not to forget to revert. `REQ: dry-run-mode` closes that gap with an opt-in flag; `REQ: transitions-query-verb` closes the companion "what could this become?" gap with a permanent read-only verb.

#### REQ: dry-run-mode

A lifecycle verb MAY accept a `--dry-run` flag. When set:

- The verb MUST perform every read and validation step it would normally perform — slug/id resolution, the state-machine check, reason-required/successor/coordination-branch gating — EXACTLY as it would without the flag. An illegal transition, a missing artifact, or a missing required flag is rejected with the IDENTICAL exit code and stderr message a real invocation would produce; `--dry-run` does not soften or reinterpret [REQ: exit-code-fidelity](#req-exit-code-fidelity), it inherits it unchanged.
- On a LEGAL transition, the verb MUST write nothing: not the target artifact, not an ancestor index file, not any other file under the project's `spec/` tree. The working tree MUST be byte-for-byte identical before and after the call.
- On a LEGAL transition, the verb MUST report every file within `spec/` that WOULD change — the target artifact, plus whatever `spec lint --fix` would additionally touch (an ancestor index row, a parent's contents table, or a bidirectional link on a second artifact for the kinds that write one) — with paths relative to the project root, and MUST exit `0`. Operational `.specscore/` event-ledger records are deliberately excluded: they carry a fresh event UUID and are delivery bookkeeping rather than artifact edits.
- The reported file list MUST be produced by running the SAME mutation code path the real command runs, against a throwaway copy of the project's `spec/` tree, then diffing that copy against the untouched original — never by a second, hand-authored prediction of what "should" change. This is what guarantees the list cannot drift from what a subsequent real run actually does: a dry run and a real run share the one mutation implementation, differing only in which tree it is pointed at.
- `--dry-run` output stays plain text — one line per changed file, `<letter> <path>` in `git status --short`'s `M`/`A`/`D` vocabulary — consistent with this contract's text-only stance in [REQ: success-output-format](#req-success-output-format). It does not resolve the deferred `--format yaml|json` question in Open Questions, which is scoped to the mutating command's own stdout.

`--dry-run` is implemented for `feature`, `idea`, `plan`, `lesson`, `decision`, `issue`, and `sidekick`. It is NOT implemented for `task`: task's mutation is a direct `lifecycle.Rewrite` call against `tasks/<task>/README.md` — outside `spec/`, with no shared per-kind `ChangeStatus` orchestrator to sandbox — plus provenance stamping and a plan-inline resolution mode (a task's status can live in a `### Task N:` block inside a Plan file) that would need its own preview semantics. Closing that gap is future work.

#### REQ: transitions-query-verb

Each kind implementing this contract SHOULD also expose a **read-only** `<kind> transitions [<id>]` verb, answering "what can this become?" without requiring a caller to already know a `--to` value to test:

- With no `<id>`, it prints the kind's complete bidirectional status matrix — every recognized status with its legal predecessors ("previous") and legal successors ("next") — derived from the SAME matrix `change-status` validates against. A status with no predecessors is initial-only (set only by the kind's `new`/scaffold verb); a status with no successors is terminal. Both MUST be stated outright rather than left for the reader to infer from a forward-only table's silence.
- With `<id>`, it resolves the artifact exactly as `change-status` would, reads its CURRENT status, and reports that one status's previous/next — i.e., exactly the `--to` values a subsequent `change-status` call on the SAME artifact would accept.

This verb is a query, not a mutation: it never writes, and it never validates a *proposed* transition — it only reports the legal set. It falls outside [REQ: scope-status-mutation-only](#req-scope-status-mutation-only)'s boundary and every REQ above that governs write behavior (rollback-on-lint-failure, the reason-required gates, and so on). It is documented in this Meta feature rather than left as a bare, uncontracted query verb because its entire content IS this contract's matrix — it has no behavior independent of it. It supports `--format text|json|yaml`, matching this CLI's existing query-verb convention (see [CLI](../README.md)); this is independent of — and does not reopen — the deferred structured-output question for `change-status`'s own mutating stdout.

`task transitions` covers board-mode tasks (`tasks/<task>/README.md`) only, for the same reason `task change-status` has no `--dry-run`: a plan-inline task's status lives inside a `### Task N:` block partway down its Plan file, which needs that Plan's own block parser to locate, not a bare artifact-path resolution.

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [CLI](../README.md) | Inherits the shared exit-code contract, including the pre-reserved code `4` for invalid state transitions and code `10` for unexpected/runtime errors. Inherits the `--project` autodetect rule. Inherits `REQ: error-on-stderr`. |
| [spec lint](../spec/lint/README.md) | Invoked after the committed lifecycle artifact transaction only by verbs that declare derived lint/index work. For transaction-profile Task/Plan verbs, failure is recovery-required and the committed artifact is retained. |
| [idea](../../idea/README.md), [feature](../../feature/README.md) | Sources of truth for the artifact document structures, including the `**Status:**` header line that lifecycle verbs rewrite, and the legal status enumerations per doc kind. |
| Source Idea: [lifecycle-verbs-for-idea-and-feature](../../../ideas/lifecycle-verbs-for-idea-and-feature.md) | This feature realizes the shared-infrastructure half of the source Idea. Per-verb features realize the kind-specific halves. |
| [`cli/idea/change-status`](../idea/change-status/README.md) | Verb implementing this contract for the Idea kind. Encodes the Idea legal-transition matrix (`Draft → Approved`, `Approved → Specifying`, `Specifying → Specified`, `Specified → Implementing`, `Implementing → Implemented`, `{Draft, In Review, Approved, Specifying, Specified, Implementing} → Rejected`, `{Draft, In Review, Approved, Specifying, Specified, Implementing} → Stale`; `{Draft, Under Review, Approved, Specifying, Specified, Implementing, Implemented} → Archived`) and extends the Meta with a `--to=archived` file-relocation side effect. Every pre-terminal status now has both a `Rejected` and a `Stale` exit — the disposition vocabulary is complete. Change-request ideas have all transitions author-managed (not derived from Feature status). |
| [`cli/feature/change-status`](../feature/change-status/README.md) | Verb implementing this contract for the Feature kind. Encodes the Feature legal-transition matrix (`Draft → Under Review`, `{Draft, Under Review} → Approved`, `Approved → Implementing`, `Implementing → Stable`, `{Approved, Stable} ↔ Amending`, `Approved → Rejected`, `{Draft, Implementing, Stable, Approved} → Deprecated`) and declares its dependency on the `feature-index-row-sync` lint rule. |
| `ai-plugin-specscore` skill wrappers _(planned, downstream)_ | When the plugin grows references for any lifecycle verb, each reference invokes `specscore <kind> change-status` directly. The plugin treats `specscore` as the canonical surface for Idea and Feature lifecycle. |

## Open Questions

- ~~Should `--reason "<text>"` become a shared flag on lifecycle verbs?~~ **Resolved** by [REQ: optional-transition-note](#req-optional-transition-note) and [REQ: reason-required-transitions](#req-reason-required-transitions): an optional `--note <markdown>` writes a `## Resolution` section atomically with the transition, and is mandatory on transitions a verb designates reason-required. Structured/audit-trail storage of the note (separate file or git trailer) remains deferred.
- Should `--format yaml|json` be added in a future revision so tooling consumes structured output (returning the artifact's full front-matter)? Currently text-only.
- Is `spec lint --fix` scope narrowed to only the affected index row (faster on large repos) or kept full-tree (safer)? Today's lint is fast enough that full-tree is acceptable, but measurement on representative consumer repos will decide if a narrow-scope path is worth the complexity.
- When a new doc kind grows lifecycle verbs (e.g., the planned `entity` and `property` Doc-Kinds from the meta-spec's [entity-and-property-definitions](https://github.com/specscore/specscore/blob/main/spec/ideas/entity-and-property-definitions.md) Idea), does it inherit this contract directly, or does the contract abstract a shared "index sync rule" parameter? Today every supported doc kind uses a `*-index-row-sync` rule, so direct inheritance works.
- Batch transitions (`specscore idea approve <slug-1> <slug-2> ...`) are out of MVP. If they land later, are they atomic-per-slug or all-or-nothing, and what explicit committed/recovery outcome applies to a partially published batch?
- Should `task change-status` grow `--dry-run` and a full (board- and plan-inline-mode) `task transitions`? It requires either extracting task's inline mutation into a shared per-kind `ChangeStatus` orchestrator (mirroring the other six kinds) or a bespoke sandbox/status-reader that understands plan-inline `### Task N:` block resolution and provenance stamping. Not attempted in this pass — see [REQ: dry-run-mode](#req-dry-run-mode) and [REQ: transitions-query-verb](#req-transitions-query-verb).

---
*This document follows the https://specscore.md/feature-specification*
