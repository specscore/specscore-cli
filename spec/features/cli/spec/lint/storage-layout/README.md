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

Adds two opt-in lint rules, `feature-storage-layout` (error) and `feature-storage-layout-pending` (warning), that make a Feature state its data-storage layout before code is written: a `## Storage layout` section that either says the Feature stores nothing, or lists its records together with an `**Approved by:**` line naming the human who approved them. A repository turns the rules on with `storage_layout.required: true` in `specscore.yaml`; repositories that do not are unaffected.

## Problem

Data-storage layouts are designed in Feature specifications, but nothing in a Feature says what it stores or whether a human has seen that layout. AI agents can design and build a layout of many collections that its owner never reviewed, and every stored read and write costs money. A Feature should not reach Approved without its layout written down and approved by a person, and a repository should be able to enforce that mechanically.

## Behavior

### Opt-in and selection

#### REQ: rules-registered

Both rules MUST be registered in the lint rule registry under the family `feature`: `feature-storage-layout` at severity `error` and `feature-storage-layout-pending` at severity `warning`. Both MUST be accepted by `--rules`, `--ignore` and `lint.AllRuleNames()`.

#### REQ: opt-in

The rules MUST report nothing unless the project's `specscore.yaml` carries:

```yaml
storage_layout:
  required: true
  guide_url: https://example.com/storage-design   # optional
```

A missing block, `required: false`, or an unreadable `specscore.yaml` MUST leave the rules off, so that a repository's default `specscore spec lint` result does not change when the CLI gains them. Naming either rule explicitly in `--rules` MUST run the rules even without the block, so a repository can try them before opting in.

#### REQ: guide-url

When `storage_layout.guide_url` is set, every finding's message MUST end with ` See <guide_url>`, and the section skeleton inserted by `--fix` MUST cite it. When it is not set, messages carry no link. Each message MUST be a single line that tells the author what to do.

### The section

#### REQ: section-required

Every Feature README (`spec/features/**/README.md` recognized by its `# Feature: <title>` H1, outside `_`-prefixed directories) whose `**Status:**` is not `Rejected` or `Deprecated` MUST contain a level-2 heading `## Storage layout` (case-insensitive, outside fenced code blocks). The section runs to the next level-1 or level-2 heading or a `---` line. HTML comments and blank lines in the section are ignored. A missing section is a `feature-storage-layout` error citing the `**Status:**` line (line 1 when there is none), with the message:

> Feature has no `## Storage layout` section; add one that says `None — this feature stores nothing.` or lists the stored records with an `**Approved by:**` line (`specscore spec lint --fix` inserts a skeleton).

An empty section is a `feature-storage-layout` error.

#### REQ: stores-nothing

A section whose first content line is `None — this feature stores nothing.` satisfies the rules. The comparison MUST ignore case and the trailing period, and MUST accept an en dash or one or more ASCII hyphens in place of the em dash.

#### REQ: approval-line

Any other section MUST contain a line beginning `**Approved by:**` (case-insensitive) followed by free text saying who approved the layout and when. A missing line, an empty value, or a section whose only content is the approval line (no layout described) is a `feature-storage-layout` error.

#### REQ: pending

A value beginning with `pending` (case-insensitive) records that the layout has not been approved yet. On a Feature whose status is `Approved`, `Implementing` or `Stable` it MUST be a `feature-storage-layout` error. On any other non-exempt status it MUST be a `feature-storage-layout-pending` warning citing the approval line, with the message:

> storage layout not yet approved (`**Approved by:** pending`); show it to a human and record their approval before the Feature is Approved.

Because the default `--severity` is `error`, a pending Draft does not fail a default run.

### Autofix

#### REQ: autofix-skeleton

