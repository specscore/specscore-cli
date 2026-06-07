---
captured_by: user
status: resolved
---

# specscore lint --fix over-promotes a source Idea's Status to Implementing when its Feature is only Approved

> **Triage 2026-06-03:** Does NOT reproduce at HEAD. `ideaSyncRules` derivation (`pkg/lint/idea.go:783`) maps all-`Approved` refs → `Specified`; `Implementing` is only reached when a referenced Feature is literally `Implementing`. Pinned by existing test `TestCheckIdeas_DerivesSpecified_WhenAllFeaturesApproved` (green). Likely fixed since capture, or observed against an older installed binary.

Observed during the canonical-grade-metadata-field work in the specscore meta-spec repo: after a Feature declared the Idea via `**Source Ideas:**`, `specscore lint --fix` reconciled the Idea's `**Status:**` to `Implementing` while the referencing Feature was only `Draft`/`Approved` (never `Implementing`).

Per the documented idea-status derivation (referencing Feature at `Approved` ⇒ Idea `Specified`; at `Implementing` ⇒ Idea `Implementing`), an `Approved` Feature should derive the Idea's Status as `Specified`, not `Implementing`. Investigate the idea-sync derivation mapping in the `lint --fix` reconciliation path.
