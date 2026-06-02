# Idea: lint --fix reports modified files

**Status:** Implementing
**Date:** 2026-06-02
**Owner:** alexander.trakhimenok
**Promotes To:** cli/spec/lint
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we ensure no file modified by `specscore spec lint --fix` is ever silently dropped from a commit, by having the CLI report exactly which files it changed so every consumer — human, CI, or agent — can stage them deliberately?

## Context

Triggered by a real failure: in a prior session an agent ran `specscore spec lint --fix`, which repaired one line (an index/footer sync caused by the agent's own edits), then omitted that line from its commit — reasoning it was "unrelated." It was related, but the agent had no way to know what `--fix` had touched.

Ground truth in this repo confirms the gap: `lint.Lint(opts)` (`pkg/lint/lint.go`) returns `[]Violation` — the violations *remaining after* the fix, never the set of files changed. The fix pass is `fix(specRoot string) error` per checker (`adherence_footer.go`, `index_entries.go`, `feature_index.go`, and ~14 write sites); each writes in place and returns only an error. The command layer (`internal/cli/spec.go`) prints only remaining violations, so a clean `--fix` produces empty stdout and exit 0. An agent's only recourse today is a `git status`/`git diff` heuristic — exactly the heuristic that failed.

## Recommended Direction

Make the CLI authoritative about what `--fix` changed, so no consumer has to guess.

**Capture changes where they happen, cheaply.** Inside `Lint()` when `Fix` is set, snapshot a content hash of each spec-tree file before the fix pass and diff after. The delta is the set of modified relative paths. This needs **zero per-checker churn** — the ~14 existing write sites and the `fix(specRoot) error` interface stay untouched — and it cannot miss a fixer that forgets to self-report.

**Surface it on the right streams.** Real errors → stderr + nonzero exit (already the case). Lint violations stay on **stdout** (preserve the existing contract; `lint` and `lint --fix` mean the same thing on stdout). In `--format json|yaml`, emit a single envelope on stdout — `{ "fixed": [...], "violations": [...] }` — never split structured data across streams. In default text mode, print a human "Fixed N file(s): …" summary to **stderr** as a run diagnostic. The report is **default-on**: silence was the original bug.

This serves *any* consumer generically — a human reads the stderr summary, CI parses the JSON envelope, and an agent reads `.fixed[]` to stage exactly what changed. The dependent specstudio skill-hardening (separate repo) is the first consumer but is out of scope here.

## Alternatives Considered

- **Agent-side `git status`/`git diff` heuristic only (no CLI change).** Cheapest, but it is the exact heuristic that already failed: it cannot distinguish lint's edits from the agent's own concurrent edits, and it pushes the burden onto every consumer. Rejected — the gap belongs at the source.
- **CLI performs `git add`/commit on the files it fixed.** Closes the loop in one step, but it makes the CLI know about the git index, breaks manual stage/commit/push workflows, and couples a spec linter to version control. Rejected on separation-of-concerns and the stage-only contract.
- **Per-checker self-reporting (`fix` returns `[]string` of touched files).** Explicit, but changes the `fix(specRoot) error` interface across ~14 call sites and silently under-reports whenever a checker forgets to append. Rejected in favor of the content-hash snapshot, which is checker-agnostic.
- **Changed files on stdout, violations on stderr.** Tempting for `lint --fix | git add`, but it overloads stdout's meaning based on whether `--fix` is set and splits two data outputs across streams. Rejected; the JSON envelope serves the pipe case without breaking the contract.

## MVP Scope

A two-to-three-day change to `pkg/lint` plus its CLI command:

1. In `Lint()` with `Fix:true`, content-hash the spec tree before/after the fix pass; return the modified relative paths (e.g. a `Result{ Violations, Fixed }` or second return value).
2. Surface them: a `fixed: []` field in `--format json|yaml` (single stdout envelope) and a "Fixed N file(s): …" stderr summary in text mode. Default-on.
3. Update the in-repo `Lint()` callers (`feature.go`, `idea.go`, `decision.go`) and the interface docs per the docs-track-interface-changes policy.

Done when an agent or script can read the exact set of files `--fix` changed from one CLI invocation, without inspecting git. Skill adoption and any `--format paths` affordance are explicitly out of MVP.

## Not Doing (and Why)

- Hunk-level attribution — separating lint's own edits from the caller's edits inside a shared file is out of scope; report at file granularity only
- CLI-side git add / commit / push — the CLI never touches the git index; staging stays the caller's job, preserving manual stage/commit/push workflows
- The specstudio skill-hardening change — it consumes this contract but lives in the specstudio plugin repo as a dependent Idea
- An opt-in flag to enable the report — silence was the original bug, so the report is default-on
- Splitting the structured fixed/violations result across stdout and stderr — one JSON/YAML envelope on stdout; never make a consumer merge two streams

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | Every fixer writes only inside the spec root, so a spec-tree content-hash snapshot captures all changes | Audit the ~14 `WriteFile`/`os.Create` sites in `pkg/lint`; confirm none write outside `SpecRoot` |
| Must-be-true | Changing `Lint()`'s return shape is acceptable for its consumers | Grep callers (`feature.go`, `idea.go`, `decision.go`, tests); all are in-repo, so the breaking change is contained |
| Should-be-true | A stderr text summary plus a `fixed` JSON/YAML field satisfies humans, CI, and agents without a new flag | Dry-run the three consumer shapes against the proposed output; confirm none need stdout path-listing |
| Might-be-true | A terse `--format paths` (one path per line on stdout) will be wanted for shell pipelines | Defer; revisit only if a real `lint --fix | git add` pipeline use case surfaces |


## SpecScore Integration

- **New Features this would create:** "lint --fix reports modified files" (CLI `pkg/lint` + `spec lint` command).
- **Existing Features affected:** the `spec lint` command and its `--format json|yaml` output contract (additive `fixed` field; tracked under the docs-track-interface-changes policy).
- **Dependencies:** the dependent consumer Idea **`specstudio-skills:autostage-lint-fix-modified-files`** consumes this contract (its skills stage exactly the paths this report names). It lives in the specstudio-skills repo; this repo only ships the CLI side. Recorded in prose because the linter does not support cross-repo `**Related Ideas:**` targets.

## Open Questions

- Should `Lint()` return a new `Result{ Violations, Fixed }` struct, or keep `[]Violation` and add a second return value / sibling `LintWithResult`? Affects the public `pkg/lint` Go API surface.
- Is a terse `--format paths` (one fixed path per line on stdout) worth adding for `lint --fix | git add`-style pipelines, or does the JSON `.fixed[]` field suffice?
- The dependent specstudio skill Idea is out of this repo — capture it via sidekick/relocate, or leave it to be authored directly in the specstudio repo?

---
*This document follows the https://specscore.md/idea-specification*
