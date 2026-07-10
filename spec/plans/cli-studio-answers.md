---
format: https://specscore.md/plan-specification
status: Approved
---
# Plan: studio answers — contradictions, resolve, ask + benchmark

**Status:** Executing
**Source Feature:** cli/studio/answers
**Date:** 2026-07-10
**Owner:** ai
**Supersedes:** —

## Summary

Implements Studio Phase 1's answers half (`cli/studio/answers`): the `studio contradictions` verb with its two evidence-class-partitioned detectors (status-vs-behavior drift in both branches; same-predicate declared disagreement) writing `contradicts` facts back through `store.Merge`, the workspace ignore-list suppression, `studio resolve <name>` alias resolution, the deterministic 13-template `studio ask` router with citations, and the committed 50-question benchmark (41 answerable / 9 expected-unanswerable) with its runner — the Phase-1 exit gate. Subsystems touched: a new `internal/studio/answers` (or sibling) package family, `internal/cli/studio.go`, `spec/features/cli/studio/answers/{benchmark,_tests}/`, `.github/workflows/go-ci.yml`.

## Approach

Detectors first (pure functions over store reads — the core value), then their negatives/plumbing, then the two query verbs that share the resolve machinery, then the benchmark that exercises everything. Task 1 lands the detector engine and the `contradictions` verb's happy paths. Task 2 hardens it: the never-flag negatives (agreement, behavioral supersession), the `contradicts` fact write-back (canonicalised pointer-free refs, merge-idempotent), the ignore list, and the missing-store guard. Task 3 lands `resolve` (aliased-as + entity-id lookup, case-insensitive, 3/5 exit codes per house vocabulary). Task 4 lands `ask`: template registry, router, store queries per the Feature's template table (including the two-step `is-it-live` fronts→serves-status hop), citations on every answer. Task 5 commits the benchmark file (exactly 50 instances mapped to the composition table), the runner (which runs `studio contradictions` before contradictions-for instances), the CI fixture floor, the 16 executable `_tests/` scenarios (same PATH-shim/fixture-store strategy as the probe corpus), and wires CI. Every task keeps `scripts/coverage-gate.sh` at 100% and `specscore spec lint` at 0, and commits with `Verifies:` trailers. The reviewer's noise watch-item (Approved-status features flagging as drift) is checked during the orchestrator's Sneat dogfood run at the exit gate.

## Tasks

### Task 1: contradiction detectors and the contradictions verb

**Verifies:** cli/studio/answers#ac:status-drift-verified-fail, cli/studio/answers#ac:status-drift-dead-domain, cli/studio/answers#ac:naming-conflict-declared-disagreement
**Depends-On:** —
**Status:** complete

The detector engine as pure functions over store facts: status-drift branch (a) shipped-implying `has-status` joined by subject prefix to failing `has-verification-status`, branch (b) same-subject+predicate declared-vs-verified-behavior disagreement; naming-conflict (declared×declared, different objects and pointers). The `studio contradictions` verb renders items with BOTH evidence sets, human + `--format json`.

### Task 2: detector negatives, contradicts write-back, suppression, guards

**Verifies:** cli/studio/answers#ac:agreement-not-flagged, cli/studio/answers#ac:behavioral-supersession-not-flagged, cli/studio/answers#ac:contradicts-fact-written, cli/studio/answers#ac:contradictions-ignore-suppresses, cli/studio/answers#ac:contradictions-without-index-errors
**Depends-On:** 1
**Status:** complete

Prove the never-flag rules (cross-class agreement; verified×verified supersession), write each item as a `contradicts` fact (canonicalised smaller-ref-first subject/object, class derived, pointer = detector id, adapter `contradictions`) via `store.Merge` so re-runs are idempotent, honor the workspace ignore-list file (`--show-ignored`), and reuse the exit-2 missing-store guidance.

### Task 3: resolve verb

**Verifies:** cli/studio/answers#ac:resolve-alias-to-canonical, cli/studio/answers#ac:resolve-ambiguous-lists-candidates, cli/studio/answers#ac:resolve-unknown-guides
**Depends-On:** 2
**Status:** in_progress

`studio resolve <name>`: case-insensitive lookup over `aliased-as` facts and entity ids themselves; unique hit prints the canonical id (exit 0), multiple candidates listed (exit 5, AmbiguousSlug), unknown prints guidance (exit 3, NotFound). The resolution helper is shared machinery for Task 4's router.

### Task 4: ask verb and the 13-template router

**Verifies:** cli/studio/answers#ac:ask-routes-with-citations, cli/studio/answers#ac:ask-unroutable-lists-templates, cli/studio/answers#ac:ask-routed-but-empty
**Depends-On:** 3
**Status:** planning

`studio ask "<question>"`: deterministic trigger-pattern router over the Feature's 13 templates, each a parameterized store query (the `is-it-live` template does the fronts→serves-status two-step hop), answers always citing fact ids/pointers; unroutable exits 1 listing the templates; routed-but-empty exits 3 with no citation-free prose.

### Task 5: benchmark, runner, self-hosting scenarios, and CI

**Verifies:** cli/studio/answers#ac:benchmark-file-has-50, cli/studio/answers#ac:benchmark-runner-scores-fixture
**Depends-On:** 4
**Status:** planning

Commit `benchmark/questions.jsonl` (exactly 50 instances matching the composition table) and the runner script (runs `studio contradictions` into the target store first, then scores answered-with-citations; composition check enforces the table), the hermetic CI fixture floor, replace the 16 pending `_tests/` stubs with executable scenarios (probe-corpus strategy: fixture stores, PATH shims where needed), and extend the Rehearse corpus CI job. Update feature Status → Implementing.

## Open Questions

None at this time.

## Autonomous Decisions

- **Five linear tasks** mirroring the probe plan's proven shape — detectors isolated from their negatives so Task 1 stays one focused session; resolve before ask because the router consumes the resolution helper.
- **Exit-gate Sneat run stays with the orchestrator** (not a task): the ≥40/50 benchmark + ≤20% contradiction-noise spot-check over `~/projects/sneat-co/*` is the human-runnable gate the Feature documents; Task 5's fixture floor is the CI-enforceable half.
- **Scenario seam strategy inherited from the probe corpus** (PATH shims + fixture stores) — recorded so implementers don't reinvent it.

---
*This document follows the https://specscore.md/plan-specification*
