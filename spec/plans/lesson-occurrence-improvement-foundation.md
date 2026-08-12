---
format: https://specscore.md/plan-specification
status: Executing
---
# Plan: Lesson Occurrence Improvement Foundation

**Status:** Executing
**Reconciled:** 2026-08-10
**Source Feature:** cli/lesson
**Date:** 2026-08-10
**Owner:** alex
**Supersedes:** —

## Summary

Implements the reviewed Lesson/Occurrence system without losing existing flat Lessons: directory-form parsing and generic registry support; normalized occurrence capture/query and `recur` compatibility; lossless legacy import; human-confirmed relations; tracking/evidence lint; a nonzero recurrence gate; coordination links delegated to Synchestra; and automatic events backed by a durable per-subscriber replay outbox. The upstream SpecScore meta-format gate is accepted; the remaining block is the native Synchestra adapter and its dependent whole-journey proof.

Tasks 1–9 are implemented in this candidate and remain validated, not merged.
The generic projection and external-hook boundary required by Task 10 is also
present, but the native Synchestra adapter is not implemented or proven, so
Task 10 remains Blocked and the dependent whole-journey Task 11 remains
Blocked. This checkpoint does not claim programme completion.

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
**Status:** in_progress
**Note:** Validated on PR #128; awaiting merge to satisfy Definition of Done
**Evidence:** https://github.com/specscore/specscore-cli/pull/128

Do not implement code until the upstream meta-spec review accepts directory layout, occurrence schema, mandatory metadata, tracking/evidence representation, relations, and import-manifest location. Pin tests to that schema and record the released CLI dependency; this task is a hard dependency, not paperwork.

### Task 2: Compatible directory/flat resolver and generic document registry

**Id:** layout-registry
**Verifies:** cli/lesson#ac:directory-form-remains-flat-compatible, cli/lesson#ac:group-exposes-subcommands
**Depends-On:** 1
**Status:** in_progress
**Note:** Validated on PR #128; awaiting merge to satisfy Definition of Done
**Evidence:** https://github.com/specscore/specscore-cli/pull/128

Extend `pkg/lesson` discovery/resolution and every read command to address flat and directory Lessons deterministically. Register both Lesson README layouts as status-bearing generic document targets; add collision, status-mirror, footer, and no-relocation tests.

### Task 3: Immutable occurrence parser, writer, and read commands

**Id:** occurrences
**Verifies:** cli/lesson#ac:occurrence-capture-is-lossless-and-lazy
**Depends-On:** 2
**Status:** in_progress
**Note:** Validated on PR #128; awaiting merge to satisfy Definition of Done
**Evidence:** https://github.com/specscore/specscore-cli/pull/128

Implement `pkg/lesson/occurrence` and `lesson occurrence add|list|info`: opaque JSON input validation, lazy default capture, ID/time ordering, lossless JSON output, and atomic writes. Test explicit-context paths prove no live Synchestra or ambient-session access.

### Task 4: Recur compatibility and denormalized occurrence rollup

**Id:** recur-compatibility
**Verifies:** cli/lesson#ac:recur-does-not-change-status, cli/lesson#ac:recur-against-retired-lesson-warns, cli/lesson#ac:not-enforced-and-min-recurred-compose
**Depends-On:** 3
**Status:** in_progress
**Note:** Validated on PR #128; awaiting merge to satisfy Definition of Done
**Evidence:** https://github.com/specscore/specscore-cli/pull/128

Delegate directory-form `recur` to occurrence creation while preserving command output/status semantics; retain flat-file behavior until migration. Update list/info/index rollups from occurrence data and test a mixed flat/directory project.

### Task 5: Lossless legacy importer and reviewed mapping

**Id:** legacy-import
**Verifies:** cli/lesson#ac:legacy-import-requires-reviewed-mapping
**Depends-On:** 2
**Status:** in_progress
**Note:** Validated on PR #128; awaiting merge to satisfy Definition of Done
**Evidence:** https://github.com/specscore/specscore-cli/pull/128

