---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: Lifecycle Transitions (Shared Contract)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lifecycle-transitions?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lifecycle-transitions?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lifecycle-transitions?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lifecycle-transitions?op=request-change) |

**Status:** Stable
**Source Ideas:** lifecycle-verbs-for-idea-and-feature

## Summary

This is **not a command group** — it has no CLI surface of its own. It is the shared cross-cutting contract every `specscore` verb that mutates the `Status` field of a SpecScore artifact MUST satisfy: atomicity, rollback, output format, exit-code mapping, and the scope boundary against coordination/concurrency concerns. Verb features (e.g., [`cli/idea/approve`](../idea/approve/README.md), planned `cli/idea/archive`, `cli/feature/approve`, etc.) reference this feature instead of restating these rules.

## Problem

Every lifecycle/state-transition verb has the same skeleton: read the artifact's current status, validate the source state against the verb's legal transitions, rewrite the `**Status:**` line, run `specscore spec lint --fix` to sync the corresponding index row, roll back on failure, print a uniform success line, map errors to standard exit codes. The [source Idea](../../../ideas/lifecycle-verbs-for-idea-and-feature.md) plans seven such verbs across two doc kinds. Restating the skeleton in seven feature specs would (a) bloat each spec, (b) guarantee drift over time as one verb's rules update without the others, and (c) make it harder for future doc kinds to inherit the contract. A single Meta feature, referenced by every verb, fixes all three.

## Behavior

A note on REQ types in this contract: some REQs declare **runtime behavior** (e.g., `status-line-rewrite`, `index-sync-on-success`, `rollback-on-lint-failure`, `optional-transition-note`) and are verified by ACs in the verb specs that consume this contract. Others are **scoping or architectural** (e.g., `scope-status-mutation-only`, `no-coordination`, `scope-no-task-lifecycle`, `exit-code-fidelity`) — they constrain what this contract is and isn't, not what a verb does at runtime. Architectural REQs are verified by design adherence and code review rather than by per-verb ACs; per-verb specs MAY but need NOT cite them.

### Scope and applicability

This contract applies to every verb under `specscore <kind> <verb>` whose effect is to mutate the `Status` field of a single SpecScore artifact. It does NOT apply to creation verbs (`<kind> new`), query verbs (`<kind> info`, `<kind> list`, `<kind> tree`, `<kind> deps`, `<kind> refs`), or `spec lint`.

#### REQ: scope-status-mutation-only

A verb that mutates fields other than `Status` (e.g., a future `owner-change` verb) is OUT of scope for this contract. Owner mutation is a field overwrite, not a state-machine transition, and is governed by its own (future) feature spec.

### Architectural positioning and scope

`specscore` is a local-file mutation primitive. It does not provide concurrency control, sync policies, claim/release semantics, or multi-agent coordination. For doc kinds where local-file mutation IS the value (Idea, Feature) — transitions are deliberate, single-actor, contention-free — `specscore` is the canonical surface. For doc kinds whose lifecycle requires coordination (the `task` doc kind in particular), an external workflow orchestrator is the appropriate surface; lifecycle verbs governed by this contract MUST NOT target those kinds.

#### REQ: no-coordination

Lifecycle verbs MUST NOT acquire locks (advisory or mandatory), push to remote git, consult a sync policy, or assume any cross-process coordination. Concurrent modification of the target file by another process is undefined behavior. Callers needing coordinated workflows over the spec graph use an external workflow orchestrator.

#### REQ: scope-no-task-lifecycle

