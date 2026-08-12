---
format: https://specscore.md/plan-specification
status: Approved
---

# Plan: Idea Relocate Implementation

**Status:** Approved
**Source Feature:** cli/idea/relocate
**Date:** 2026-05-21
**Owner:** alexandertrakhimenok
**Supersedes:** —

## Summary

Implements the `specscore idea relocate <slug> --to-repo=<target>` CLI verb specified in [`cli/idea/relocate`](../features/cli/idea/relocate/README.md). Decomposes the verb's 19 acceptance criteria into seven ordered tasks: scaffolding + slug/target resolution, pre-flight clean-tree checks, the mutation phase (file copy + in-file rewrite + collision check), cross-repo link cleanup, commit semantics, pre-commit rollback, and end-to-end integration coverage.

## Approach

Linear task ordering matches the verb's runtime sequence: resolve inputs (slug + target) → verify clean trees → mutate → propagate updates → commit → handle failure. The two happy-path ACs are intentionally deferred to the final task (E2E) because they exercise the whole flow end-to-end and are only meaningful once every earlier task lands. All 19 source-Feature ACs are covered by at least one task; no ACs are deferred.

## Tasks

### Task 1: Scaffold verb + slug-and-target resolution

**Verifies:** cli/idea/relocate#ac:ambiguous-slug-rejected, cli/idea/relocate#ac:slug-not-found, cli/idea/relocate#ac:to-repo-slug-form-resolves-via-sibling-scan, cli/idea/relocate#ac:to-repo-path-form-bypasses-scan, cli/idea/relocate#ac:to-repo-without-specscore-yaml-rejected, cli/idea/relocate#ac:to-repo-slug-multiple-matches-rejected

Register the `relocate` subcommand under `specscore idea` in `internal/cli/idea.go`. Parse the `<slug>` positional, `--to-repo` (required), `--no-commit` (optional), and `--project` (inherited) flags. Implement slug auto-detection (Idea-first at `spec/ideas/<slug>.md`, then seed at `spec/ideas/seeds/<slug>.md`; both → exit 5; neither → exit 3). Implement target-repo resolution: value with no `/` is a slug resolved via `find ../ -maxdepth 2 -name specscore.yaml` and matched against each candidate's `project.repo`; value with `/` is a path. Validate that the resolved target contains a `specscore.yaml`; zero matches or missing yaml → exit 6; multiple slug matches → exit 2.

### Task 2: Pre-flight clean-tree checks

**Verifies:** cli/idea/relocate#ac:preflight-source-dirty, cli/idea/relocate#ac:preflight-sibling-with-references-dirty

Before any mutation, verify clean git working trees in source (the artifact path), target (the destination path), and every sibling SpecScore repo whose `spec/**/*.md` contains references to the relocated slug. Implement the sibling-reference scan: search for bold-prefixed metadata (`**Source Ideas:**`, `**Related Ideas:**`, `**Supersedes:**`, `**Promotes To:**`) whose value contains the slug, and markdown links ending in `<slug>.md`. Any dirty path in any affected repo → exit 7 with a stderr message naming each repo + path. No mutations on failure.

### Task 3: File copy + in-file rewrite + destination-collision check

**Verifies:** cli/idea/relocate#ac:destination-collision, cli/idea/relocate#ac:in-file-rewrite-org-rename, cli/idea/relocate#ac:in-file-rewrite-this-repo

If the destination path in the target repo already has a file at the same relative location → exit 1 (Conflict), no mutations. Otherwise: copy the artifact to the target (creating parent dirs), apply substitution rules in-place on the copied file — rewrite every `specscore/<repo>` to `specscore/<repo>`, and rewrite word-bounded "this repo" (case-insensitive, in body prose only — NOT inside fenced code blocks, inline code spans, or table cells) to the target's `project.repo` value. Delete the artifact from the source path.

### Task 4: Cross-repo link cleanup

**Verifies:** cli/idea/relocate#ac:cross-repo-link-cleanup-markdown-link, cli/idea/relocate#ac:cross-repo-link-cleanup-preserves-slug-metadata

