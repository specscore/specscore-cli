---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: Storage Layout Lint Rules

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/storage-layout?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/storage-layout?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/storage-layout?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/spec/lint/storage-layout?op=request-change) |

**Status:** Draft
**Date:** 2026-11-13
**Owner:** alexander.trakhimenok
**Source Ideas:** —
**Supersedes:** —

## Summary

Adds three opt-in lint rules — `feature-storage-layout` (error), `feature-storage-layout-pending` (warning) and `feature-storage-layout-existing` (info, advisory) — that make a Feature state its data-storage layout before code is written: one `## Storage layout` section that either says the Feature stores nothing, or lists its records together with an `**Approved by:**` line naming who approved them. A repository turns the rules on with `storage_layout.required: true` in `specscore.yaml`; repositories that do not are unaffected.

## Problem

Data-storage layouts are designed in Feature specifications, but nothing in a Feature says what it stores or whether a human has seen that layout. AI agents can design and build a layout of many collections that its owner never reviewed, and every stored read and write costs money. A Feature should not reach Approved without its layout written down and approved by a person, and a repository should be able to enforce that mechanically.

## Behavior

### Opt-in and selection

#### REQ: rules-registered

The rules MUST be registered in the lint rule registry under the family `feature`: `feature-storage-layout` at severity `error`, `feature-storage-layout-pending` at severity `warning`, and `feature-storage-layout-existing` at severity `info`, marked advisory (shown at `--severity info`, never fails a run). All MUST be accepted by `--rules`, `--ignore` and `lint.AllRuleNames()`.

#### REQ: opt-in

The rules MUST report nothing unless the project's `specscore.yaml` carries:

```yaml
storage_layout:
  required: true
  guide_url: https://example.com/storage-design   # optional
  existing_until: 2026-11-13                      # optional, see existing-layouts
```

A missing block, `required: false`, or an unreadable `specscore.yaml` MUST leave the rules off, so that a repository's default `specscore spec lint` result does not change when the CLI gains them. Naming one of the rules in `--rules`, or `feature-storage-layout` in `--fix=`, MUST run the rules even without the block, so a repository can try them before opting in.

#### REQ: config-shape

A present `storage_layout` block that is malformed MUST be reported as one `feature-storage-layout` error citing `specscore.yaml`, whether or not it would have turned the rules on, and the rules and their fix MUST NOT run while it stands. Malformed means: not a mapping; `required` not a YAML boolean; `guide_url` not an `http://` or `https://` URL; `existing_until` not a `YYYY-MM-DD` date; or an unknown key (unknown keys are still preserved in the parsed config). A malformed block MUST NOT make the rest of `specscore.yaml` unreadable.

#### REQ: guide-url

When `storage_layout.guide_url` is set, every per-Feature finding's message MUST end with ` See <guide_url>`, and the section skeleton MUST cite it. When it is not set, messages carry no link. Each message MUST be a single line that tells the author what to do.

### The section

#### REQ: section-required

Every Feature README (`spec/features/**/README.md` recognized by its `# Feature: <title>` H1, outside `_`-prefixed directories) whose `**Status:**` is not `Rejected` or `Deprecated` MUST contain exactly one level-2 heading `## Storage layout` (case-insensitive). Headings inside fenced code blocks or HTML comments do not count. The section runs to the next level-1 or level-2 heading or a `---` line. HTML comments and blank lines in the section are ignored; lines inside fenced code blocks count as content but never as an approval line. A missing section is a `feature-storage-layout` error citing the `**Status:**` line (line 1 when there is none), with the message:

> Feature has no `## Storage layout` section; add one that says `None — this feature stores nothing.` or lists the stored records with an `**Approved by:**` line (`specscore spec lint --fix=feature-storage-layout` inserts a skeleton).

An empty section, and a second `## Storage layout` heading, are each a `feature-storage-layout` error.

#### REQ: stores-nothing

A section whose ONLY content line is `None — this feature stores nothing.` satisfies the rules. The comparison MUST ignore case and the trailing period, and MUST accept an en dash or one or more ASCII hyphens in place of the em dash. A section that contains that statement together with any other content (records, an approval line) is a contradiction and MUST be a `feature-storage-layout` error.

#### REQ: approval-line

Any other section MUST contain a line beginning `**Approved by:**` (case-insensitive; a leading `- `, `* ` or `+ ` list marker is allowed) followed by free text saying who approved the layout and when. A missing line, or a section whose only content is the approval line (no layout described), is a `feature-storage-layout` error.