Lifecycle verbs governed by this contract MUST NOT target the `task` doc kind for any **coordination-bearing** transition — concurrency, claim/release, contention-resolved or conflict-aware semantics — which remains owned by external workflow orchestrators. The single permitted exception is a **single-actor** task status transition that performs pure file mutation with none of those coordination concerns: the [`cli/task/change-status`](../task/change-status/README.md) verb, governed by [cli/task/change-status#req:single-actor-task-lifecycle-permitted](../task/change-status/README.md#req-single-actor-task-lifecycle-permitted). That verb exists to provide the implementation-commit provenance capture point; it does not reopen the broader coordination lane this contract keeps out.

### State-machine semantics

The contract enforces a strict, declared-transition state machine per verb. Idempotence is NOT carved out — re-running a verb on an artifact already in the target state is an illegal transition.

#### REQ: state-machine-strictness

Every verb MUST declare its legal source-status set and target status. Before any mutation, the verb MUST read the artifact's current `**Status:**` value and confirm it appears in the declared legal-source set. If not, the verb MUST exit `4` (InvalidTransition) per the [shared exit-code contract](../README.md#shared-exit-code-contract) and leave the artifact unchanged. The stderr message MUST name both the artifact's current status and the legal source-status set for the verb.

#### REQ: not-idempotent

Lifecycle verbs MUST NOT special-case the case where the artifact is already in the target status. An artifact in the target status is, by definition, not in any legal source status (because no verb's legal-source set includes its own target), so the strict source check rejects it with exit `4`. This is a contract invariant: per-kind transition tables for any verb MUST NOT declare the verb's target status as one of its own legal source values. Callers wanting idempotent behavior read state first (via the artifact's index row or by parsing the file).

### Atomic mutation and index sync

A lifecycle transition is a two-step operation — file rewrite, then `spec lint --fix` to sync the corresponding index. Both MUST succeed for the verb to exit `0`. A failure in either step MUST leave the on-disk state observably identical to its pre-invocation state.

#### REQ: status-line-rewrite

On valid transition, the artifact's `**Status:** <old>` line MUST be rewritten to `**Status:** <new>`. The rewrite MUST be line-targeted: every other line in the file (including ordering, indentation, and trailing whitespace) MUST remain byte-identical to its pre-mutation content. The rewrite uses the same artifact parser the lint layer uses, so format detection is shared.

#### REQ: index-sync-on-success

After a successful file rewrite, the verb MUST invoke `specscore spec lint --fix` scoped to the project root (full-tree today; see Open Questions for future narrowing to the affected index row only). The lint pass picks up the relevant `*-index-row-sync` rule (e.g., `idea-index-row-sync` for Idea transitions) and rewrites the corresponding row in the artifact's index file. The verb's exit code MUST be `0` only if the file rewrite AND the lint pass BOTH succeed.

#### REQ: rollback-on-lint-failure

If derived post-mutation work such as `spec lint --fix` reports an error after the artifact transaction commits, the verb exits `10` and retains the committed artifact as an explicit recovery-required state; it MUST NOT perform a later split rollback that could erase another writer. Fail-fast lifecycle-lock contention exits `1` and requires a re-read.

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

The markdown is written verbatim except for trailing-newline normalization; the verb MUST NOT reflow, wrap, truncate, or sanitize it. An empty or whitespace-only `--note` value is treated as absent — no section is written and no error is raised (unless the transition is reason-required; see below). The body write participates in the same atomicity guarantee as the status rewrite: if the note write fails, or any subsequent step (index sync, kind-specific relocation) fails, the verb MUST restore the body to its exact pre-invocation content together with the `**Status:**` line per [REQ: rollback-on-lint-failure](#req-rollback-on-lint-failure), and exit `10`. `--note` does not alter the single-line [success output](#req-success-output-format) — the note is a body mutation, not stdout. Per-verb specs that consume this contract carry the ACs exercising `--note` for their kind.

#### REQ: reason-required-transitions

A verb MAY designate a subset of its legal transitions as **reason-required** — typically negative or terminal-rejection transitions (e.g. a seed `Queued → Rejected`). For a reason-required transition, `--note` is mandatory: if `--note` is absent or empty/whitespace-only, the verb MUST exit `2` (InvalidArgs) BEFORE any mutation, with a stderr message naming the transition and stating that a reason is required. When supplied, the note is written per [REQ: optional-transition-note](#req-optional-transition-note). Transitions a verb does NOT designate reason-required keep `--note` optional; a verb that designates none is unaffected by this REQ.

### Shared exit-code mapping

| Exit code | Condition |
|---|---|
| `0` | Transition succeeded and index synced. |
| `2` | Missing or malformed positional slug, an unknown flag, or a missing/empty `--note` on a reason-required transition. |
| `3` | No artifact file found at the expected path. |
| `4` | Source status was not in the verb's legal-source set (illegal transition, including re-running on the target status). |
| `10` | I/O failure during read/write, or derived post-mutation work failed after a committed artifact transaction (recovery required). |

#### REQ: exit-code-fidelity

A lifecycle verb MUST map errors to the codes above per their declared meanings. Codes `1` (Conflict) and `5–9` are NOT used by this contract: lifecycle verbs have no notion of concurrent-modification conflict (see [REQ: no-coordination](#req-no-coordination)), and `5–9` are reserved for future standard codes per the [CLI exit-code contract](../README.md#shared-exit-code-contract).

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [CLI](../README.md) | Inherits the shared exit-code contract, including the pre-reserved code `4` for invalid state transitions and code `10` for unexpected/runtime errors. Inherits the `--project` autodetect rule. Inherits `REQ: error-on-stderr`. |
| [spec lint](../spec/lint/README.md) | Invoked after the committed lifecycle artifact transaction to sync derived indexes. A failure is reported as recovery-required; the committed artifact is retained. |
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
- Batch transitions (`specscore idea approve <slug-1> <slug-2> ...`) are out of MVP. If they land later, are they atomic-per-slug or all-or-nothing? This affects whether [REQ: rollback-on-lint-failure](#req-rollback-on-lint-failure) extends partial-batch rollback or only single-slug.

---
*This document follows the https://specscore.md/feature-specification*
