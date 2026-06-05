---
format: https://specscore.md/feature-specification
status: Implementing
---

# Feature: Idea Relocate

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/idea/relocate?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/idea/relocate?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/idea/relocate?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/idea/relocate?op=request-change) |
>
> **AI skill:** _planned_ — `specstudio:relocate-idea` (sibling Feature in [`specscore/specstudio-skills`](https://github.com/specscore/specstudio-skills)) will wrap this verb as a thin shell and surface it as `/relocate-idea` for slash-trigger discoverability.

**Status:** Implementing
**Source Ideas:** —

<!-- The Source Idea idea-skills-destination-resolution lives in specscore/specstudio-skills, not in this repo.
     `specscore spec lint` rule `idea-feature-cross-reference` only resolves local Idea slugs, so we set the
     body-metadata line to — and document the cross-repo origin in the Interaction with Other Features section
     via a full GitHub URL. Tracked as a Specstudio gap in
     https://github.com/specscore/specstudio-skills/blob/main/spec/ideas/seeds/cross-repo-source-idea-references-should-be-supported-by-lin.md -->


## Summary

`specscore idea relocate <slug> --to-repo=<target>` relocates an Idea or sidekick-seed artifact from the current project to a different SpecScore-managed repo. Automates the full manual ritual: pre-flight clean-tree check, file copy with cross-repo reference rewrite, cross-repo link cleanup in every sibling SpecScore repo whose docs reference the artifact, and auto-commit per affected repo with cross-linking commit messages.

## Synopsis

```
specscore idea relocate <slug> --to-repo=<target> [--no-commit] [--project <path>]
```

## Problem

When an Idea-creation surface (most commonly `specstudio:sidekick`, occasionally `specstudio:ideate` user-error) writes an artifact into the wrong repo in a multi-repo SpecScore workspace, the manual relocation ritual is ~30 minutes of careful, error-prone work. The 2026-05-20 relocate of `artifact-frontmatter-convention` from `specstudio-skills` to `specscore/specscore` (commits [`7e32851`](https://github.com/specscore/specscore/commit/7e32851), [`160ae03`](https://github.com/specscore/specstudio-skills/commit/160ae03)) is the worked reference: copy the file, rewrite stale `specscore/*` org references to `specscore/*`, disambiguate "this repo" wording, then commit on both sides with cross-linking messages. In a workspace where other repos reference the artifact via `**Source Ideas:**`, `**Related Ideas:**`, `**Supersedes:**`, or back-link sections, the user would also need to update those references across every sibling repo and commit each. This verb collapses all of that into one command.

## Behavior

### Slug resolution

#### REQ: slug-resolves-idea-or-seed

The `<slug>` positional argument MUST resolve to an artifact file via lookup at one of two paths in the source project, in order:

1. `spec/ideas/<slug>.md` (Idea form), checked first
2. `spec/ideas/seeds/<slug>.md` (seed form), checked second

If exactly one path exists, that file is the artifact. If both paths exist for the same slug, the verb MUST exit `5` (AmbiguousSlug) with a stderr message naming both paths and instructing the user to disambiguate by renaming. If neither path exists, the verb MUST exit `3` (NotFound).

### Target repo resolution

#### REQ: target-repo-resolution

The `--to-repo=<target>` flag MUST accept either a **repo slug** or a **path**, disambiguated by the presence of `/`:

- Value containing no `/` is a repo slug. Resolution: scan sibling directories of the source project's parent (`find ../ -maxdepth 2 -name specscore.yaml` semantics) and match against each candidate's `project.repo` field in `specscore.yaml`. Unique match wins. Zero matches exits `3` (NotFound). Multiple matches exits `2` (InvalidArgs) with a stderr message naming each match.
- Value containing `/` is a path. Resolution: treat as relative to the source project root (or absolute if starting with `/`). The path MUST be a directory.

In either case, the resolved target directory MUST contain a `specscore.yaml`. A target directory without `specscore.yaml` MUST exit `6` (TargetNotSpecScore) with a stderr message naming the path.

### Pre-flight clean-tree checks

#### REQ: preflight-clean-tree-source-and-target

Before any mutation, the verb MUST verify that both the source project and the target repo have a clean git working tree in the paths that will be modified. Concretely:

- Source: the artifact file (`spec/ideas/<slug>.md` or `spec/ideas/seeds/<slug>.md`) MUST have no uncommitted modifications. The corresponding `spec/ideas/README.md` (which will get an index update on commit-phase via `spec lint --fix` semantics in the source) MUST also be clean.
- Target: the destination path (same relative location as the source) MUST not exist as an uncommitted file, and the target's `spec/ideas/README.md` MUST be clean.

If either repo has uncommitted changes in those paths, the verb MUST exit `7` (DirtyTree) with a stderr message naming each affected repo and path. No mutations.

#### REQ: preflight-clean-tree-siblings-with-references

Before any mutation, the verb MUST discover all sibling SpecScore-managed repos (siblings of the source project's parent containing `specscore.yaml`), scan each for references to the relocated artifact, and verify that any sibling with discovered references has a clean working tree in the files that would be modified. References to search in each markdown file under `spec/**/*.md` of each sibling:

- Bold-prefixed metadata lines whose value contains the slug: `**Source Ideas:** <slug>`, `**Related Ideas:** <slug>`, `**Supersedes:** <slug>`, `**Promotes To:** <slug>`.
- Markdown links of the form `[...](<path-ending-in>/<slug>.md)`.

If any such sibling has uncommitted changes in the affected files, the verb MUST exit `7` (DirtyTree) with a stderr message naming each affected sibling repo and path. No mutations.

### Destination collision

#### REQ: destination-collision-rejected

If a file already exists at the destination path in the target repo (`<target>/spec/ideas/<slug>.md` for an Idea or `<target>/spec/ideas/seeds/<slug>.md` for a seed), the verb MUST exit `1` (Conflict) with a stderr message naming the collision path. No mutations. User-resolvable by renaming the source artifact or the target's existing file.

### File copy and in-file rewrite

#### REQ: file-copy-with-rewrite

After pre-flight passes and no collision exists, the verb MUST:

1. Copy the artifact file from the source path to the destination path in the target repo, creating any missing parent directories.
2. In the copied file, apply substitution rules:
   - Every occurrence of `specscore/<repo>` MUST be rewritten to `specscore/<repo>` for any `<repo>` value (org rename from the pre-2026 era).
   - Every standalone occurrence of the phrase `this repo` (case-insensitive, word-bounded) in body prose MUST be rewritten to the target repo's slug (the value of `project.repo` in its `specscore.yaml`). Occurrences inside fenced code blocks (` ``` `), inline code spans (` ` `` ` `), or table cells MUST NOT be rewritten — disambiguation in those contexts is the user's responsibility post-relocate.
3. Delete the artifact file from the source path.

The artifact's body metadata (`**Status:**`, `**Date:**`, `**Owner:**`, etc.) MUST NOT be modified beyond the substitution rules above.

### Cross-repo link cleanup

#### REQ: cross-repo-link-cleanup

After the file is copied to the target and deleted from the source, the verb MUST update references to the relocated artifact across the source repo, the target repo, and every sibling repo whose pre-flight scan returned hits. For each affected file:

- Bold-prefixed metadata lines (`**Source Ideas:**`, `**Related Ideas:**`, `**Supersedes:**`, `**Promotes To:**`) MUST NOT be modified — slugs are durable identifiers and references by slug remain valid after relocation.
- Markdown links of the form `[<text>](<path-ending-in>/<slug>.md)` MUST be updated so that the link target points at the new location:
  - When the referencing file is in the same repo as the new location, the rewritten link uses a relative path computed from the referencing file's directory to the new artifact path.
  - When the referencing file is in a different repo than the new location, the rewritten link uses the full GitHub URL form: `https://github.com/<target-org>/<target-repo>/blob/main/spec/ideas/<slug>.md` (or `…/spec/ideas/seeds/<slug>.md` for seeds). The `<target-org>` and `<target-repo>` are read from the target repo's `specscore.yaml` `project.org` and `project.repo` fields.

The `spec/ideas/README.md` indexes in both source and target are NOT updated by this verb directly — they are auto-managed by `specscore spec lint --fix` semantics, which the user MAY run post-relocate.

### Commit semantics

#### REQ: auto-commit-default

By default (no `--no-commit` flag), the verb MUST commit changes on each affected repo as soon as that repo's mutations complete. The order is: source first, target second, then siblings in alphabetical order by `project.repo` slug. Each commit's subject line MUST be exactly:

- Source: `chore(relocate): move <kind> <slug> to <target-repo-slug>`
- Target: `chore(relocate): receive <kind> <slug> from <source-repo-slug>`
- Sibling: `chore(relocate): update links for <slug> (<source-repo-slug> → <target-repo-slug>)`

Where `<kind>` is `idea` or `seed`, and `<source-repo-slug>` / `<target-repo-slug>` are the `project.repo` values from each repo's `specscore.yaml`. The commit message body MAY include cross-references to the other affected repos' commit SHAs once they are known (best-effort; for the target commit, source SHA is known; for siblings, both source and target SHAs are known).

The verb MUST capture and report each commit's resulting SHA in its stdout per [REQ: stdout-format](#req-stdout-format).

#### REQ: no-commit-flag

When `--no-commit` is supplied, the verb MUST perform every mutation (file copy, file delete, in-file rewrites, cross-repo link updates) and MUST stage the affected paths in each affected repo (`git add` on each touched file), but MUST NOT commit. The verb's stdout MUST list each affected repo and the paths it staged, so the user can `git -C <repo> commit` per repo after review.

**Pre-flight is still enforced under `--no-commit`.** The clean-tree checks ([REQ: preflight-clean-tree-source-and-target](#req-preflight-clean-tree-source-and-target) and [REQ: preflight-clean-tree-siblings-with-references](#req-preflight-clean-tree-siblings-with-references)) MUST run identically whether or not `--no-commit` is supplied. The flag relaxes the commit step only — staging mutations on top of pre-existing uncommitted changes in the same paths would produce a confused `git status` and defeat the review-before-commit goal of `--no-commit`.

### Failure handling

#### REQ: stop-on-first-commit-failure

If a commit fails in any affected repo during the auto-commit phase (e.g., pre-commit hook rejection, locked index, or any non-zero exit from `git commit`), the verb MUST:

1. Stop processing remaining repos. Do NOT attempt later commits.
2. Exit `10` (CommitFailed).
3. Write to stderr: repos already committed (with their SHAs), the failing repo and its failure reason (stderr from `git commit`), and any unprocessed sibling repos (with their pending mutations still applied in-tree but not committed).
4. Print exact rollback commands for the user: `git -C <repo-path> reset HEAD~1 --hard` for each already-committed repo, and `git -C <repo-path> reset HEAD && git -C <repo-path> checkout -- .` for the failed repo to discard its in-tree mutations.

The verb MUST NOT roll back automatically; cross-repo rollback is the user's responsibility per the source Idea's "git has no distributed transactions" rationale.

#### REQ: rollback-pre-commit-mutations

If any mutation fails before any commit phase begins (file I/O failure during copy, in-file rewrite failure, link-update failure in any affected repo), the verb MUST restore the on-disk state to its pre-invocation form before exiting `10` (IOFailure):

- Source: restore the artifact file at its original path if it was already deleted.
- Target: delete the partial copy if it was already created.
- Affected siblings: revert any in-flight link updates via `git -C <sibling> checkout -- <paths>`.

Stderr MUST name the failed step and the rollback actions performed.

### Output format

#### REQ: stdout-format

On success (exit `0`), the verb MUST write to stdout one line per affected repo, in commit order (source, target, siblings alphabetically), of the form:

```
<repo-slug>: <action> <kind> <slug>  [<commit-sha-7chars>]
```

Where `<action>` is one of `moved` (source), `received` (target), or `updated-links` (sibling), and `[<commit-sha-7chars>]` is the 7-character abbreviated commit SHA when commits ran (omitted entirely when `--no-commit`). Two spaces precede the bracketed SHA.

After the per-repo lines, exactly one summary line:

```
relocate complete: <N> repos affected
```

Where `<N>` is the count of affected repos.

## Parameters

| Name | Required | Description |
|---|---|---|
| `slug` | Yes | Idea or seed slug. Auto-resolved to `spec/ideas/<slug>.md` first, falling back to `spec/ideas/seeds/<slug>.md`. Ambiguity (both exist) exits `5`. |

## Flags

| Flag | Required | Description |
|---|---|---|
| `--to-repo` | Yes | Target SpecScore repo. Slug form (no `/`) resolves via sibling-dir scan against each candidate's `specscore.yaml` `project.repo`. Path form (contains `/`) resolves as relative to project root, or absolute. |
| `--no-commit` | No | Stage all mutations across affected repos without committing. User commits manually per repo. |
| `--project` | No | Source project root. Autodetected per [CLI#req:project-autodetect](../../README.md#req-project-autodetect). |

## Exit codes

| Code | Condition |
|---|---|
| `0` | Relocation succeeded; affected repos committed (or staged with `--no-commit`). |
| `1` | Destination collision: file already exists at the destination path in the target repo. |
| `2` | InvalidArgs: missing `<slug>`, missing `--to-repo`, unrecognized flag, or `--to-repo` slug-form matches multiple sibling repos. |
| `3` | NotFound: source artifact path doesn't exist (neither Idea nor seed location), or `--to-repo` slug-form matches no sibling. |
| `5` | AmbiguousSlug: slug exists at both `spec/ideas/<slug>.md` and `spec/ideas/seeds/<slug>.md` in the source project. |
| `6` | TargetNotSpecScore: target directory exists but contains no `specscore.yaml`. |
| `7` | DirtyTree: source, target, or some sibling repo with discovered references has uncommitted changes in the paths to be modified. |
| `10` | I/O failure (rollback applied) or commit failure (stop-on-first-failure; partial commits remain). |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [idea (CLI group)](../README.md) | Parent group; Contents table includes this sub-feature. |
| [cli/idea/change-status](../change-status/README.md) | Sibling verb. Both verbs mutate the on-disk location of an Idea artifact: `change-status --to=archived` moves the file within-repo to `spec/ideas/archived/`; `relocate` moves the file across repos. Different scopes, different exit-code spaces. |
| [cli/spec/lint](../../spec/lint/README.md) | NOT invoked by this verb. The user runs `specscore spec lint` (optionally `--fix`) post-relocate in any repo where they want lint-clean index reconciliation. The `--no-commit` workflow naturally leaves time for this between staging and committing. |
| Source Idea: [idea-skills-destination-resolution](https://github.com/specscore/specstudio-skills/blob/main/spec/ideas/idea-skills-destination-resolution.md) | The Idea that specifies this verb's purpose, scope, and key assumptions. **Cross-repo Source-Idea reference** — the Idea lives in `specscore/specstudio-skills`, not in `specscore-cli`. Full GitHub URL is used because relative-path resolution does not span repos. |
| [`specstudio:relocate-idea` skill](https://github.com/specscore/specstudio-skills) (sibling Feature, post-MVP) | Thin shell over this verb. Surfaces `/relocate-idea` slash-trigger and propagates the verb's stdout/stderr. Pins a CLI version per the source Idea's cross-repo sequencing. |

## Acceptance Criteria

### AC: idea-relocate-happy-path
**Requirements:** [#req:slug-resolves-idea-or-seed](#req-slug-resolves-idea-or-seed), [#req:target-repo-resolution](#req-target-repo-resolution), [#req:file-copy-with-rewrite](#req-file-copy-with-rewrite), [#req:auto-commit-default](#req-auto-commit-default), [#req:stdout-format](#req-stdout-format)

Given a workspace at `~/projects/specscore/` with two repos — `specstudio-skills` (source, containing `spec/ideas/foo.md`) and `specscore` (target, no such file) — and both repos have clean working trees, running `specscore idea relocate foo --to-repo=specscore` from `specstudio-skills` exits `0`, removes `spec/ideas/foo.md` from `specstudio-skills`, creates `spec/ideas/foo.md` in `specscore/` with any `specscore/specscore` substrings rewritten to `specscore/specscore`, commits each repo with the canonical subject-line format, and prints two per-repo lines (`specstudio-skills: moved idea foo  [<sha>]`, `specscore: received idea foo  [<sha>]`) plus the summary line `relocate complete: 2 repos affected`.

### AC: seed-relocate-happy-path
**Requirements:** [#req:slug-resolves-idea-or-seed](#req-slug-resolves-idea-or-seed), [#req:file-copy-with-rewrite](#req-file-copy-with-rewrite)

Given `spec/ideas/seeds/foo.md` exists in source (and `spec/ideas/foo.md` does NOT exist), running `specscore idea relocate foo --to-repo=specscore` resolves to the seed path, copies the artifact to `<target>/spec/ideas/seeds/foo.md` (creating the `seeds/` directory if absent), and reports `<kind>` as `seed` in the per-repo stdout lines.

### AC: ambiguous-slug-rejected
**Requirements:** [#req:slug-resolves-idea-or-seed](#req-slug-resolves-idea-or-seed)

Given both `spec/ideas/foo.md` AND `spec/ideas/seeds/foo.md` exist in source with the same slug `foo`, running `specscore idea relocate foo --to-repo=specscore` exits `5` (AmbiguousSlug). Stderr names both paths. No mutations.

### AC: slug-not-found
**Requirements:** [#req:slug-resolves-idea-or-seed](#req-slug-resolves-idea-or-seed)

Running `specscore idea relocate nonexistent --to-repo=specscore` where neither `spec/ideas/nonexistent.md` nor `spec/ideas/seeds/nonexistent.md` exists in source exits `3` (NotFound). No mutations.

### AC: to-repo-slug-form-resolves-via-sibling-scan
**Requirements:** [#req:target-repo-resolution](#req-target-repo-resolution)

Given a workspace `~/projects/specscore/{specstudio-skills,specscore,specscore-cli}/` where each repo's `specscore.yaml` has `project.repo` matching the directory name, running `specscore idea relocate foo --to-repo=specscore` from inside `specstudio-skills` resolves the target to `~/projects/specscore/specscore`. The value `specscore` contains no `/`, triggering sibling-dir scan.

### AC: to-repo-path-form-bypasses-scan
**Requirements:** [#req:target-repo-resolution](#req-target-repo-resolution)

Running `specscore idea relocate foo --to-repo=../specscore` from `specstudio-skills` resolves the target via the literal path, with no sibling-dir scan. Same outcome if the path is absolute (`--to-repo=/Users/x/projects/specscore/specscore`).

### AC: to-repo-without-specscore-yaml-rejected
**Requirements:** [#req:target-repo-resolution](#req-target-repo-resolution)

Running `specscore idea relocate foo --to-repo=/tmp/empty-dir` where `/tmp/empty-dir` exists as a directory but contains no `specscore.yaml` exits `6` (TargetNotSpecScore) with a stderr message naming the path.

### AC: to-repo-slug-multiple-matches-rejected
**Requirements:** [#req:target-repo-resolution](#req-target-repo-resolution)

Given a misconfigured workspace where two sibling directories each have `specscore.yaml` declaring `project.repo: specscore` (a collision that violates SpecScore conventions), running `specscore idea relocate foo --to-repo=specscore` exits `2` (InvalidArgs) with a stderr message naming each matching repo and instructing the user to either fix the collision or use path-form `--to-repo=<path>`.

### AC: preflight-source-dirty
**Requirements:** [#req:preflight-clean-tree-source-and-target](#req-preflight-clean-tree-source-and-target)

Given `spec/ideas/foo.md` exists in source and has unstaged modifications (e.g., the user edited the Idea body but hasn't committed), running `specscore idea relocate foo --to-repo=specscore` exits `7` (DirtyTree) with stderr naming the source repo and `spec/ideas/foo.md`. No mutations.

### AC: preflight-sibling-with-references-dirty
**Requirements:** [#req:preflight-clean-tree-siblings-with-references](#req-preflight-clean-tree-siblings-with-references)

Given `spec/ideas/foo.md` exists in source, AND a sibling repo `~/projects/specscore/specscore-cli` contains a Feature at `spec/features/cli/idea/relocate/README.md` with the markdown link `[Source Idea](https://github.com/specscore/specstudio-skills/.../foo.md)` referencing the artifact, AND that file has unstaged modifications, running `specscore idea relocate foo --to-repo=specscore` exits `7` (DirtyTree) with stderr naming the sibling repo and the affected file path. No mutations.

### AC: destination-collision
**Requirements:** [#req:destination-collision-rejected](#req-destination-collision-rejected)

Given `spec/ideas/foo.md` exists in source AND `spec/ideas/foo.md` already exists in the target repo (from a previous Idea with the same slug), running `specscore idea relocate foo --to-repo=specscore` exits `1` (Conflict). The source file is unchanged; the target file is unchanged.

### AC: in-file-rewrite-org-rename
**Requirements:** [#req:file-copy-with-rewrite](#req-file-copy-with-rewrite)

Given the source artifact contains the substring `specscore/specscore` somewhere in its body (legacy org reference), the copied artifact in the target repo has every occurrence rewritten to `specscore/specscore`. Matches the 2026-05-20 manual relocate of `artifact-frontmatter-convention` as the worked reference.

### AC: in-file-rewrite-this-repo
**Requirements:** [#req:file-copy-with-rewrite](#req-file-copy-with-rewrite)

Given the source artifact (relocating from `specstudio-skills` to `specscore`) contains a body-prose sentence `Existing artifacts in this repo are migrated by a one-shot script.`, the copied artifact has `this repo` rewritten to `specscore`. A code block inside the same artifact containing `git -C this-repo ...` is NOT modified — code blocks are left alone.

### AC: cross-repo-link-cleanup-markdown-link
**Requirements:** [#req:cross-repo-link-cleanup](#req-cross-repo-link-cleanup)

Given a sibling repo contains a markdown link `[See the Idea](../../specstudio-skills/spec/ideas/foo.md)` referencing the source artifact, after `specscore idea relocate foo --to-repo=specscore`, that link is rewritten to the full GitHub URL form `https://github.com/specscore/specscore/blob/main/spec/ideas/foo.md`. The link's display text is unchanged.

### AC: cross-repo-link-cleanup-preserves-slug-metadata
**Requirements:** [#req:cross-repo-link-cleanup](#req-cross-repo-link-cleanup)

Given a sibling repo's Feature contains `**Source Ideas:** foo` (slug-only reference, no path), after relocate, the line is unchanged — slugs are durable identifiers and the slug-only reference remains valid post-relocation.

### AC: auto-commit-three-repo-flow
**Requirements:** [#req:auto-commit-default](#req-auto-commit-default), [#req:stdout-format](#req-stdout-format)

Given the relocate touches three repos (source `specstudio-skills`, target `specscore`, sibling `specscore-cli` with a discovered link reference), after a successful run, each repo has a new commit at HEAD with the canonical subject lines (`chore(relocate): move idea foo to specscore`, `chore(relocate): receive idea foo from specstudio-skills`, `chore(relocate): update links for foo (specstudio-skills → specscore)`). The verb's stdout lists the three repos in order (source, target, alphabetical siblings) with their 7-char commit SHAs, plus the summary `relocate complete: 3 repos affected`.

### AC: no-commit-flag-stages-everywhere
**Requirements:** [#req:no-commit-flag](#req-no-commit-flag), [#req:stdout-format](#req-stdout-format)

After `specscore idea relocate foo --to-repo=specscore --no-commit`, every affected repo has all mutations applied AND staged (`git status --short` in each shows `D`/`A`/`M` lines, all with index-set status). No new commits exist anywhere. The verb's stdout lists each affected repo and its staged paths, with no `[<sha>]` bracket since no commits ran.

### AC: commit-failure-mid-flight
**Requirements:** [#req:stop-on-first-commit-failure](#req-stop-on-first-commit-failure)

Given the source repo is processed and committed successfully, but the target repo has a pre-commit hook that exits non-zero, the verb exits `10` (CommitFailed). Stderr names: source repo's commit SHA, target repo's hook failure (verbatim stderr from `git commit`), any sibling repos not yet processed (still with in-tree mutations). The stderr includes exact `git -C <source-repo> reset HEAD~1 --hard` for the source-side rollback and `git -C <target-repo> reset HEAD && git -C <target-repo> checkout -- .` for the target-side cleanup.

### AC: io-failure-rollback-pre-commit
**Requirements:** [#req:rollback-pre-commit-mutations](#req-rollback-pre-commit-mutations)

Given the target repo's `spec/ideas/` directory is write-protected (chmod 555 for the test) so file copy fails, running `specscore idea relocate foo --to-repo=specscore` exits `10` (IOFailure). The source artifact remains at its original path in `specstudio-skills` (delete-step was rolled back or never reached). The target has no partial file at `spec/ideas/foo.md`. Stderr names the I/O failure and the restore actions performed.

## Open Questions

- Should the verb invoke `specscore spec lint` (or `--fix`) automatically in each affected repo post-mutation, before committing? The manual ritual ends with a lint pass to sync indexes. Lean: skip in v1 — lint is fast and the `--no-commit` workflow naturally invites a manual lint check; revisit if dogfooding shows users routinely forget.
- Bare-text mentions of a slug in prose (not as a markdown link, not as bold-prefixed metadata) are NOT updated by the cross-repo link-cleanup pass. Should they be? Lean: no — too ambiguous (slugs can collide with common nouns or appear inside larger words; false positives would mangle prose). The verb's stdout MAY optionally suggest a workspace-wide `grep` for prose mentions as a post-step.
- Code-annotation cleanup (`specscore:` annotations in source code, bare `https://specscore.md/...` URLs in code comments) is deferred to v1.5 per the source Idea. The eventual expansion of scope (new `--include-code` flag? automatic? Idea-spec follow-on?) is deferred until the markdown-doc case is proven in real use.
- Best-effort commit-message body cross-linking: when the source commit runs first, its SHA is unknown to the target commit at the moment the source commit message is composed. The target commit can reference the source SHA in its body; the source commit cannot reference the target SHA without rewriting history. Lean: commit-message bodies are best-effort one-direction (later commits reference earlier SHAs; earlier commits don't); detail at implementation time.
- The cross-repo Source-Idea reference for this Feature is a documented workaround — the body-metadata `**Source Ideas:**` line is `—` because the local lint rule does not resolve cross-repo slugs. Tracked separately as [`spec/ideas/seeds/cross-repo-source-idea-references-should-be-supported-by-lin.md`](https://github.com/specscore/specstudio-skills/blob/main/spec/ideas/seeds/cross-repo-source-idea-references-should-be-supported-by-lin.md) in `specstudio-skills`. When that gap is closed, update this Feature's body metadata to use the structured Source Idea reference.

---
*This document follows the https://specscore.md/feature-specification*
