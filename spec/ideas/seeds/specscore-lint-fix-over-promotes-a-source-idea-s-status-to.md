---
type: sidekick-seed
slug: specscore-lint-fix-over-promotes-a-source-idea-s-status-to
captured_at: 2026-05-29T10:56:46Z
captured_by: user
captured_during: null
trigger: explicit
status: queued
synchestra_task: null
---
# specscore lint --fix over-promotes a source Idea's Status to Implementing when its Feature is only Approved

Observed during the canonical-grade-metadata-field work in the specscore meta-spec repo: after a Feature declared the Idea via `**Source Ideas:**`, `specscore lint --fix` reconciled the Idea's `**Status:**` to `Implementing` while the referencing Feature was only `Draft`/`Approved` (never `Implementing`).

Per the documented idea-status derivation (referencing Feature at `Approved` ⇒ Idea `Specified`; at `Implementing` ⇒ Idea `Implementing`), an `Approved` Feature should derive the Idea's Status as `Specified`, not `Implementing`. Investigate the idea-sync derivation mapping in the `lint --fix` reconciliation path.
