---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: AC Heading Format

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/ac-heading-format?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/ac-heading-format?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/ac-heading-format?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/ac-heading-format?op=request-change) |

**Status:** Approved
**Date:** 2026-07-22
**Owner:** alexander.trakhimenok
**Source Ideas:** —
**Supersedes:** —

## Summary

Adds the `ac-heading-format` lint rule, enforcing the canonical Acceptance Criteria heading shape `### AC: <id>` (exactly one space after the colon, kebab-case id) on Feature README files — the Chatwright/Sneat downstream convention. `--fix` repairs whitespace-only deviations; an invalid id or unrecognized trailing text is reported but never auto-fixed, since both require a human decision. A pre-existing `(verifies REQ:...)` trailing annotation — already used across this repo's own spec tree and the upstream `specscore` meta-spec repo, and already recognized as a named construct by the Plan-rules AC parser — is grandfathered as a second accepted form, with only its surrounding whitespace subject to the same formatting checks.

## Problem

No rule previously caught defects in `### AC: <id>` headings: a missing space after the colon, doubled spaces, an uppercase or underscored id, or stray trailing text all passed `spec lint` silently. Downstream repos adopting the stricter Chatwright/Sneat convention had no mechanical way to catch or fix drift.

## Behavior

### AC heading format

#### REQ: rule-registered

The rule MUST be registered in the lint rule registry under the name `ac-heading-format` (lowercase, hyphenated), at severity `error`, MUST execute as part of the default rule suite, and MUST be accepted by `--rules` / `--ignore` and `lint.AllRuleNames()`.

#### REQ: trigger-scope

The rule MUST examine every line of every Feature README (`spec/features/**/README.md` recognized by its `# Feature: <title>` H1) that matches `^###\s*AC\s*:` (three hashes, optional whitespace, the literal `AC`, optional whitespace, a colon) — however malformed — as an attempted AC heading.

#### REQ: canonical-form

A heading MUST be reported as a violation unless it is exactly `### AC: <id>`, where `<id>` matches `[a-z0-9]+(-[a-z0-9]+)*`, or that form followed by exactly one space and a `(verifies ...)` annotation (REQ:verifies-grandfather). A violation MUST cite the file path and the 1-based line number of the offending heading.

#### REQ: verifies-grandfather

A heading of the form `### AC: <id> (verifies ...)` MUST be accepted without violation when its whitespace already matches canonical spacing — exactly one space after `AC:` and exactly one space before the parenthetical. This mirrors the pre-existing `(verifies REQ:...)` annotation the Plan-rules AC parser (`cli/spec/lint/plan-rules`) already treats as a named, parseable construct, and which is used throughout this repo and the upstream `specscore` meta-spec repo. It is not part of the Chatwright/Sneat convention this rule enforces, but this rule MUST NOT break it.

#### REQ: autofix-whitespace-only

`specscore spec lint --fix` MUST rewrite a heading to its canonical form (`### AC: <id>`, or `### AC: <id> (verifies ...)` when a grandfathered annotation is present) when the only defect is whitespace formatting around `###`, `AC`, the colon, or the id/parenthetical boundary, and the extracted id is already valid kebab-case. Fixes MUST be idempotent, and the modified file MUST be reported in the `--fix` summary like every other autofix.

#### REQ: no-autofix-for-id-or-trailing

`--fix` MUST NOT modify a heading whose extracted id is not valid kebab-case (uppercase letters, underscores, or other non-kebab characters), or whose trailing content is not the grandfathered `(verifies ...)` annotation. Both cases require a human decision — what the correct id is, or whether the trailing text is meaningful — and MUST remain reported as violations after `--fix` runs.

## Acceptance Criteria

### AC: canonical-heading-accepted

**Requirements:** cli/spec/lint/ac-heading-format#req:canonical-form

**Given** a Feature README with heading `### AC: valid-id`,
**When** `specscore spec lint` runs,
**Then** no `ac-heading-format` violation is emitted.

### AC: missing-space-flagged

**Requirements:** cli/spec/lint/ac-heading-format#req:canonical-form, cli/spec/lint/ac-heading-format#req:trigger-scope

**Given** a Feature README with heading `### AC:x`,
**When** `specscore spec lint` runs,
**Then** an `ac-heading-format` violation is emitted citing the file and line.

### AC: non-kebab-id-flagged-not-fixed

**Requirements:** cli/spec/lint/ac-heading-format#req:no-autofix-for-id-or-trailing

**Given** a Feature README with heading `### AC: My_Id`,
**When** `specscore spec lint --fix` runs,
**Then** an `ac-heading-format` violation citing the invalid id is still reported and the line is left byte-for-byte unchanged.

### AC: trailing-text-flagged-not-fixed

**Requirements:** cli/spec/lint/ac-heading-format#req:no-autofix-for-id-or-trailing

**Given** a Feature README with heading `### AC: valid-id and some other words`,
**When** `specscore spec lint --fix` runs,
**Then** an `ac-heading-format` violation citing the trailing content is still reported and the line is left byte-for-byte unchanged.

### AC: verifies-annotation-accepted

**Requirements:** cli/spec/lint/ac-heading-format#req:verifies-grandfather

**Given** a Feature README with heading `### AC: valid-id (verifies REQ:something)`,
**When** `specscore spec lint` runs,
**Then** no `ac-heading-format` violation is emitted.

### AC: whitespace-autofix-normalizes

**Requirements:** cli/spec/lint/ac-heading-format#req:autofix-whitespace-only

**Given** a Feature README with heading `###  AC:  valid-id`,
**When** `specscore spec lint --fix` runs,
**Then** the heading is rewritten to `### AC: valid-id`, the file is reported in the `--fix` summary, and a subsequent `specscore spec lint` reports no `ac-heading-format` violation.

## Open Questions

- Whether to eventually retire the grandfathered `(verifies ...)` annotation across this repo and the upstream meta-spec repo (replacing it with the plain `**Requirements:**` line, which already carries the same traceability) is a separate migration decision, out of scope here.

---
*This document follows the https://specscore.md/feature-specification*
