---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Legacy status-value autofix

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/legacy-status-autofix?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/legacy-status-autofix?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/legacy-status-autofix?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/legacy-status-autofix?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Summary

`specscore spec lint --fix` MUST rewrite a **closed, documented set** of legacy artifact status tokens to their canonical replacements for Plans (`P-006`), Decisions (`D-status-values`), and Ideas (`idea-status-values`). The mapping is exactly the legacy→canonical table established by the upstream [`status-vocabulary`](https://github.com/specscore/specscore/blob/main/spec/features/status-vocabulary/README.md) Feature. Values outside this table — typos, in-flight prose, any token with no documented predecessor — remain plain `error` violations and are NEVER auto-rewritten, preserving the original rule rationale that *choosing* a lifecycle value requires human intent.

## Problem

The status-vocabulary migration (specscore #29) renamed several artifact status values: Plans in the wild carry `Completed` (intended `Implemented`) and `Under Review` (now `In Review`); Decisions carry the legacy ADR words `Accepted` (now `Approved`) and `Proposed` (now `In Review`); Ideas carry `Archived` as a *status* (now the orthogonal `**Archived:** true` flag with terminal status `Stale`). Across a multi-repo fleet these legacy tokens now fail `P-006`, `D-status-values`, and `idea-status-values` with no automated remedy: today every one of these rules is non-autofixable, so each of dozens of files must be hand-edited.

These renames are a **closed, mechanical substitution** — a fixed dictionary, not a judgment call — so they are safely autofixable. This Feature narrows the existing non-autofixable stance (notably [`plan-rules#req:rule-p-006-not-autofixable`](../plan-rules/README.md)) to carve out exactly the documented legacy tokens, while leaving every other invalid value a hand-fix error.

## Behavior

### Legacy mapping tables

#### REQ: legacy-status-map

The fixer MUST recognize exactly this legacy→canonical table and no other entries. Matching is exact and case-sensitive on the artifact's body `**Status:**` token (after trimming surrounding whitespace):

| Artifact | Rule | Legacy value | Canonical value |
|----------|------|--------------|-----------------|
| Plan | `P-006` | `Completed` | `Implemented` |
| Plan | `P-006` | `Under Review` | `In Review` |
| Decision | `D-status-values` | `Accepted` | `Approved` |
| Decision | `D-status-values` | `Proposed` | `In Review` |
| Idea | `idea-status-values` | `Archived` | `Stale` (see [idea-legacy-fixer](#req-idea-legacy-fixer)) |

A token that is not a key in this table for its artifact type MUST NOT be rewritten. The table is the single source of truth; canonical values themselves are never keys (so a canonical value is left untouched).

### Fixer behavior

#### REQ: plan-decision-legacy-fixer

For Plans (`P-006`) and Decisions (`D-status-values`), when a present body `**Status:**` value is a legacy key for that artifact type, `specscore spec lint --fix` MUST rewrite ONLY that `**Status:**` line to the canonical value, byte-preserving the rest of the file. When the artifact carries frontmatter `status:` mirroring the body status (per the `status-mirror` rule), the fixer MUST update the frontmatter `status:` in the same pass so the mirror invariant holds after the fix. No other line is modified.

#### REQ: idea-legacy-fixer

For Ideas (`idea-status-values`), when the body `**Status:**` value is the legacy `Archived`, `specscore spec lint --fix` MUST (a) rewrite the body `**Status:**` (and mirrored frontmatter `status:`, if present) to `Stale`, and (b) set the `**Archived:** true` body-metadata flag, adding the line if absent and leaving it unchanged if already `true`. The fixer preserves the orthogonal-archival model: the archived intent is carried by the flag, not the status. Physical relocation of the file into `spec/ideas/archived/` is OUT OF SCOPE for this fixer and remains owned by the `idea-archived-location` rule and the `idea archive` verb.

#### REQ: closed-set-only

Any body `**Status:**` value that is NOT a legacy key in [legacy-status-map](#req-legacy-status-map) for its artifact type MUST remain a plain `error` violation from its existing rule (`P-006`, `D-status-values`, or `idea-status-values`) and MUST NOT be modified by `--fix`. In particular, free-form prose status lines (e.g. a stub plan whose `**Status:**` is a sentence) are never rewritten. The fixer adds NO new violation and removes none other than the ones it actually rewrites.

#### REQ: idempotent-and-targeted

Each rewrite MUST be idempotent: a second `--fix` pass over an already-canonical tree is a no-op (zero files modified). The legacy fixers MUST run on the unscoped `specscore spec lint --fix` pass and MUST also run when named explicitly via `--fix=P-006`, `--fix=D-status-values`, or `--fix=idea-status-values`. No new enabling flag is introduced; the fix surface and reporting (`fixed`/`violations` output contract) are exactly those already defined by `cli/spec/lint`.

## Acceptance Criteria

### AC: plan-completed-rewrite (verifies REQ:legacy-status-map, REQ:plan-decision-legacy-fixer, REQ:idempotent-and-targeted)

Scenario: Plan `Completed` becomes `Implemented`
Given a single-file Plan whose body `**Status:**` is `Completed`
When `specscore spec lint --fix` runs
Then the Plan's `**Status:**` line is rewritten to `**Status:** Implemented`, every other line is byte-identical, the `P-006` violation is gone, and a second `--fix` pass modifies zero files.

### AC: plan-under-review-rewrite (verifies REQ:legacy-status-map, REQ:plan-decision-legacy-fixer)

Scenario: Plan `Under Review` becomes `In Review`
Given a single-file Plan whose body `**Status:**` is `Under Review`
When `specscore spec lint --fix=P-006` runs
Then the Plan's `**Status:**` line is rewritten to `**Status:** In Review` and the `P-006` violation is gone.

### AC: decision-legacy-rewrite (verifies REQ:legacy-status-map, REQ:plan-decision-legacy-fixer)

Scenario: Decision legacy ADR words map to the prep band
Given one Decision with `**Status:** Accepted` and another with `**Status:** Proposed`
When `specscore spec lint --fix` runs
Then the first becomes `**Status:** Approved`, the second becomes `**Status:** In Review`, and both `D-status-values` violations are gone.

### AC: decision-frontmatter-mirror (verifies REQ:plan-decision-legacy-fixer)

Scenario: Mirrored frontmatter is updated in the same pass
Given a Decision with frontmatter `status: Accepted` mirroring body `**Status:** Accepted`
When `specscore spec lint --fix` runs
Then both the frontmatter `status:` and the body `**Status:**` read `Approved`, and no `status-mirror` violation is introduced.

### AC: idea-archived-rewrite (verifies REQ:idea-legacy-fixer)

Scenario: Idea `Archived` status becomes `Stale` plus the archived flag
Given an Idea whose body `**Status:**` is `Archived` and which has no `**Archived:**` line
When `specscore spec lint --fix` runs
Then its body `**Status:**` reads `Stale`, an `**Archived:** true` line is present, the file is not relocated, and the `idea-status-values` violation is gone.

### AC: unknown-value-untouched (verifies REQ:closed-set-only)

Scenario: A non-legacy invalid status is left for a human
Given a single-file Plan whose body `**Status:**` is the prose sentence `Not started.`
When `specscore spec lint --fix` runs
Then the Plan file is unmodified and the `P-006` violation for that file is still reported.

## Not Doing

- **No physical relocation** of Ideas into `spec/ideas/archived/` — owned by `idea-archived-location` / `idea archive`.
- **No new lifecycle judgement** — only the closed documented legacy table is mapped; ambiguous or prose values stay errors.
- **No new `--fix` surface or flag** — reuses the existing `cli/spec/lint` fix reporting and targeting contract.
- **No task-status (`P-004` lowercase) changes** — this Feature is about artifact body statuses only.

## Rehearse Integration

All ACs are CLI-observable (filesystem state + lint exit/report after `specscore spec lint --fix`), so each is testable via existing `pkg/lint` table tests; Rehearse stubs are deferred in favor of the established Go test seams that already cover `P-006`/`P-007` fixers.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
