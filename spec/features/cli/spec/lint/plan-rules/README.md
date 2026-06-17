---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Plan Lint Rules

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/plan-rules?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/plan-rules?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/plan-rules?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/plan-rules?op=request-change) |
**Status:** Approved
**Date:** 2026-05-19
**Owner:** alexander.trakhimenok
**Source Ideas:** —
**Supersedes:** —

## Summary

Adds six lint rules (`P-001`–`P-006`) and the underlying single-file Plan parser to `specscore spec lint`, implementing the contract reserved by the SpecStudio `plan` Feature (`spec/features/skills/plan/README.md` in the [`specstudio-skills`](https://github.com/specscore/specstudio-skills) repo). `P-001`–`P-004` unblock the in-development `specstudio:implement` skill, which depends on machine-checkable validation of `**Mode:**`, `**Status:**`, and `**Depends-On:**` task fields on single-file Plans at `spec/plans/<slug>.md`. `P-005` validates the optional `**Parent:**` body-metadata line that `cli/plan/new --parent` emits — the master/sub-plan composition primitive — resolving same-repo parents and accepting cross-repo `<repo-slug>:<slug>` references syntactically without sibling-repo scanning.

## Problem

The SpecStudio `plan` Feature defines four lint rules (`P-001` AC coverage gap, `P-002` stale AC reference, `P-003` Depends-On cycle / dangling / self-reference, `P-004` placeholder body on `done`-status task in `stub` Plan) and extends the Plan task schema with three new body fields (`**Status:**`, `**Depends-On:**`, `**Mode:**`) plus a placeholder body marker for `stub` Plans. None of this exists in `specscore-cli` today. The `plan` Feature revision is approved upstream and `specstudio:implement` cannot ship until this CLI work lands.

This Feature is the CLI-side companion to the upstream contract: it adds the parser extensions and the four lint rules, then registers them so `specscore spec lint` runs them as part of the default rule suite.

## Behavior

### Plan artifact detection

`specscore-cli` already supports directory-form plans at `spec/plans/<slug>/README.md` (used by this repo's own plans). The new rules MUST coexist with that format by only operating on single-file plans.

#### REQ: plan-detection-single-file

The new rules MUST only operate on single-file Plans at `spec/plans/<slug>.md` (files directly under `spec/plans/`, with the `.md` extension and not named `README.md`). Directory-form plans (`spec/plans/<slug>/README.md`) MUST be left to the existing `plan-hierarchy` and `plan-roi-metadata` checkers and MUST NOT be linted by `P-001`–`P-004`.

#### REQ: plan-detection-title-prefix

A file at `spec/plans/<slug>.md` is recognized as a single-file Plan when its first H1 heading matches `# Plan: <title>` (exact prefix, leading hash and single space). Files at the same path without this title prefix MUST be silently skipped by `P-001`–`P-004` so that unrelated `.md` files dropped into `spec/plans/` do not break the linter.

### Plan body metadata

A single-file Plan declares its source Feature and its posture in body-metadata lines directly after the title heading.

#### REQ: plan-source-feature-field

The parser MUST recognize a `**Source Feature:** <feature-slug>` body-metadata line. The `<feature-slug>` value is a forward-slash-separated path that MUST resolve to an existing Feature at `spec/features/<feature-slug>/README.md` relative to the project root. Lint rules `P-001` and `P-002` consume this field to locate the source-Feature AC list.

#### REQ: plan-mode-field

The parser MUST recognize a `**Mode:** <full|stub>` body-metadata line. The value MUST be exactly `full` or `stub` (lowercase, no surrounding whitespace inside the value). When the field is absent, the parser MUST treat the Plan as `**Mode:** full` (backward-compatible default per the upstream `REQ:plan-mode-field` in the `plan` Feature). When the field is present with any value other than `full` or `stub`, the parser MUST report the violation through `P-004` so the lint suite surfaces it.

### Task block parsing

#### REQ: task-block-parse

The parser MUST recognize `### Task N: <name>` blocks under the `## Tasks` H2 section, where `N` is a positive integer. Each task block extends from its `### Task` heading to the next `### Task` heading, the next `## ` H2 heading, or end-of-file — whichever comes first. Task numbering MUST be linearly 1..N with no gaps; gapped or non-monotonic numbering MUST NOT be tolerated by the parser and MUST surface as a `P-003` violation (since the dependency graph cannot reconcile non-linear numbers).

#### REQ: task-verifies-field

The parser MUST recognize `**Verifies:** <feature-slug>#ac:<ac-slug>, <feature-slug>#ac:<ac-slug>, …` as a task body field. The field value is a comma-separated list of AC IDs in the form `<feature-slug>#ac:<ac-slug>`. The `<feature-slug>` portion MUST equal the Plan's `**Source Feature:**` value; cross-Feature AC references are out of scope and MUST surface as `P-002` violations. An empty `**Verifies:**` line (no AC IDs) is a `P-002` violation (it is treated as a stale reference rather than introducing a separate rule).

#### REQ: task-status-field

The parser MUST recognize `**Status:** <pending|in-progress|done|blocked>` as a task body field. The value MUST be exactly one of those four lowercase tokens. When the field is absent, the parser MUST treat the task as `**Status:** pending` (backward-compatible default per upstream `REQ:task-status-field`). When the field is present with any other value, the lint suite MUST report the violation through `P-004` so a single rule covers schema-level posture and status validity.

#### REQ: task-depends-on-field

The parser MUST recognize `**Depends-On:** —` or `**Depends-On:** <task-number>, <task-number>, …` as a task body field, where `<task-number>` is the integer task number of a predecessor task in the same Plan. The em-dash (`—`) sentinel means "no predecessors". When the field is absent, the parser MUST treat the task as `**Depends-On:** —` (backward-compatible default per upstream `REQ:depends-on-field`). Predecessor numbers MUST be positive integers in decimal form; whitespace around commas is permitted; trailing commas are permitted but produce no extra predecessor.

#### REQ: task-placeholder-body

The parser MUST recognize the exact token `<!-- implement: pending -->` as a placeholder body marker for a task. The token MUST appear on a line of its own inside the task body (after any required `**Verifies:**` / `**Status:**` / `**Depends-On:**` lines), with surrounding whitespace permitted on that line. The match MUST be byte-exact; case variations (`<!-- IMPLEMENT: pending -->`), alternate spacings inside the comment (`<!--implement: pending-->`), or alternative tokens MUST NOT be recognized as placeholders.

### Deferred AC coverage section

#### REQ: deferred-ac-coverage-parse

The parser MUST recognize an optional `## Deferred AC Coverage` H2 section whose body is a Markdown list of `- <feature-slug>#ac:<ac-slug> — <reason>` entries. Each entry's AC ID MUST follow the same grammar as the `**Verifies:**` task field. Entries listed here MUST be treated by `P-001` as satisfying AC coverage. The reason text is opaque to the CLI lint rules (the SpecStudio reviewer subagent enforces non-vague reasons); `P-001` does NOT validate reason quality.

### Lint rule P-001 — AC coverage gap

`P-001` enforces that every AC in the source Feature is accounted for — either covered by at least one task or explicitly deferred.

#### REQ: rule-p-001-registered

`P-001` MUST be registered in the lint rule registry under the name `P-001` (uppercase, hyphenated), at severity `error`, and MUST execute as part of the default rule suite.

#### REQ: rule-p-001-coverage-gap

`P-001` MUST report a violation when an AC declared in the Plan's source Feature (every `### AC: <ac-slug>` heading under `## Acceptance Criteria` in `spec/features/<source-feature-slug>/README.md`) is neither covered by any task's `**Verifies:**` line nor listed under `## Deferred AC Coverage`. The violation MUST name the uncovered AC ID in `<feature-slug>#ac:<ac-slug>` form and MUST cite the Plan file path and the AC heading line in the source Feature.

#### REQ: rule-p-001-not-autofixable

`P-001` MUST NOT be autofixable in the MVP. The fix requires user intent (add a task vs. defer the AC vs. revise the source Feature), so the CLI MUST surface the violation without offering `--fix`.

### Lint rule P-002 — Stale AC reference

`P-002` enforces that every AC reference in a task's `**Verifies:**` line or in `## Deferred AC Coverage` resolves to a real AC in the source Feature.

#### REQ: rule-p-002-registered

`P-002` MUST be registered in the lint rule registry under the name `P-002`, at severity `error`, and MUST execute as part of the default rule suite.

#### REQ: rule-p-002-stale-reference

`P-002` MUST report a violation when an AC ID referenced by a task's `**Verifies:**` line or by a `## Deferred AC Coverage` entry does not resolve to a real `### AC: <ac-slug>` heading in the source Feature's `README.md`. The violation MUST cite the offending AC ID, the Plan file path, and the line where the reference appears. When the source Feature does not exist (the `**Source Feature:**` field points to a path with no `README.md`), `P-002` MUST report a single violation citing the missing source Feature rather than emitting one violation per AC reference.

#### REQ: rule-p-002-not-autofixable

`P-002` MUST NOT be autofixable in the MVP. Resolving a stale reference requires user intent (rename the reference vs. delete the task vs. add the AC to the source Feature).

### Lint rule P-003 — Depends-On graph

`P-003` enforces that the task dependency graph is well-formed: acyclic, all references resolve to real tasks, no self-references, and task numbering is linear.

#### REQ: rule-p-003-registered

`P-003` MUST be registered in the lint rule registry under the name `P-003`, at severity `error`, and MUST execute as part of the default rule suite.

#### REQ: rule-p-003-cycle

`P-003` MUST report a violation when the task dependency graph contains a cycle. The violation message MUST cite the full cycle path in the form `Task A → Task B → … → Task A` so the user can locate every node that needs editing.

#### REQ: rule-p-003-dangling

`P-003` MUST report a violation when a task's `**Depends-On:**` field references a task number that does not exist in the same Plan. The violation message MUST cite the offending task number (the dependent) and the dangling predecessor number, in the form `Task N depends on nonexistent task M`.

#### REQ: rule-p-003-self-reference

`P-003` MUST report a violation when a task's `**Depends-On:**` field lists its own task number. The violation message MUST cite the offending task number.

#### REQ: rule-p-003-non-linear-numbering

`P-003` MUST report a violation when task numbering is not linear 1..N (gaps, duplicates, non-positive integers, or non-monotonic order). The violation message MUST cite the first offending task heading. Linear-1..N numbering is a precondition for the dependency graph; without it, dangling/cycle detection has no stable referent.

#### REQ: rule-p-003-not-autofixable

`P-003` MUST NOT be autofixable in the MVP. Resolving a cycle, dangling reference, or numbering gap requires user intent (rename the dependency vs. split the task vs. renumber).

### Lint rule P-004 — Stub placeholder body / posture-and-status validity

`P-004` covers placeholder-body validity in `stub` Plans plus schema-level validity of the new `**Mode:**` and `**Status:**` tokens (one rule covers all three because they share a single posture-aware code path).

#### REQ: rule-p-004-registered

`P-004` MUST be registered in the lint rule registry under the name `P-004`, at severity `error`, and MUST execute as part of the default rule suite.

#### REQ: rule-p-004-stub-placeholder-done

`P-004` MUST report a violation when, in a Plan with `**Mode:** stub`, a task with `**Status:** done` has a placeholder body marker (`<!-- implement: pending -->`) as its task body. The violation message MUST cite the offending task number and MUST reference both the placeholder rule (the upstream `REQ:posture-stub-placeholder`) and the writeback contract (the upstream `REQ:stub-placeholder-done-lint`) so the user knows where to look for the fix.

#### REQ: rule-p-004-stub-placeholder-not-done-permitted

In a Plan with `**Mode:** stub`, a placeholder body on a task whose `**Status:**` is `pending`, `in-progress`, or `blocked` MUST NOT trigger `P-004`. Placeholder bodies are exactly the case the `stub` posture exists to permit.

#### REQ: rule-p-004-invalid-mode-value

`P-004` MUST report a violation when `**Mode:**` is present with a value other than `full` or `stub`. The violation message MUST cite the offending line and the accepted value set.

#### REQ: rule-p-004-invalid-status-value

`P-004` MUST report a violation when a task's `**Status:**` field is present with a value other than `pending`, `in-progress`, `done`, or `blocked`. The violation message MUST cite the offending task number and the accepted value set.

#### REQ: rule-p-004-not-autofixable

`P-004` MUST NOT be autofixable in the MVP. Replacing a placeholder body with prose requires user intent (re-run `specstudio:implement` to write back the post-batch journal, vs. revert `**Status:**` to `pending`). Schema-token violations on `**Mode:**` and `**Status:**` are also user-driven (typo fix vs. value choice).

### Lint rule P-005 — Parent reference validity

`P-005` validates the optional `**Parent:**` body-metadata line that records master/sub-plan composition. Same-repo parents are resolved and checked for cycles; cross-repo parents are validated syntactically only.

#### REQ: plan-parent-field

The parser MUST recognize an optional `**Parent:** <plan-ref>` body-metadata line on a single-file Plan (in the header block, conventionally after `**Supersedes:**`). When the line is absent the Plan is a **root** plan and `P-005` MUST emit nothing for it. The parser MUST classify `<plan-ref>` by its single `:` separator: a value with **no** colon is a **same-repo** plan slug; a value with **exactly one** colon is a **cross-repo** reference of the form `<repo-slug>:<plan-slug>`.

#### REQ: rule-p-005-registered

`P-005` MUST be registered in the lint rule registry under the name `P-005` (uppercase, hyphenated), at severity `error`, and MUST execute as part of the default rule suite.

#### REQ: rule-p-005-same-repo-resolves

For a same-repo `**Parent:**` value, `P-005` MUST report a violation when no single-file Plan exists at `spec/plans/<plan-ref>.md`. The message MUST name the dangling parent slug and cite the Plan file path and the `**Parent:**` line.

#### REQ: rule-p-005-no-self-parent

`P-005` MUST report a violation when a Plan's same-repo `**Parent:**` value equals the Plan's own slug. The message MUST cite the Plan path and the `**Parent:**` line.

#### REQ: rule-p-005-acyclic

`P-005` MUST report a violation when same-repo `**Parent:**` links form a cycle (a Plan reachable as its own ancestor by following `**Parent:**`). The message MUST cite the cycle path in the form `plan-a → plan-b → … → plan-a`. A cross-repo `**Parent:**` terminates the chain (it is not followed) and therefore cannot participate in a detected cycle.

#### REQ: rule-p-005-cross-repo-syntactic-only

For a cross-repo `**Parent:**` value `<repo-slug>:<plan-slug>`, `P-005` MUST validate only the syntactic shape: exactly one `:` separator, with both `<repo-slug>` and `<plan-slug>` non-empty, lowercase, hyphen-separated, URL-safe slugs (`^[a-z0-9]+(?:-[a-z0-9]+)*$`). `P-005` MUST NOT resolve the reference, MUST NOT scan sibling repositories, and MUST NOT report a violation merely because the referenced repo or plan cannot be found in the workspace. A malformed cross-repo value (empty side, more than one `:`, or an invalid slug token on either side) MUST be a violation citing the offending value.

#### REQ: rule-p-005-not-autofixable

`P-005` MUST NOT be autofixable in the MVP. Resolving a dangling, self-referential, cyclic, or malformed parent reference requires user intent (rename the parent vs. remove the line vs. fix the cross-repo slug).

### Lint rule P-006 — Plan status vocabulary

`P-006` validates the single-file Plan's own body `**Status:**` document-status line against the canonical Plan status set defined by the upstream [Status Vocabulary](https://github.com/specscore/specscore/blob/main/spec/features/status-vocabulary/README.md) Feature (`REQ:per-artifact-status-sets`, Plan row). This is the Plan's *document* status (the header `**Status:**` line), distinct from a task's `**Status:**` field (the lowercase `pending`/`in-progress`/`done`/`blocked` set validated by `P-004`).

#### REQ: rule-p-006-registered

`P-006` MUST be registered in the lint rule registry under the name `P-006` (uppercase, hyphenated), at severity `error`, MUST execute as part of the default rule suite (per `cli/spec/lint#req:default-runs-all-rules`), and MUST NOT be autofixable (correcting a non-canonical status requires user intent — pick the right lifecycle value).

#### REQ: rule-p-006-plan-status-enum

`P-006` MUST report a violation when a single-file Plan's body `**Status:**` value is not one of the eleven canonical Plan statuses: `Draft`, `In Review`, `Approved`, `Executing`, `Blocked`, `Implemented`, `Failed`, `Rejected`, `Withdrawn`, `Superseded`, `Deprecated` (Title Case, single ASCII space between words, per the upstream `REQ:per-artifact-status-sets`). The comparison is exact and case-sensitive. The violation MUST name the offending value and the legal set, with `File` set to the Plan path and `Line` set to the `**Status:**` line. When the `**Status:**` line is absent, `P-006` MUST emit nothing (presence of the line is governed by other rules); only a present-but-out-of-set value is a `P-006` violation. Directory-form plans at `spec/plans/<slug>/README.md` MUST NOT be inspected by `P-006`.

### Co-existence with existing plan checkers

#### REQ: directory-plans-untouched

`P-001`–`P-005` MUST NOT inspect or report violations on directory-form plans at `spec/plans/<slug>/README.md`. The existing `plan-hierarchy` and `plan-roi-metadata` checkers continue to own that path. This isolation lets this repo's own directory-form plans coexist with single-file SpecStudio Plans without spurious violations from either rule set.

#### REQ: no-rule-overlap

`P-001`–`P-004` MUST NOT duplicate violations already surfaced by other registered rules (`adherence-footer`, `oq-section`, `heading-levels`, `internal-links`, etc.). When an issue is structurally covered by another rule (e.g., a missing `## Open Questions` section), the existing rule reports it; `P-001`–`P-004` operate only on Plan-specific semantics (AC coverage, AC reference validity, dependency-graph well-formedness, posture/status validity).

### Rule registration in `specscore spec lint`

#### REQ: rules-in-default-suite

`P-001`, `P-002`, `P-003`, `P-004`, `P-005`, and `P-006` MUST be added to the canonical rule-name set returned by `lint.AllRuleNames()` so that `--rules` and `--ignore` accept them and `--rules P-001` runs only that rule. They MUST execute under the default rule suite (per `cli/spec/lint#req:default-runs-all-rules`).

#### REQ: rules-emit-stable-violation-shape

Violations from `P-001`–`P-006` MUST use the existing `lint.Violation` struct (`File`, `Line`, `Severity`, `Rule`, `Message`). No new severity, no new fields. `File` is the Plan path relative to the spec root; `Line` is the line in the Plan where the violation surfaces (e.g., the offending task's `### Task N:` heading line for task-scoped findings, the `## Acceptance Criteria` AC heading line in the source Feature for `P-001` coverage gaps, the `**Source Feature:**` line for `P-002` missing-Feature violations).

## Acceptance Criteria

### AC: skip-non-plan-files (verifies REQ:plan-detection-single-file, REQ:plan-detection-title-prefix)

**Given** a project with `spec/plans/notes.md` whose first H1 is `# Random notes` (not a Plan title) and a directory-form plan at `spec/plans/legacy-plan/README.md`,
**When** `specscore spec lint` runs with no filters,
**Then** `P-001`–`P-004` emit zero violations against `notes.md` and zero violations against `legacy-plan/README.md`; existing rules (`plan-hierarchy`, `plan-roi-metadata`, `adherence-footer`) are unaffected.

### AC: coverage-gap-flagged (verifies REQ:rule-p-001-coverage-gap, REQ:rule-p-001-registered)

**Given** a source Feature with three ACs (`alpha`, `beta`, `gamma`) and a single-file Plan whose tasks' `**Verifies:**` lines cover `alpha` and `beta` only, with no `## Deferred AC Coverage` section,
**When** `specscore spec lint` runs,
**Then** lint exits non-zero and a single `P-001` violation is emitted naming `<feature-slug>#ac:gamma` as the uncovered AC, with `File` set to the Plan path.

### AC: deferred-ac-counts-as-covered (verifies REQ:rule-p-001-coverage-gap, REQ:deferred-ac-coverage-parse)

**Given** the same Feature/Plan as `coverage-gap-flagged`, but the Plan adds `## Deferred AC Coverage` with the entry `- <feature-slug>#ac:gamma — post-MVP scope`,
**When** `specscore spec lint` runs,
**Then** no `P-001` violation is emitted for `gamma`.

### AC: stale-ac-flagged (verifies REQ:rule-p-002-stale-reference, REQ:rule-p-002-registered)

**Given** a Plan whose task 2 declares `**Verifies:** <feature-slug>#ac:typo-slug` where no AC named `typo-slug` exists in the source Feature,
**When** `specscore spec lint` runs,
**Then** lint exits non-zero and a single `P-002` violation is emitted naming `<feature-slug>#ac:typo-slug`, with `File` set to the Plan path and `Line` set to the task's `### Task 2:` heading.

### AC: missing-source-feature (verifies REQ:rule-p-002-stale-reference)

**Given** a Plan declaring `**Source Feature:** does/not/exist` and three tasks each with `**Verifies:**` lines,
**When** `specscore spec lint` runs,
**Then** exactly one `P-002` violation is emitted citing the missing source Feature (not three violations — one per task `**Verifies:**` line).

### AC: cycle-detected-and-cited (verifies REQ:rule-p-003-cycle, REQ:rule-p-003-registered)

**Given** a Plan with task 1 declaring `**Depends-On:** 3`, task 2 declaring `**Depends-On:** 1`, and task 3 declaring `**Depends-On:** 2`,
**When** `specscore spec lint` runs,
**Then** lint exits non-zero and a `P-003` violation is emitted whose message contains a cycle path of the form `Task 1 → Task 3 → Task 2 → Task 1` (or any rotation thereof), with `File` set to the Plan path.

### AC: dangling-depends-on (verifies REQ:rule-p-003-dangling)

**Given** a Plan with four tasks numbered 1..4 where task 3 declares `**Depends-On:** 7`,
**When** `specscore spec lint` runs,
**Then** a `P-003` violation is emitted whose message contains `Task 3 depends on nonexistent task 7`.

### AC: self-reference-flagged (verifies REQ:rule-p-003-self-reference)

**Given** a Plan with task 2 declaring `**Depends-On:** 2`,
**When** `specscore spec lint` runs,
**Then** a `P-003` violation is emitted citing task 2 as a self-reference.

### AC: non-linear-numbering-flagged (verifies REQ:rule-p-003-non-linear-numbering)

**Given** a Plan whose `## Tasks` section contains headings `### Task 1:`, `### Task 3:`, `### Task 5:` (gaps in numbering),
**When** `specscore spec lint` runs,
**Then** a `P-003` violation is emitted citing the first offending heading (`### Task 3:` — the first heading not equal to its expected linear index).

### AC: stub-done-placeholder-flagged (verifies REQ:rule-p-004-stub-placeholder-done, REQ:rule-p-004-registered)

**Given** a Plan with `**Mode:** stub` and three tasks where task 2 has `**Status:** done` and the body `<!-- implement: pending -->`,
**When** `specscore spec lint` runs,
**Then** a `P-004` violation is emitted citing task 2, the placeholder rule (`REQ:posture-stub-placeholder`), and the writeback contract (`REQ:stub-placeholder-done-lint`).

### AC: stub-pending-placeholder-permitted (verifies REQ:rule-p-004-stub-placeholder-not-done-permitted, REQ:task-placeholder-body)

**Given** a Plan with `**Mode:** stub` and three tasks each with `**Status:** pending` and the body `<!-- implement: pending -->`,
**When** `specscore spec lint` runs,
**Then** no `P-004` violation is emitted for placeholder body presence; other rules are unaffected.

### AC: invalid-mode-value-flagged (verifies REQ:rule-p-004-invalid-mode-value, REQ:plan-mode-field)

**Given** a Plan whose header contains `**Mode:** sketch` (an unrecognized value),
**When** `specscore spec lint` runs,
**Then** a `P-004` violation is emitted citing the offending line and naming the accepted value set (`full`, `stub`).

### AC: invalid-status-value-flagged (verifies REQ:rule-p-004-invalid-status-value, REQ:task-status-field)

**Given** a Plan with a task declaring `**Status:** waiting` (an unrecognized value),
**When** `specscore spec lint` runs,
**Then** a `P-004` violation is emitted citing the offending task number and the accepted value set (`pending`, `in-progress`, `done`, `blocked`).

### AC: defaults-when-fields-absent (verifies REQ:plan-mode-field, REQ:task-status-field, REQ:task-depends-on-field)

**Given** a Plan that omits `**Mode:**` from the header and a task that omits `**Status:**` and `**Depends-On:**`,
**When** `specscore spec lint` runs,
**Then** the parser treats the Plan as `**Mode:** full`, the task as `**Status:** pending`, and the task as `**Depends-On:** —`; no `P-004` or `P-003` violations are emitted on the basis of those absent fields alone.

### AC: rules-in-default-suite (verifies REQ:rules-in-default-suite, REQ:rules-emit-stable-violation-shape)

**Given** a project with single-file Plans containing one `P-001`, one `P-002`, one `P-003`, one `P-004`, and one `P-005` violation,
**When** `specscore spec lint` runs with no `--rules` filter,
**Then** exactly five violations are reported (one per rule), each violation's `Rule` field equals the rule name (`P-001`, `P-002`, `P-003`, `P-004`, `P-005`), each violation's `Severity` is `error`, and `specscore spec lint --rules P-005` returns only the parent-reference violation.

### AC: root-plan-no-parent (verifies REQ:plan-parent-field, REQ:rule-p-005-registered)

**Given** a single-file Plan with no `**Parent:**` line,
**When** `specscore spec lint` runs,
**Then** `P-005` emits zero violations for that Plan (it is a root plan).

### AC: dangling-parent-flagged (verifies REQ:rule-p-005-same-repo-resolves)

**Given** a Plan `sub.md` declaring `**Parent:** ghost` where `spec/plans/ghost.md` does not exist,
**When** `specscore spec lint` runs,
**Then** a `P-005` violation is emitted naming `ghost` as a dangling parent, with `File` set to `sub.md`'s path and `Line` at the `**Parent:**` line.

### AC: self-parent-flagged (verifies REQ:rule-p-005-no-self-parent)

**Given** a Plan `loop.md` declaring `**Parent:** loop`,
**When** `specscore spec lint` runs,
**Then** a `P-005` violation is emitted citing `loop` as its own parent.

### AC: parent-cycle-flagged (verifies REQ:rule-p-005-acyclic)

**Given** plans `a.md` declaring `**Parent:** b` and `b.md` declaring `**Parent:** a`,
**When** `specscore spec lint` runs,
**Then** a `P-005` violation is emitted whose message contains a cycle path of the form `a → b → a` (or any rotation thereof).

### AC: cross-repo-parent-accepted (verifies REQ:rule-p-005-cross-repo-syntactic-only, REQ:plan-parent-field)

**Given** a Plan `sub.md` declaring `**Parent:** specscore:master-rollout` and no sibling repo is scanned,
**When** `specscore spec lint` runs,
**Then** `P-005` emits no violation — the cross-repo reference is accepted on its syntax alone, with no resolution or sibling-repo scan.

### AC: malformed-cross-repo-parent-flagged (verifies REQ:rule-p-005-cross-repo-syntactic-only)

**Given** a Plan declaring `**Parent:** Bad_Repo:` (uppercase/underscore repo token and an empty plan side),
**When** `specscore spec lint` runs,
**Then** a `P-005` violation is emitted citing the malformed `**Parent:**` value.

### AC: plan-status-enum-flagged (verifies REQ:rule-p-006-plan-status-enum, REQ:rule-p-006-registered)

**Given** a single-file Plan whose body `**Status:**` line reads `Completed` (a value outside the canonical Plan status set),
**When** `specscore spec lint` runs,
**Then** lint exits non-zero and a single `P-006` violation is emitted naming `Completed` and the legal set, with `File` set to the Plan path and `Line` at the `**Status:**` line.

### AC: plan-status-enum-accepts-canonical (verifies REQ:rule-p-006-plan-status-enum)

**Given** a single-file Plan whose body `**Status:**` line reads `Executing` (a canonical Plan status),
**When** `specscore spec lint` runs,
**Then** `P-006` emits zero violations for that Plan.

### AC: directory-plans-untouched (verifies REQ:directory-plans-untouched, REQ:no-rule-overlap)

**Given** a project containing both a directory-form plan at `spec/plans/legacy/README.md` (with the historical schema this repo uses) and a single-file Plan at `spec/plans/new-plan.md` (with the SpecStudio schema),
**When** `specscore spec lint` runs,
**Then** `P-001`–`P-004` emit zero violations against `legacy/README.md`, and existing plan checkers (`plan-hierarchy`, `plan-roi-metadata`) emit zero violations against `new-plan.md`.

### AC: not-autofixable (verifies REQ:rule-p-001-not-autofixable, REQ:rule-p-002-not-autofixable, REQ:rule-p-003-not-autofixable, REQ:rule-p-004-not-autofixable, REQ:rule-p-005-not-autofixable)

**Given** a project with `P-001`, `P-002`, `P-003`, `P-004`, and `P-005` violations,
**When** `specscore spec lint --fix` runs,
**Then** no file under `spec/plans/` is modified by these five rules (other autofixable rules may still run), and the violations are still reported on the post-`--fix` lint pass.

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [Spec Lint](../README.md) | These rules register into the same rule registry and execute under the default rule suite. `--rules` / `--ignore` filtering applies per the parent Feature's contract. |
| [Feature](../../../feature/README.md) | `P-001` and `P-002` read each source Feature's `## Acceptance Criteria` section to enumerate ACs and validate references. The AC heading grammar (`### AC: <ac-slug>`) is owned by the Feature schema, not this Feature. |
| [Status Vocabulary](https://github.com/specscore/specscore/blob/main/spec/features/status-vocabulary/README.md) | Owns the canonical Plan status set (`REQ:per-artifact-status-sets`, Plan row) that `P-006` enforces. Any change to the legal Plan statuses MUST land upstream first; `P-006` is the downstream enforcement. |
| [SpecStudio `plan` Feature](https://github.com/specscore/specstudio-skills/blob/main/spec/features/skills/plan/README.md) | Locks the upstream contract for `P-001`–`P-004`, the `**Mode:**` / `**Status:**` / `**Depends-On:**` task fields, and the placeholder body token. Any change to that contract MUST land in the upstream Feature first; this CLI Feature is the downstream implementation. |
| [SpecStudio `implement` Idea](https://github.com/specscore/specstudio-skills/blob/main/spec/ideas/specstudio-implement-skill.md) | Hard-blocks on these rules and parser extensions. `specstudio:implement` cannot ship until this Feature ships. |

## Open Questions

- **Placeholder body token (working decision).** The upstream `plan` Feature lists three candidates: `<!-- implement: pending -->` (HTML comment, invisible in rendered markdown, machine-friendly), `**Implementation:** _pending_` (visible, scannable), and `_to be journaled by `implement`_` (visible, self-documenting). This Feature picks `<!-- implement: pending -->` for the MVP because (a) HTML comments are invisible in rendered Plans (zero visual noise in stub Plans the user only ever interacts with through `implement`), (b) the token is byte-exact and unambiguous to parse, and (c) the convention is well-established in the markdown ecosystem. The upstream Outstanding Question remains open; if the SpecStudio team selects a different token, this Feature MUST revise the parser before `specstudio:implement` ships.
- **`P-002` for the case where the source Feature exists but its AC list is empty.** Today every Feature MUST have at least one AC per `cli/feature/README.md`, so this is structurally impossible — but if that requirement is ever relaxed, `P-001` against an AC-less source Feature would silently pass for all-deferred Plans. Revisit if the Feature-AC-required rule changes.
- **Cross-Feature AC references.** The current contract restricts `**Verifies:**` AC IDs to the Plan's declared source Feature (single-Feature scope). A future Idea may relax this (multi-Feature Plans for roadmap work); the parser and lint rules would need updates to follow the relaxation.

---
*This document follows the https://specscore.md/feature-specification*
