---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: Artifact Lifecycle Outbox Integration

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/event/artifact-lifecycle-outbox?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/event/artifact-lifecycle-outbox?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/event/artifact-lifecycle-outbox?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/event/artifact-lifecycle-outbox?op=request-change) |
**Status:** Draft
**Source Ideas:** —

## Summary

Founder decision (2026-08-10): every SpecScore artifact lifecycle mutation must
eventually use the durable outbox, not only Lesson commands. This Feature owns
the follow-on `preparedArtifactEvent` command boundary and an auditable matrix
for the rollout. It does not expand the current Lesson-focused implementation.

The generic `pkg/event` durable outbox is **Full**. Automatic lifecycle-event
integration is **Partial** and currently Lesson-only; capability, help, and AI
surfaces must keep making that distinction until this Feature is implemented.

## Problem

Artifact commands currently have different publication boundaries: some have no
event, some emit after a mutation, and Lesson commands prepare an immutable
event before mutation then commit/replay after the durable artifact and derived
index are consistent. An event emitted too early can describe a rolled-back
artifact; one emitted too late can be lost after a successful mutation. Each
command must make the same durable decision and recovery semantics observable.

## Behavior

### REQ: prepared-artifact-event-boundary

`internal/cli` MUST expose one private `preparedArtifactEvent` boundary used by
lifecycle commands. It receives a typed artifact identity, event name, payload,
and a command-specific mutation/compensation contract. It prepares the complete
subscriber ledger before the first artifact write; commits only after the
artifact and every declared derived projection are durable; and leaves an
inspectable prepared record when the mutation outcome is uncertain. It MUST NOT
infer success from file existence alone.

### REQ: lifecycle-command-matrix

The implementation Plan MUST maintain and test this matrix before a command is
advertised as automatically integrated. “Fence” means the precise artifact and
projection state that must be durable before event commit.

| Artifact / command family | Event | Prepare → commit point | Rollback / uncertain outcome | Payload and index fence | Tests, help, AI skill |
|---|---|---|---|---|---|
| Idea: create, status, archive, unarchive, promote, relocate, supersede | `idea.<action>` | Prepare before artifact write; commit after idea file and ideas index fence. | Compensate file/index when safe; otherwise leave prepared and require reconcile. | slug, status/action, source/revision; idea file plus `spec/ideas/README.md`. | Fault matrix, replay/reconcile journey; help and `ai/skills` name Partial until complete. |
| Feature: create, status, archive, unarchive, promote, relocate, supersede | `feature.<action>` | Prepare before feature mutation; commit after hierarchy/index fence. | Restore bounded mutation or retain prepared recovery. | feature id/status and parent; feature README plus parent contents/index. | Same deterministic fault/replay/help/AI proof. |
| Plan: create, status, archive, unarchive, promote, relocate, supersede | `plan.<action>` | Prepare before plan write; commit after plan/index and task-rollup fence. | Restore plan/projection or retain prepared recovery. | slug/status/source; plan file and plan index/rollup. | Same proof. |
| Task: create, status, archive, unarchive, promote, relocate, supersede | `task.<action>` | Prepare before task mutation; commit after parent Plan task rollup. | Restore bounded task/rollup or retain prepared recovery. | task id/status/plan; task artifact and parent rollup. | Same proof. |
| Decision: create, status, archive, unarchive, promote, relocate, supersede | `decision.<action>` | Prepare before decision mutation; commit after decisions projection. | Restore artifact/projection or retain prepared recovery. | id/status; decision file and index. | Same proof. |
| Issue: create, status, archive, unarchive, promote, relocate, supersede | `issue.<action>` | Prepare before issue mutation; commit after issue projection. | Restore artifact/projection or retain prepared recovery. | id/status; issue file and index. | Same proof. |
| Proposal: create, status, archive, unarchive, promote, relocate, supersede | `proposal.<action>` | Prepare before proposal mutation; commit after proposal projection. | Restore artifact/projection or retain prepared recovery. | id/status; proposal file and index. | Same proof. |
| Sidekick artifact: create, status, archive, unarchive, promote, relocate, supersede | `sidekick.<action>` | Prepare before Sidekick artifact mutation; commit after declared Sidekick projection. | Restore declared bounded writes or retain prepared recovery. | id/status/source; artifact and registered projection. | Same proof. |
| Graph/plugin artifact: create, status, archive, unarchive, promote, relocate, supersede | `graph.<action>` or `plugin.<action>` | Prepare before registered artifact mutation; commit after registry/graph fence. | Restore registered writes or retain prepared recovery. | typed id/action; artifact plus registry/graph index. | Same proof. |

### REQ: truthful-capability-surfaces

Capability JSON, command help, and every SpecScore AI skill MUST say that the
outbox primitive is Full while automatic lifecycle coverage is Partial and
Lesson-only until every matrix row is implemented and proven. No consumer may
claim generic automatic lifecycle delivery from the existence of `pkg/event`.

## Acceptance Criteria

### AC: prepared-boundary-preserves-artifact-truth

Given any integrated lifecycle command and a configured durable subscriber,
when preparation, mutation, index fencing, commit, or replay is interrupted,
then the event is either committed only after the declared fence or remains a
reconcilable prepared record; it never describes a mutation that was rolled
back or whose outcome is unknown.

### AC: matrix-is-executable-and-complete

Given every matrix row, when its command/action is implemented, then a
deterministic fault test proves prepare, commit, compensation/uncertain
recovery, payload content, and exact index fence. The end-to-end journey proves
an external subscriber sees the committed event exactly after the artifact is
durable and can replay an interrupted delivery.

### AC: capability-stays-honest-during-rollout

Given a row not yet implemented, when users read capability output, command
help, or an AI skill, then it says Partial/Lesson-only rather than Full
automatic lifecycle integration.

## Open Questions

None at this time. Command-specific fence details are implementation Plan
tasks, not unresolved product authority.

---
*This document follows the https://specscore.md/feature-specification*