The value is **not an approval** when, lower-cased, trimmed and with one trailing period removed, it is one of the closed placeholder set `` (empty), `—`, `–`, `-`, `none`, `n/a`, `na`, `tbd`, `tba`, `todo`, `self`, `?`, `unknown`, `nobody`; or when it begins with `pending` (so `pending review` is pending); or when it begins with `not reviewed` and is not an admitted existing layout (req:existing-layouts). The set lives in one place in the code (`storageLayoutPlaceholders`). Whether the named approver is really a human cannot be linted; that stays with the process rule that requires the approval.

#### REQ: pending

A value that is not an approval is graded by the Feature's status. At `Draft`, `In Review` or `Amending` it MUST be a `feature-storage-layout-pending` warning citing the approval line, with the message:

> storage layout not yet approved (`**Approved by:** pending`); show it to a human and record their approval before the Feature is Approved.

(the backticked part quotes the actual value). At every other non-exempt status — `Approved`, `Implementing`, `Stable`, and any unknown, misspelled or missing status, which fails closed and is named as unrecognized — it MUST be a `feature-storage-layout` error. Because the default `--severity` is `error`, a pending Draft does not fail a default run.

#### REQ: existing-layouts

When `storage_layout.existing_until: YYYY-MM-DD` is configured, an approval value of the form `not reviewed — existing layout, recorded YYYY-MM-DD` (em dash, en dash or ASCII hyphens; case-insensitive; optional trailing period) whose date is a real date on or before `existing_until` passes at any status, provided the section also describes the layout, and MUST be reported as a `feature-storage-layout-existing` info finding: `existing storage layout recorded, not reviewed by a human; …`. So `specscore spec lint --severity info` lists every such Feature. Without `existing_until`, or with a later date, the value is not an approval (req:pending), and the message says why. The skeleton and the fix MUST never write this form.

### Autofix and scaffold

#### REQ: autofix-skeleton

The fix is opt-in, like `no-source`: only `specscore spec lint --fix=feature-storage-layout` runs it; a bare `--fix` and `--fix=all` MUST NOT insert anything, and neither may any other command's internal fix pass (`feature new`, `feature change-status`). When named, it MUST insert a section skeleton into every non-exempt Feature README that has no `## Storage layout` section and leave every other file byte-for-byte unchanged. The skeleton is the heading, an HTML comment holding the instructions (show the layout to a human; or replace the body with the stores-nothing statement; the guide URL when configured; example records with paths; existing records considered for reuse; reads and writes per user action and behaviour at very large scale), and `**Approved by:** pending`. It is placed before `## Acceptance Criteria`, else before `## Open Questions`, else before the adherence footer, else at the end of the file. A file that ends inside an unclosed fenced code block is left unchanged (an insert there would be invisible and repeat on every run). A CRLF file keeps CRLF line endings. The fix is idempotent.

In an opted-in repository, `specscore feature new` MUST add the same skeleton to the README it creates, and to no other file.

#### REQ: change-status-rollback

When `specscore feature change-status` rolls back because its post-rewrite lint fails, every regular file under `spec/` MUST be left exactly as it was before the command ran — including files the command's `--fix` pass rewrote, such as the features index row — not only the Feature's `**Status:**` line.

## Acceptance Criteria

### AC: off-without-opt-in

**Requirements:** cli/spec/lint/storage-layout#req:opt-in

**Given** a project without a `storage_layout` block and a Stable Feature with no `## Storage layout` section,
**When** `specscore spec lint --severity info --fix` runs,
**Then** no `feature-storage-layout*` finding is reported and no file gains the section.

### AC: explicit-selection-runs

**Requirements:** cli/spec/lint/storage-layout#req:opt-in, cli/spec/lint/storage-layout#req:rules-registered

**Given** the same project,
**When** `specscore spec lint --rules feature-storage-layout` runs, and separately `specscore spec lint --fix=feature-storage-layout`,
**Then** the first reports one `feature-storage-layout` error carrying the `feature-storage-layout` fix target, and the second inserts the skeleton.

### AC: malformed-config-visible

**Requirements:** cli/spec/lint/storage-layout#req:config-shape

**Given** a `storage_layout` block with `required: "true"`, or `storage_layout: true`, or `requird: true`, or a non-URL `guide_url`,
**When** `specscore spec lint` runs,
**Then** one `feature-storage-layout` error citing `specscore.yaml` is reported and the fix changes nothing.

### AC: missing-section-flagged

**Requirements:** cli/spec/lint/storage-layout#req:section-required, cli/spec/lint/storage-layout#req:guide-url

