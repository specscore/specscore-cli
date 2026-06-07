---
captured_by: user
status: resolved
---

# specscore feature change-status --to=stable backward-transitions Source Idea Implementing → Specified

**Action:** `specscore feature change-status <feature> --to=stable` where the Feature's Source Idea is in `Implementing`.

**Observed:** Feature transitions `Implementing → Stable` AND the Source Idea transitions `Implementing → Specified` (backward). The `spec/ideas/README.md` index is updated to match. Lint stays clean (0 violations).

**Why suspicious:** `Specified` is *behind* `Implementing` in the Idea lifecycle. The cascade is either an undocumented sync rule that should be surfaced in the verb's `--help` and stderr, or a bug in the `spec lint --fix` follow-up step the verb runs after the Status rewrite.

**Repro:** Today's specstudio-skills commit `b62457e` (sidekick-capture/destination-resolution `Implementing → Stable`). The Source Idea `spec/ideas/idea-skills-destination-resolution.md` and `spec/ideas/README.md` both transitioned alongside the Feature.

**Suggested investigation:** identify whether the cascade is in (a) the verb itself, (b) the shared `spec lint --fix` engine that derives Idea status from referencing-Feature status, or (c) some other auto-sync hook. Then either document the rule (and surface it in the verb's stderr — "also transitioned Source Idea <slug>: <from> → <to>") or remove it.

---

**Triage 2026-06-03:** Reported scenario does NOT reproduce at HEAD — a single `Stable` ref derives the Idea to `Implemented` (forward), not `Specified`. Cascade is in the `lint --fix` follow-up (`ideaSyncRules`, `pkg/lint/idea.go:662`).

**Resolved 2026-06-03:** the latent bug (mixed `{Stable, Deprecated}` refs derived an `Implementing` Idea backward to `Specified`) is fixed by treating `Deprecated` like `Stable` in the derivation — a post-Stable "done" state contributes to the `Implemented` level. Spec (`idea-sync-lint-strict`) and tests updated.
