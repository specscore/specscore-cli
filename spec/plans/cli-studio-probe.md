---
format: https://specscore.md/plan-specification
status: Approved
---
# Plan: studio probe — probes + freshness as data

**Status:** Executing
**Source Feature:** cli/studio/probe
**Date:** 2026-07-10
**Owner:** ai
**Supersedes:** —

## Summary

Implements Studio Phase 1's probe half (`cli/studio/probe`): `verified_at` freshness plumbing in the fact model and store (with a non-destructive per-kind `Merge`), the `--stale` filter and age rendering on `studio facts`, the new `studio probe` verb with its domain-liveness (`probe-domain`) and CI-state (`probe-ci`) kinds, and the self-hosting Rehearse scenarios + CI wiring. Subsystems touched: `internal/studio/{fact,store}`, a new `internal/studio/probe` package, `internal/cli/studio.go`, `spec/features/cli/studio/probe/_tests/`, `.github/workflows/go-ci.yml`.

## Approach

Data model first, then the verb, then each probe kind, then the self-hosted proof. Task 1 lands `VerifiedAt` + the store's `verified_at` column, the merge entry point, and the `facts` query surface (`--stale`, VERIFIED age column) — everything downstream writes through it. Task 2 adds the `studio probe` verb skeleton with the domain-liveness kind (https-first/http-fallback, `down` on both-schemes failure, per-kind adapter id `probe-domain`). Task 3 proves the merge semantics end-to-end through the verb (index facts preserved, re-probe refreshes `verified_at`, changed object = new observation, missing store exits 2). Task 4 adds the CI kind: targets resolved per workspace repo via `git remote get-url origin` through the exec seam (subjects re-mint `fact.RepoSlugger` slugs over the same `ResolveRepos` order the index run used — the slug-join invariant), `gh api` conclusions, visible skips. Task 5 authors the 14 executable `_tests/` scenarios and extends CI. Rehearse scenarios reach the seams via PATH shims (fake `git`/`gh` executables) and a localhost fixture HTTP server — no cross-process Go-var stubbing. Every task keeps `scripts/coverage-gate.sh` at 100% and `specscore spec lint` at 0, and commits with `Verifies:` trailers.

## Tasks

### Task 1: verified_at plumbing and the facts freshness surface

**Verifies:** cli/studio/probe#ac:stale-filter-selects-old-facts, cli/studio/probe#ac:stale-filter-malformed-duration, cli/studio/probe#ac:age-column-rendered
**Depends-On:** —
**Status:** complete

Add `VerifiedAt` to `internal/studio/fact`, the `verified_at` column to the store schema, and `store.Merge` (per-adapter-id replace; refresh-vs-new-observation stamp semantics per REQ verified-at-field). Extend `studio facts` with the `--stale <duration>` filter (malformed duration exits 2 with a helpful message) and the VERIFIED age column in human output.

### Task 2: studio probe verb and the domain-liveness kind

**Verifies:** cli/studio/probe#ac:probe-writes-verified-serves-status, cli/studio/probe#ac:declared-and-verified-coexist, cli/studio/probe#ac:network-failure-records-down
**Depends-On:** 1
**Status:** complete

New `internal/studio/probe` package + `studio probe` verb in `internal/cli/studio.go`: reads domain targets from `serves-status` declared facts in the store, checks each via https-first/http-fallback through an injectable HTTP seam, and emits `verified-behavior` facts under adapter id `probe-domain` (both-schemes failure → `down`), merged into the store without touching declared facts. Run summary (human + JSON `{kinds, facts_written, verified_refreshed, warnings}`).

### Task 3: merge semantics end-to-end and verb guards

**Verifies:** cli/studio/probe#ac:probe-preserves-index-facts, cli/studio/probe#ac:reprobe-refreshes-verified-at, cli/studio/probe#ac:changed-object-new-observation, cli/studio/probe#ac:probe-without-index-errors
**Depends-On:** 2
**Status:** complete

Prove the non-destructive merge through the verb: probing preserves all index-written facts; re-probing an unchanged fact refreshes `verified_at` while keeping `observed_at`; a changed object writes a fresh observation (both stamps new); `studio probe` without a prior `studio index` store exits 2 with the same guidance `studio facts` gives.

### Task 4: CI-state kind

**Verifies:** cli/studio/probe#ac:ci-state-fact, cli/studio/probe#ac:non-github-repo-skipped, cli/studio/probe#ac:gh-absent-skips-ci
**Depends-On:** 3
**Status:** complete

Add the `probe-ci` kind: per workspace repo, resolve the GitHub coordinate from `git remote get-url origin` (exec seam), skip non-GitHub/no-remote repos with a visible per-repo notice, query the latest default-branch run conclusion via `gh api` (absent `gh` skips the kind upfront, like rehearse's missing-binary skip), and emit `ci-status` facts subject-keyed by the store's repo slug.

### Task 5: self-hosting scenarios, CI, and docs

**Verifies:** cli/studio/probe#ac:cadences-in-help
**Depends-On:** 4
**Status:** planning

Document the per-class cadence table in `studio probe --help` and replace the 14 pending `_tests/` stubs with executable scenarios (PATH-shim `git`/`gh` fakes + a localhost fixture server for domain checks), wired into the existing Rehearse corpus CI job. Update `.gitignore`/docs as touched.

## Open Questions

None at this time.

## Autonomous Decisions

- **Five linear tasks** (alternatives: 4 fatter or 6 thinner) — keeps every task ≤4 ACs and one focused session, with the data model isolated from the verb and each probe kind landing separately.
- **Rehearse seam mechanism pinned to PATH shims + localhost fixture server** (alternative: cross-process Go-var stubs, which are impossible) — per the reviewer's advisory, recorded here so implementers don't reinterpret "stubbed seams".
- **Slug-join invariant** recorded in Approach: probe re-mints repo slugs over the same `ResolveRepos` order the index used, so `-N` collision suffixes cannot diverge.

---
*This document follows the https://specscore.md/plan-specification*
