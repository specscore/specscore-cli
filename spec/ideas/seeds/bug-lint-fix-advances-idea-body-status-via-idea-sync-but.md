---
captured_by: user
status: resolved
---
# Bug: lint --fix advances Idea body Status via idea-sync but leaves frontmatter mirror stale, is non-convergent, needs migrate to reconcile

**Resolved 2026-06-16** — `rewriteIdeaHeader` (`pkg/lint/idea.go`) now syncs the leading frontmatter `status:` mirror from the body `**Status:**` whenever the idea-sync auto-advance rewrites it, reusing the canonical `setFrontmatterStatus` helper. A single `lint --fix` pass is convergent again; the body stays canonical and a missing frontmatter block is still left for `migrate`. Regression test: `TestCheckIdeas_SyncFixUpdatesFrontmatterMirror`.

GitHub issue: https://github.com/specscore/specscore-cli/issues/68

Reproduced on specscore CLI v0.10.1 while specifying a Feature whose **Source Ideas:** referenced an Approved Idea.

Steps:
1. Idea is Approved (frontmatter status: Approved, body **Status:** Approved, index in sync).
2. Create a Feature referencing the Idea -> 'specscore spec lint' reports idea-sync-lint-strict (Promotes To / Status disagree with referencing features).
3. Run 'specscore spec lint --fix'. It advances the Idea BODY **Status:** to 'Specifying' (and sets Promotes To) but does NOT update the frontmatter 'status:' mirror, which stays 'Approved'.
4. Re-lint now fails with status-mirror (frontmatter Approved vs body Specifying) -- i.e. --fix INTRODUCED a violation.
5. Running 'lint --fix' again does NOT converge (still 1 violation). Only 'specscore migrate' reconciles the frontmatter mirror.

Bugs:
- 'lint --fix' is non-idempotent/non-convergent: it should leave the tree lint-clean, not in a state only 'migrate' can repair.
- '--fix' applies the status-mirror rule in some paths but not when its own idea-sync auto-advance changes body status.

Minor papercut: 'idea change-status' refuses when body and frontmatter disagree (reads body as source of truth) and offers no way to reconcile the mirror -- user must know about 'migrate'.