Implement parser fixtures from the real legacy variants, deterministic dry-run report/digest/mapping, and apply-only creation. Include destructive-safety tests: source files are byte-identical, ambiguous rows block apply, and repeated apply writes nothing new.

### Task 6: Human-confirmed relation model and supersession graph checks

**Id:** relations
**Verifies:** cli/lesson#ac:relations-are-human-confirmed
**Depends-On:** 2
**Status:** in_progress
**Note:** Validated on PR #128; awaiting merge to satisfy Definition of Done
**Evidence:** https://github.com/specscore/specscore-cli/pull/128

Add durable relation storage, preview-token confirmation, relation list output, and graph validation. Test that no fuzzy/title-match heuristic changes a Lesson and that failed confirmation/cycle validation is mutation-free.

### Task 7: Required tracking/evidence/occurrence lint family and index exactness

**Id:** lesson-lints
**Verifies:** cli/lesson#ac:process-gap-required, cli/lesson#ac:ownership-and-enforcement-evidence-are-linted
**Depends-On:** 3, 6
**Status:** in_progress
**Note:** Validated on PR #128; awaiting merge to satisfy Definition of Done
**Evidence:** https://github.com/specscore/specscore-cli/pull/128

Implement L-005–L-009 and strengthen index detection for duplicate/unknown rows. Add negative fixtures for missing metadata/tracking, invalid evidence path, flat-directory collision, invalid occurrence JSON/time, broken relation, and status-mirror registry coverage; preserve L-001 TODO-friendly behavior.

### Task 8: Nonzero recurrence policy command

**Id:** recurrence-check
**Verifies:** cli/lesson#ac:recurrence-policy-is-ci-visible
**Depends-On:** 4, 7
**Status:** in_progress
**Note:** Validated on PR #128; awaiting merge to satisfy Definition of Done
**Evidence:** https://github.com/specscore/specscore-cli/pull/128

Implement `lesson check` by reusing the list filter parser and ordered result model, with explicit `--max` exit semantics. Add command tests for error/empty/baseline behavior and a CI documentation fixture; it never changes status.

### Task 9: Durable event ledger, per-subscriber outbox, replay, and Lesson emissions

**Id:** durable-events
**Verifies:** cli/lesson#ac:lesson-events-replay-per-subscriber
**Depends-On:** 3, 4, 5, 6, 7
**Status:** in_progress
**Note:** Validated on PR #128; awaiting merge to satisfy Definition of Done
**Evidence:** https://github.com/specscore/specscore-cli/pull/128

Extend the existing event subsystem with immutable ledger, independent durable-subscriber outboxes/cursors, UUID idempotency, and `event replay`. Wire automatic events only after each successful Lesson mutation's durable commit point; test rollback emits none and one failed subscriber does not redeliver an acknowledged peer.

### Task 10: Generic external-resource coordination and native adapter

**Id:** synchestra-coordination
**Verifies:** cli/lesson#ac:coordination-delegates-live-work
**Depends-On:** 9
**Status:** blocked

Implement local projection rendering plus explicit `--refresh|--open|--message|--resume` adapter calls. SpecScore resolves the selected Lesson and sends the configured adapter only project context plus a canonical project-relative external-resource reference and revision digest. The adapter maps that opaque reference to generic Synchestra effort/run/session associations; Synchestra stores, compares, and returns the reference unchanged and does not parse or resolve Lesson slugs, statuses, occurrences, relations, or any other SpecScore Doc-Kind semantics.

The candidate implements the vendor-neutral core: it reads the adapter-owned
`agents.json` projection without network access and delegates live actions to
an explicitly configured external hook using the SpecScore lesson-agents
request schema version 2, whose `project` context and `external_resource` value
replace the former structured `lesson` object. The native adapter must use only
Synchestra's configured authoritative public interface and generic
external-resource associations; it must not require a Lesson-specific endpoint.
Contract tests must prove no direct mirror, Git, SQLite, DALgo, or inGitDB
access, no SpecScore-side message persistence/outbox, and graceful visible
mirror-lag rendering.

