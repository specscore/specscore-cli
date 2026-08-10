---
format: https://specscore.md/plan-specification
status: Draft
---
# Plan: Artifact Lifecycle Outbox Rollout

**Status:** Draft
**Source Feature:** cli/event/artifact-lifecycle-outbox
**Date:** 2026-08-10
**Owner:** codex
**Supersedes:** —

## Summary

Implements the founder-approved durable-outbox rollout for all artifact
lifecycle commands, beginning with a private `preparedArtifactEvent` boundary
and a deterministic command/artifact matrix. This Plan deliberately follows,
rather than expands, the Lesson-only implementation currently under review.

## Approach

Start by making the common boundary and truthful partial capability surface
explicit. Then port one artifact family at a time through the same
prepare→mutation→projection fence→commit/replay journey. A row cannot advance
until its failure/compensation/reconcile path is proven with an external
subscriber; broad coverage percentages cannot replace that journey.

## Tasks

### Task 1: Define the private prepared-artifact-event boundary

**Id:** task-1
**Verifies:** cli/event/artifact-lifecycle-outbox#ac:prepared-boundary-preserves-artifact-truth
**Depends-On:** —
**Status:** planning

Implement the private command boundary and its deterministic test harness. It
must make prepare, exact commit fence, compensation, and uncertain outcome
explicit without creating an exported lifecycle abstraction.

### Task 2: Publish the executable matrix and truthful Partial surface

**Id:** task-2
**Verifies:** cli/event/artifact-lifecycle-outbox#ac:capability-stays-honest-during-rollout
**Depends-On:** 1
**Status:** planning

Add the command/action matrix as executable test inventory and align capability
JSON, help, and AI skills to Full primitive versus Partial/Lesson-only automatic
integration.

### Task 3: Port Idea, Feature, Plan, and Task lifecycle families

**Id:** task-3
**Verifies:** cli/event/artifact-lifecycle-outbox#ac:matrix-is-executable-and-complete
**Depends-On:** 1, 2
**Status:** planning

Port create/status/archive/unarchive/promote/relocate/supersede flows for these
four families, proving each artifact/index fence and recovery journey.

### Task 4: Port Decision, Issue, Proposal, Sidekick, and Graph/plugin families

**Id:** task-4
**Verifies:** cli/event/artifact-lifecycle-outbox#ac:matrix-is-executable-and-complete
**Depends-On:** 1, 2
**Status:** planning

Port the remaining matrix families with the same deterministic outbox journey
and then graduate the Partial surface only when every row is proven.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