When the rules are on, `specscore spec lint --fix` (bare, `--fix=all`, or `--fix=feature-storage-layout`) MUST insert a section skeleton into every non-exempt Feature README that has no `## Storage layout` section, and MUST leave every other file byte-for-byte unchanged. The skeleton is the heading, an HTML comment holding the instructions (show the layout to a human; or replace the body with the stores-nothing statement; the guide URL when configured; example records with paths; existing records considered for reuse; reads and writes per user action and behaviour at very large scale), and `**Approved by:** pending`. It is placed before `## Acceptance Criteria`, else before `## Open Questions`, else before the adherence footer, else at the end of the file. The fix is idempotent. Because `specscore feature new` runs the `--fix` pass on the scaffold it writes, a Feature scaffolded in an opted-in repository carries the skeleton.

## Acceptance Criteria

### AC: off-without-opt-in

**Requirements:** cli/spec/lint/storage-layout#req:opt-in

**Given** a project without a `storage_layout` block and a Stable Feature with no `## Storage layout` section,
**When** `specscore spec lint --severity info` runs,
**Then** no `feature-storage-layout*` finding is reported.

### AC: explicit-selection-runs

**Requirements:** cli/spec/lint/storage-layout#req:opt-in, cli/spec/lint/storage-layout#req:rules-registered

**Given** the same project,
**When** `specscore spec lint --rules feature-storage-layout` runs,
**Then** one `feature-storage-layout` error is reported for the Feature, carrying the `feature-storage-layout` fix target.

### AC: missing-section-flagged

**Requirements:** cli/spec/lint/storage-layout#req:section-required, cli/spec/lint/storage-layout#req:guide-url

**Given** an opted-in project with `guide_url` set and a Draft Feature without the section,
**When** `specscore spec lint` runs,
**Then** one `feature-storage-layout` error cites the Feature's `**Status:**` line and its one-line message ends with ` See <guide_url>`.

### AC: stores-nothing-accepted

**Requirements:** cli/spec/lint/storage-layout#req:stores-nothing

**Given** an opted-in project and Stable Features whose sections read `None — this feature stores nothing.`, `None - this feature stores nothing.` and `none -- this feature stores nothing`,
**When** `specscore spec lint` runs,
**Then** none of them is reported.

### AC: approval-required

**Requirements:** cli/spec/lint/storage-layout#req:approval-line

**Given** an opted-in project and Features whose sections list records but carry no `**Approved by:**` line, carry an empty one, or carry only an approval line,
**When** `specscore spec lint` runs,
**Then** each is reported as a `feature-storage-layout` error; a Feature listing records with `**Approved by:** "yes" — owner, 2026-11-13` is not reported.

### AC: pending-by-status

**Requirements:** cli/spec/lint/storage-layout#req:pending

**Given** an opted-in project and Features carrying `**Approved by:** pending` with statuses Draft, In Review, Amending, Approved, Implementing and Stable,
**When** `specscore spec lint --severity warning` runs,
**Then** the first three are `feature-storage-layout-pending` warnings, the last three are `feature-storage-layout` errors, and a default (`--severity error`) run reports only the errors.

### AC: exempt-statuses

**Requirements:** cli/spec/lint/storage-layout#req:section-required

**Given** an opted-in project and Rejected and Deprecated Features without the section,
**When** `specscore spec lint` runs and `specscore spec lint --fix` runs,
**Then** neither is reported and neither file changes.

### AC: fix-inserts-skeleton

**Requirements:** cli/spec/lint/storage-layout#req:autofix-skeleton

**Given** an opted-in project and a Draft Feature without the section,
**When** `specscore spec lint --fix=feature-storage-layout` runs twice,
**Then** the first run inserts the skeleton before `## Acceptance Criteria` and reports the file as fixed, the Feature then carries only the pending warning, and the second run changes nothing.

## Open Questions

- **Config key shape.** The opt-in lives in a top-level `storage_layout:` block, following the `parked:` and `grade:` precedent. A general `lint:` block that turns any rule on per repository may be preferable if more opt-in rules follow.
- **Grandfathering inside an opted-in repository.** Opting in reports every existing Feature that lacks the section; `--fix` turns those into pending skeletons (warnings for not-yet-approved Features, errors for Approved and later ones). Whether to offer a cut-over date or an exemption list instead is open.

---
*This document follows the https://specscore.md/feature-specification*
