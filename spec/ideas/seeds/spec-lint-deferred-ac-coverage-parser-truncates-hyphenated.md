---
type: sidekick-seed
slug: spec-lint-deferred-ac-coverage-parser-truncates-hyphenated
captured_at: 2026-06-03T09:59:29Z
captured_by: user
captured_during: spec/plans/approval-autonomy.md
trigger: explicit
status: archived
superseded_by: deferred-ac-coverage-regex-truncates-hyphenated-ac-slugs
synchestra_task: null
---

# spec-lint Deferred AC Coverage parser truncates hyphenated AC slugs at first hyphen

Observed in `specscore` 0.5.0 while implementing the specstudio-skills approval-autonomy Plan. The `## Deferred AC Coverage` section parser appears to tokenize an AC reference only up to the first hyphen: `reviewer-gates#ac:when-condition-masks-by-branch` is read as `reviewer-gates#ac:when`, so a legitimately-deferred multi-hyphen AC still trips `P-001` (AC coverage gap). The deferred-coverage route is therefore unusable for any AC slug containing a hyphen, forcing a task `**Verifies:**` line as the only lint-clean way to account for such an AC. Fix: make the Deferred-AC slug tokenizer consume the full `<feature-slug>#ac:<ac-slug>` token including interior hyphens, matching how the `**Verifies:**` parser already handles them. Source workspace: specstudio-skills (cross-repo capture); cross-repo back-link write skipped (best-effort).
