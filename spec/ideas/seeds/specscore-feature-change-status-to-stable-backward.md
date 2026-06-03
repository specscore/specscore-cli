---
type: sidekick-seed
slug: specscore-feature-change-status-to-stable-backward
captured_at: 2026-05-22T00:00:00Z
captured_by: user
captured_during: null
trigger: explicit
status: queued
synchestra_task: null
---

# specscore feature change-status --to=stable backward-transitions Source Idea Implementing → Specified

**Action:** `specscore feature change-status <feature> --to=stable` where the Feature's Source Idea is in `Implementing`.

**Observed:** Feature transitions `Implementing → Stable` AND the Source Idea transitions `Implementing → Specified` (backward). The `spec/ideas/README.md` index is updated to match. Lint stays clean (0 violations).

**Why suspicious:** `Specified` is *behind* `Implementing` in the Idea lifecycle. The cascade is either an undocumented sync rule that should be surfaced in the verb's `--help` and stderr, or a bug in the `spec lint --fix` follow-up step the verb runs after the Status rewrite.

**Repro:** Today's specstudio-skills commit `b62457e` (sidekick-capture/destination-resolution `Implementing → Stable`). The Source Idea `spec/ideas/idea-skills-destination-resolution.md` and `spec/ideas/README.md` both transitioned alongside the Feature.

**Suggested investigation:** identify whether the cascade is in (a) the verb itself, (b) the shared `spec lint --fix` engine that derives Idea status from referencing-Feature status, or (c) some other auto-sync hook. Then either document the rule (and surface it in the verb's stderr — "also transitioned Source Idea <slug>: <from> → <to>") or remove it.

---

**Triage 2026-06-03:** The *literally reported* scenario does NOT reproduce at HEAD — a single `Stable` ref derives the Idea to `Implemented` (forward), not `Specified`. The cascade lives in the `lint --fix` follow-up step (b): `runFeatureChangeStatus` (`internal/cli/feature.go:893`) runs `lint --fix` → `ideaSyncRules` (`pkg/lint/idea.go:662`).

**However, a real latent bug remains OPEN:** the "all Approved → Specified" else-branch (`idea.go:782-784`) silently also fires for `Deprecated` refs. So mixed refs `{Stable, Deprecated}` with an Idea at `Implementing` derive it **backward to `Specified`**. The existing test `TestCheckIdeas_DeprecatedFeatureGivesSpecified` bakes in this behavior — its assertion may itself be the bug. **Needs a spec/REQ decision on intended `Deprecated`-ref handling in the derivation matrix before coding a fix** (likely: treat `Deprecated` like `Stable` — "past Implementing").