The task remains Blocked on implementation, not a product decision: Synchestra
does not yet expose the required generic external-resource association journey,
and the native integration does not yet prove Synchestra authorization, stable
idempotent message IDs, durable receipts/retry, auditable resume, plus the
adapter's atomic `agents.json` refresh end to end. Synchestra owns those
coordination guarantees; the adapter owns the mapping and projection
render/write, while SpecScore retains all Lesson semantics.

### Task 11: Whole journey and downstream migration verification

**Id:** journey-e2e
**Verifies:** cli/lesson#ac:occurrence-capture-is-lossless-and-lazy, cli/lesson#ac:recurrence-policy-is-ci-visible, cli/lesson#ac:lesson-events-replay-per-subscriber, cli/lesson#ac:coordination-delegates-live-work
**Depends-On:** 5, 8, 9, 10
**Status:** blocked

Build an end-to-end fixture that records a Lesson, captures an occurrence, shares durable agent context through the real native adapter and Synchestra's generic external-resource association, promotes with evidence, exercises policy, fails/replays one subscriber, and verifies both closure and human-confirmed-overlap epilogues. Dogfood the release against Backstage with a dry-run import before any apply; do not enable the recurrence CI gate until its baseline is explicitly handled. A fake hook or Lesson-aware Synchestra endpoint does not satisfy this task.

### Task 12: Finalize validated task lifecycle after merge

**Id:** post-merge-finalization
**Verifies:** cli/lesson#ac:directory-form-remains-flat-compatible, cli/lesson#ac:group-exposes-subcommands, cli/lesson#ac:occurrence-capture-is-lossless-and-lazy, cli/lesson#ac:recur-does-not-change-status, cli/lesson#ac:recur-against-retired-lesson-warns, cli/lesson#ac:not-enforced-and-min-recurred-compose, cli/lesson#ac:legacy-import-requires-reviewed-mapping, cli/lesson#ac:relations-are-human-confirmed, cli/lesson#ac:process-gap-required, cli/lesson#ac:ownership-and-enforcement-evidence-are-linted, cli/lesson#ac:recurrence-policy-is-ci-visible, cli/lesson#ac:lesson-events-replay-per-subscriber
**Depends-On:** 9
**Status:** blocked

After PR #128 is merged and exact-main CI is green, a separate implementation
agent confirms the landed ancestry, transitions Tasks 1–9 from in_progress to
complete through `specscore task change-status`, then advances this task through
in_progress to complete. This is deliberately not assigned to the mechanical
merger: merging establishes the Definition of Done, while lifecycle
finalization records that established fact without expanding merge authority.
Tasks 10–11 remain Blocked until their own acceptance criteria are delivered.

## Open Questions

None at this time. Task 10's remaining dependency is the generic Synchestra
capability and native-adapter implementation described above, not a decision
about Lesson semantics or a Lesson-specific API.

---

## Resolution

**Reconciled Draft → Implemented outside the tracked `change-status` flow** (11 task(s) marked complete to match delivered code; this did not walk the legal-transition matrix).

Founder selected reconciliation and landing on 2026-08-10 after approving the Lesson/Occurrence design; all eleven delivered tasks are reconciled from the recovery implementation rather than being represented as an unstarted Draft.

Evidence: 09ed24b, aa45ba9, e935c1f, adf7530, d0b6386

**Reconciled Implemented → Blocked outside the tracked `change-status` flow** (2 task(s) marked blocked; this did not walk the legal-transition matrix).

Tasks 10 and 11 were incorrectly swept into the prior all-complete
reconciliation. The generic lesson-agents projection and external-hook
contract are now present in this candidate, while the native Synchestra
adapter and the dependent whole journey are still absent. Their Blocked
statuses therefore remain truthful.

Evidence: 2caec14, cf8cf3c, spec/features/cli/lesson/coordination/README.md, internal/cli/lesson_agents.go

**Reconciled Blocked → Blocked outside the tracked `change-status` flow** (9 task(s) marked blocked; this did not walk the legal-transition matrix).

Tasks are validated on PR #128 but remain unmerged; Definition of Done requires main

Evidence: https://github.com/specscore/specscore-cli/pull/128, d07d39e8320c0189faa878748a64556e346787f1
*This document follows the https://specscore.md/plan-specification*
