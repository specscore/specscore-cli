---
format: https://specscore.md/plan-specification
status: Draft
---
# Plan: Lesson Occurrence Improvement Foundation

**Status:** Draft
**Source Feature:** cli/lesson
**Date:** 2026-08-10
**Owner:** alex
**Supersedes:** —

## Summary

Implements the reviewed Lesson/Occurrence system without losing existing flat Lessons: directory-form parsing and generic registry support; normalized occurrence capture/query and `recur` compatibility; lossless legacy import; human-confirmed relations; tracking/evidence lint; a nonzero recurrence gate; coordination links delegated to Synchestra; and automatic events backed by a durable per-subscriber replay outbox. This plan remains Draft until the upstream SpecScore meta-format accepts the schema; no Go task may start beforehand.

## Approach

The format gate is first because every parser, linter, writer, and migration must consume one approved schema. Layout/parser and registry work establish a compatible read substrate. Occurrences then become the canonical source for recurrence, followed by compatibility/query behavior. Import and relations require stable identity. Lints precede the CI gate so the gate does not bless structurally incomplete evidence. Event durability is built before automatic emissions; coordination consumes those durable facts but delegates every live action to Synchestra's public CLI/server contract, never its Git or SQLite backend.

### End-to-end journey

1. An agent sees a process gap and records a directory Lesson with tracking. **Observable good result:** the Lesson and its owner link exist, and all collaborating agents can read the same durable artifact.
2. Another agent observes the gap again and adds opaque JSON context. **Observable good result:** a distinct occurrence appears without changing lifecycle or overwriting the original Lesson; the other agent sees it in chronological query output.
3. The owner links the real enforcement check, promotes the Lesson, and CI evaluates recurrence policy. **Observable good result:** local evidence resolves, the gate is green only when the applicable lesson is Enforced, and subscribers receive one replayable lifecycle event each.
4. Divergent epilogue A: the gap is genuinely closed. **Observable good result:** `Enforced` carries evidence and Synchestra-linked work can be opened/resumed from the Lesson. Divergent epilogue B: it overlaps another lesson. **Observable good result:** a human-confirmed relation preserves both histories, or an explicit supersession names the successor; no automatic merge erases evidence.

## Tasks

### Task 1: Accept and bind the upstream Lesson/Occurrence meta-format

**Id:** format-gate
**Verifies:** cli/lesson#ac:directory-form-remains-flat-compatible
**Depends-On:** —
**Status:** planning

Do not implement code until the upstream meta-spec review accepts directory layout, occurrence schema, mandatory metadata, tracking/evidence representation, relations, and import-manifest location. Pin tests to that schema and record the released CLI dependency; this task is a hard dependency, not paperwork.

### Task 2: Compatible directory/flat resolver and generic document registry

**Id:** layout-registry
**Verifies:** cli/lesson#ac:directory-form-remains-flat-compatible, cli/lesson#ac:group-exposes-subcommands
**Depends-On:** 1
**Status:** planning

Extend `pkg/lesson` discovery/resolution and every read command to address flat and directory Lessons deterministically. Register both Lesson README layouts as status-bearing generic document targets; add collision, status-mirror, footer, and no-relocation tests.

### Task 3: Immutable occurrence parser, writer, and read commands

**Id:** occurrences
**Verifies:** cli/lesson#ac:occurrence-capture-is-lossless-and-lazy
**Depends-On:** 2
**Status:** planning

Implement `pkg/lesson/occurrence` and `lesson occurrence add|list|info`: opaque JSON input validation, lazy default capture, ID/time ordering, lossless JSON output, and atomic writes. Test explicit-context paths prove no live Synchestra or ambient-session access.

### Task 4: Recur compatibility and denormalized occurrence rollup

**Id:** recur-compatibility
**Verifies:** cli/lesson#ac:recur-does-not-change-status, cli/lesson#ac:recur-against-retired-lesson-warns, cli/lesson#ac:not-enforced-and-min-recurred-compose
**Depends-On:** 3
**Status:** planning

