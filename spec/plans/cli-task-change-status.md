---
format: https://specscore.md/plan-specification
status: Approved
---
# Plan: Cli Task Change Status

**Status:** Approved
**Source Feature:** cli/task/change-status
**Date:** 2026-06-26
**Owner:** alex
**Supersedes:** —
**Parent:** specscore:implementation-commit-provenance

## Summary

Implements the `specscore task change-status` verb — the capture vehicle for implementation-commit provenance. A sub-plan of the cross-repo master `specscore:implementation-commit-provenance`. Covers the verb's status-transition behavior, the dual board/plan-inline target resolution, and the optional `--repo`/`--commit`/`--branch` provenance flags. The data-model lint rule and plan rollup are owned by the master plan (also coded here, tracked there).

## Approach

Linear. **Task 1** stands up the command and the strict single-actor transition matrix on the shared `lifecycle-transitions` contract (no claim/release). **Task 2** adds task resolution across both stores — board (`tasks/<task>/`) and plan-inline (by the task block's stable `**Id:**`). **Task 3** implements the provenance write path (assembly, bare-sha, complete-only, never-from-HEAD); **Task 4** the provenance-flag rejections; **Task 5** the `--amend-provenance` corrective re-stamp. Provenance work (Tasks 3–5) depends on resolution (Task 2) so the verb can locate the task it stamps.

## Tasks

### Task 1: Scaffold `task change-status` command, matrix, and single-actor mutation

**Verifies:** cli/task/change-status#ac:to-flag-validation, cli/task/change-status#ac:illegal-transition-rejected, cli/task/change-status#ac:single-actor-no-coordination
**Depends-On:** —
**Status:** done

Add the `task change-status` cobra command wired to the shared `lifecycle-transitions` contract: required `--to` flag (case-insensitive; missing/unknown exits `2`), the strict task legal-transition matrix (illegal pairs exit `4`, non-idempotent), and pure single-actor status-field rewrite with no claim/release/locking.

### Task 2: Task target resolution — board and plan-inline

**Verifies:** cli/task/change-status#ac:plan-inline-target-resolves
**Depends-On:** 1
**Status:** pending

Resolve `<task>` in board mode (`tasks/<task>/README.md`, default) and plan-inline mode (`--plan <slug>` → the task block whose `**Id:**` equals `<task>` in `spec/plans/<slug>.md`), exiting `3` when neither resolves (plan-inline: no block with a matching `**Id:**`). Addressing is by the stable `**Id:**` field (resolved upstream).

### Task 3: Provenance write path on `--to=complete`

**Verifies:** cli/task/change-status#ac:complete-with-provenance, cli/task/change-status#ac:complete-without-provenance-is-valid, cli/task/change-status#ac:bare-sha-same-repo-assembly, cli/task/change-status#ac:provenance-not-derived-from-head
**Depends-On:** 2
**Status:** pending

Assemble `<repo>@<sha> (<branch>)` from the optional flags (bare `<sha>` when `--repo` omitted), write it to the resolved task's `implementation_commit` property on `--to=complete`, treat a flagless completion as valid, and never read ambient `HEAD` for values.

### Task 4: Provenance-flag validation and rejections

**Verifies:** cli/task/change-status#ac:provenance-flag-without-complete-rejected, cli/task/change-status#ac:provenance-flag-without-commit-rejected
**Depends-On:** 3
**Status:** pending

Reject (exit `2`) any provenance flag supplied with a non-`complete` `--to`, and any provenance flag set without `--commit`, leaving the task unchanged in both cases.

### Task 5: Corrective provenance re-stamp (`--amend-provenance`)

**Verifies:** cli/task/change-status#ac:corrective-restamp
**Depends-On:** 3
**Status:** pending

Implement `--amend-provenance` to overwrite (or clear) `implementation_commit` on a task already in `complete` without a status transition — mutually exclusive with `--to` (exit `2`), requiring `complete` (else exit `4`), and following the same `--commit`-required and syntactic-only rules. Emit `<task>: provenance amended`.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