**Given** an opted-in project with `guide_url` set and a Draft Feature without the section (or with the section only inside a code fence or HTML comment),
**When** `specscore spec lint` runs,
**Then** one `feature-storage-layout` error cites the Feature's `**Status:**` line and its one-line message ends with ` See <guide_url>`.

### AC: stores-nothing-alone

**Requirements:** cli/spec/lint/storage-layout#req:stores-nothing

**Given** an opted-in project and Stable Features whose sections read only `None — this feature stores nothing.`, `None - this feature stores nothing.` and `none -- this feature stores nothing`, and a fourth whose section has that statement followed by a record list,
**When** `specscore spec lint` runs,
**Then** the first three are not reported and the fourth is a `feature-storage-layout` contradiction error.

### AC: placeholders-not-approval

**Requirements:** cli/spec/lint/storage-layout#req:approval-line, cli/spec/lint/storage-layout#req:pending

**Given** an opted-in project and Features listing a record with `**Approved by:**` set to each placeholder (`—`, `n/a`, `TBD`, `todo`, `self`, `?`, empty, …) or `pending review`,
**When** `specscore spec lint --severity warning` runs,
**Then** each is a `feature-storage-layout` error when the Feature is Approved and a `feature-storage-layout-pending` warning when it is a Draft, and a Feature listing records with `**Approved by:** "yes" — owner, 2026-11-13` is not reported.

### AC: status-fails-closed

**Requirements:** cli/spec/lint/storage-layout#req:pending

**Given** an opted-in project and Features carrying `**Approved by:** pending` with statuses Draft, In Review, Amending, Approved, Implementing, Stable, `Aproved` and no status,
**When** `specscore spec lint --severity warning` runs,
**Then** the first three are warnings, and the rest are `feature-storage-layout` errors, the last two named as unrecognized statuses.

### AC: existing-layouts-gated

**Requirements:** cli/spec/lint/storage-layout#req:existing-layouts

**Given** an opted-in project with `existing_until: 2026-11-13` and a Stable Feature listing records with `**Approved by:** not reviewed — existing layout, recorded 2026-11-13`,
**When** `specscore spec lint --severity info` runs,
**Then** one advisory `feature-storage-layout-existing` info finding is listed and the run's exit status is not affected; with a recorded date of 2026-11-14, or without `existing_until`, the same Feature is a `feature-storage-layout` error.

### AC: exempt-statuses

**Requirements:** cli/spec/lint/storage-layout#req:section-required

**Given** an opted-in project and Rejected and Deprecated Features without the section,
**When** `specscore spec lint` runs and `specscore spec lint --fix=feature-storage-layout` runs,
**Then** neither is reported and neither file changes.

### AC: fix-only-when-named

**Requirements:** cli/spec/lint/storage-layout#req:autofix-skeleton

**Given** an opted-in project and a Draft Feature without the section,
**When** `specscore spec lint --fix` and `--fix=all` run, then `specscore spec lint --fix=feature-storage-layout` runs twice,
**Then** the first two change nothing, the third inserts the skeleton before `## Acceptance Criteria` and reports the file as fixed, the Feature then carries only the pending warning, and the fourth changes nothing.

### AC: scaffold-touches-only-new-readme

**Requirements:** cli/spec/lint/storage-layout#req:autofix-skeleton

**Given** an opted-in project with existing Features that lack the section,
**When** `specscore feature new --title Rooms --description "Video rooms"` runs,
**Then** the new README carries the skeleton and every other file except the features index (which gains the new row) is byte-identical.

### AC: failed-change-status-restores-tree

**Requirements:** cli/spec/lint/storage-layout#req:change-status-rollback

**Given** an opted-in project whose Implementing Feature lacks the section,
**When** `specscore feature change-status <draft-feature> --to "In Review"` fails its post-fix lint,
**Then** every file under `spec/`, including the features index, is byte-identical to before.

## Open Questions

- **Config key shape.** The opt-in lives in a top-level `storage_layout:` block, following the `parked:` and `grade:` precedent. A general `lint:` block that turns any rule on per repository may be preferable if more opt-in rules follow. The nearest related existing concept is the `gates: feature.approved: reviewers:` list with a `type: human` reviewer; whether the storage approval should eventually be recorded through that gate instead of a free-text line is open.
- **Grandfathering inside an opted-in repository.** Opting in reports every existing Feature that lacks the section. `existing_until` offers an honest "recorded, not reviewed" state for layouts that predate the rule; whether a repository uses it is the owner's decision.

---
*This document follows the https://specscore.md/feature-specification*