Delegate directory-form `recur` to occurrence creation while preserving command output/status semantics; retain flat-file behavior until migration. Update list/info/index rollups from occurrence data and test a mixed flat/directory project.

### Task 5: Lossless legacy importer and reviewed mapping

**Id:** legacy-import
**Verifies:** cli/lesson#ac:legacy-import-requires-reviewed-mapping
**Depends-On:** 2
**Status:** planning

Implement parser fixtures from the real legacy variants, deterministic dry-run report/digest/mapping, and apply-only creation. Include destructive-safety tests: source files are byte-identical, ambiguous rows block apply, and repeated apply writes nothing new.

### Task 6: Human-confirmed relation model and supersession graph checks

**Id:** relations
**Verifies:** cli/lesson#ac:relations-are-human-confirmed
**Depends-On:** 2
**Status:** planning

Add durable relation storage, preview-token confirmation, relation list output, and graph validation. Test that no fuzzy/title-match heuristic changes a Lesson and that failed confirmation/cycle validation is mutation-free.

### Task 7: Required tracking/evidence/occurrence lint family and index exactness

**Id:** lesson-lints
**Verifies:** cli/lesson#ac:process-gap-required, cli/lesson#ac:ownership-and-enforcement-evidence-are-linted
**Depends-On:** 3, 6
**Status:** planning

Implement L-005–L-009 and strengthen index detection for duplicate/unknown rows. Add negative fixtures for missing metadata/tracking, invalid evidence path, flat-directory collision, invalid occurrence JSON/time, broken relation, and status-mirror registry coverage; preserve L-001 TODO-friendly behavior.

### Task 8: Nonzero recurrence policy command

**Id:** recurrence-check
**Verifies:** cli/lesson#ac:recurrence-policy-is-ci-visible
**Depends-On:** 4, 7
**Status:** planning

Implement `lesson check` by reusing the list filter parser and ordered result model, with explicit `--max` exit semantics. Add command tests for error/empty/baseline behavior and a CI documentation fixture; it never changes status.

### Task 9: Durable event ledger, per-subscriber outbox, replay, and Lesson emissions

**Id:** durable-events
**Verifies:** cli/lesson#ac:lesson-events-replay-per-subscriber
**Depends-On:** 3, 4, 5, 6, 7
**Status:** planning

Extend the existing event subsystem with immutable ledger, independent durable-subscriber outboxes/cursors, UUID idempotency, and `event replay`. Wire automatic events only after each successful Lesson mutation's durable commit point; test rollback emits none and one failed subscriber does not redeliver an acknowledged peer.

### Task 10: Synchestra coordination projection and adapter boundary

**Id:** synchestra-coordination
**Verifies:** cli/lesson#ac:coordination-delegates-live-work
**Depends-On:** 9
**Status:** planning

Implement local projection rendering plus explicit `--refresh|--open|--message|--resume` adapter calls. Integrate only through Synchestra's configured authoritative public CLI/server endpoint/outbox; contract tests must prove no direct mirror, Git, SQLite, DALgo, or inGitDB access, no message persistence/outbox, and graceful visible mirror-lag rendering.

### Task 11: Whole journey and downstream migration verification

**Id:** journey-e2e
**Verifies:** cli/lesson#ac:occurrence-capture-is-lossless-and-lazy, cli/lesson#ac:recurrence-policy-is-ci-visible, cli/lesson#ac:lesson-events-replay-per-subscriber, cli/lesson#ac:coordination-delegates-live-work
**Depends-On:** 5, 8, 9, 10
**Status:** planning

Build an end-to-end fixture that records a Lesson, captures an occurrence, shares durable agent context, promotes with evidence, exercises policy, fails/replays one subscriber, and verifies both closure and human-confirmed-overlap epilogues. Dogfood the release against Backstage with a dry-run import before any apply; do not enable the recurrence CI gate until its baseline is explicitly handled.

## Open Questions

- The plan is intentionally blocked on upstream meta-format acceptance. The implementation owner must not substitute a local schema merely to begin Task 2.

---
*This document follows the https://specscore.md/plan-specification*