After the file is in its new home, scan the source repo, target repo, and every sibling SpecScore-managed repo for markdown links whose target ends in `<slug>.md` and rewrite each to point at the new location — relative path when the referencing file is in the same repo as the new artifact, full GitHub URL form (`https://github.com/<target-org>/<target-repo>/blob/main/spec/ideas/<slug>.md` or `…/seeds/<slug>.md`) when crossing repos. Bold-prefixed slug-only metadata references (`**Source Ideas:** <slug>`) are NOT modified — slugs are durable identifiers.

### Task 5: Commit semantics — auto-commit, `--no-commit`, stop-on-first-failure

**Verifies:** cli/idea/relocate#ac:auto-commit-three-repo-flow, cli/idea/relocate#ac:no-commit-flag-stages-everywhere, cli/idea/relocate#ac:commit-failure-mid-flight

Implement the post-mutation commit phase. Without `--no-commit`: commit each affected repo in order (source, target, then siblings alphabetically by `project.repo`) with canonical subject lines, capturing each commit's SHA for stdout. On any commit failure mid-flight: stop, exit 10, and print to stderr the committed-repos+SHAs, the failing repo+reason, the unprocessed repos, and exact `git -C <repo> reset HEAD~1 --hard` rollback commands. With `--no-commit`: stage every affected path via `git add` in each repo, no commits; stdout lists each repo and its staged paths.

### Task 6: Pre-commit-phase rollback

**Verifies:** cli/idea/relocate#ac:io-failure-rollback-pre-commit

Note on sequence: this task implements the failure path that fires **before** Task 5's commit phase begins (file copy I/O failure, in-file rewrite failure, link-update failure). Task 5's mid-flight commit-failure path is separate and does NOT roll back. The two failure paths share the exit code (`10`) but operate at different phases.

If any mutation fails before the commit phase begins, restore on-disk state to its pre-invocation form: restore the source artifact at its original path if it was deleted, delete the partial copy at target if it was created, and `git -C <sibling> checkout -- <paths>` to revert any in-flight link updates. Exit 10 (IOFailure) with a stderr message naming the failed step and the rollback actions performed.

### Task 7: End-to-end integration tests

**Verifies:** cli/idea/relocate#ac:idea-relocate-happy-path, cli/idea/relocate#ac:seed-relocate-happy-path

Single integration test exercising both happy paths against a temp workspace fixture: a parent dir with `specstudio-skills/` (containing `spec/ideas/foo.md` for the Idea path, and `spec/ideas/seeds/bar.md` for the seed path), `specscore/` (target, empty), and `specscore-cli/` (sibling with a Feature containing a markdown link to the Idea). Run `specscore idea relocate foo --to-repo=specscore` and `specscore idea relocate bar --to-repo=specscore`, assert exit 0 in both cases, verify file presence at new location, verify per-repo commits with canonical subject lines, verify stdout per-repo lines + summary, verify any cross-repo links got rewritten. Final state: `specscore spec lint --project <tmpdir>/specstudio-skills` and `--project <tmpdir>/specscore` both return 0 violations.

## Open Questions

- Should each task in this Plan be implemented behind an experimental flag in the CLI verb (e.g., `--experimental` gating sibling-scan, or a build-tag gating the whole subcommand) so the verb can ship behind a flag per the source Idea's cross-repo sequencing strategy? Recommended position: yes — register the subcommand but emit a "experimental" notice in `--help` and on first invocation; promote to GA in a follow-on plan after `specstudio-skills` ships its consumer skill. Resolve at implementation time.
- What test-fixture infrastructure exists in the repo for "create N temp git repos, each with `specscore.yaml`"? If none, Task 1 needs to first author a tiny test helper (~50 lines) before any pre-flight or cross-repo AC can be exercised. Resolve in Task 1.
- The cross-repo link-rewrite logic in Task 4 needs to know each affected repo's `project.org` value (from its `specscore.yaml`) to compose full GitHub URLs. Multiple repos with stale/different `org` values (e.g., `specscore` vs `specscore`) will produce mixed URLs in the same cleanup pass — by design, since each repo's truth is its own yaml. Worth noting in the implementation but not a Plan-level decision.

---
*This document follows the https://specscore.md/plan-specification*
