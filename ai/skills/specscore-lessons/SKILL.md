---
name: specscore-lessons
description: Create, migrate, query, relate, and enforce canonical SpecScore Lessons and safe append-only occurrences; use when an agent records a process gap, deduplicates a legacy lessons log, migrates flat Lessons, fixes Open Questions headings, or recovers durable SpecScore event delivery.
---

# SpecScore Lessons

Keep the current rule compact at `spec/lessons/<slug>/README.md`. Keep each manifestation in a new `occurrences/<uuid-v4>.json`. Never commit original/user/agent prompts, raw logs or diffs, credentials, personal contact data, or private/absolute paths.

## Create and record

<!-- capability: specscore.lesson.new -->

Configure a non-empty `lessons.classifications` vocabulary before creating a Lesson. Treat failed preflight as a zero-write result. The bounded write set is the requested directory, its empty occurrence store, declared index rows, and the durable prepared event/outbox.

```sh
specscore lesson new verify-before-merge --title "Verify before merge" --owner codex
```

<!-- capability: specscore.lesson.occurrence -->

Append one bounded fact. Context accepts only generic repository, git, worktree, and execution objects. Use strict JSON; repository-relative forward-slash path hints/evidence or `redacted`; UTC `Z`; unique redaction reasons. Prefer omission/null over guessed context.

```sh
specscore lesson occurrence add verify-before-merge --summary "The merge gate was skipped." --context-json '{"repository":"github.com/org/repo","execution":{"kind":"interactive","id":"run-42"},"worktree":{"path_hint":"redacted","id":"wt-42"}}' --evidence-kind path --evidence-ref spec/tests/merge-gate.md
specscore lesson occurrence list verify-before-merge --format json
specscore lesson occurrence info verify-before-merge 123e4567-e89b-42d3-a456-426614174000 --format json
specscore lesson recur verify-before-merge --note "The bounded gap manifested again."
specscore lesson check --not-enforced --min-recurred 1 --format json
```

`recur` appends a canonical child occurrence without editing the README. It only edits the old recurrence prose while a flat Lesson remains in compatibility mode. `info`, `list`, and `check` reject invalid children rather than hiding them.

## Durable agent context

<!-- capability: specscore.lesson.agents -->

Read agent context offline from an adapter-produced `agents.json` beside a canonical Lesson. SpecScore never creates or rewrites that projection: the repository's native orchestration plugin decides whether it is versioned or local. Live actions are opt-in and use only the neutral executable named by `SPECSCORE_LESSON_AGENTS_HOOK`; the hook receives a versioned JSON request on stdin and owns authentication, messages, retries, and all backend access.

```sh
specscore lesson agents verify-before-merge --format json
SPECSCORE_LESSON_AGENTS_HOOK=synchestra-lesson-agents specscore lesson agents verify-before-merge --refresh
SPECSCORE_LESSON_AGENTS_HOOK=synchestra-lesson-agents specscore lesson agents verify-before-merge --message codex-1 --text "Please review the occurrence."
```

## Import and migration

<!-- capability: specscore.lesson.import-legacy -->

Inventory an authoritative monolith without writing. Review every heading and inline recurrence-marker byte range/hash, duplicate-ID collision, and unmatched candidate; compare the ordered entry-projection hash so parser drift cannot hide behind unchanged counts. Explicitly choose `new`, `occurrence`, or `manual`; resolve every `manual` row before apply. For each `new` row, provide `slug`, `status: Recorded`, one or more configured `classifications`, and concise safe `lesson` plus `process_gap` text. Treat the source status as provenance: the importer reports its conservative Recorded decision and never fabricates Enforced evidence. `new` creates both the compact Lesson and exactly one provider occurrence. Keep one aggregate marker as one ambiguity-tagged evidence record—never infer its incident count. Apply only committed immutable source identity and never copy raw legacy prose into committed artifacts.

```sh
specscore lesson import-legacy --source LESSONS-LEARNED.md --dry-run --format json
specscore lesson import-legacy --source LESSONS-LEARNED.md --apply --mapping reviewed-mapping.json --format json
```

<!-- capability: specscore.lesson.migrate-flat -->

Migrate each structured compatibility file explicitly. Commit the exact source first. Choose classifications from repository configuration. Keep the transaction no-clobber and resumable across canonical files, the exact index row, and its deterministic prepared event. Record the source file as a provider occurrence, create one child per structured recurrence bullet, refuse count mismatch, and remove only the verified old path. For an Enforced source, provide reviewed `--control`, `--verification`, and `--evidence`; provenance alone never certifies enforcement.

```sh
specscore lesson migrate-flat verify-before-merge --classification process --format json
specscore lesson migrate-flat enforced-boundary --classification validation --control "Run the durable boundary check." --verification "go test ./pkg/event -run TestBoundary" --evidence "pkg/event/outbox_test.go" --format json
```

## Relations

<!-- capability: specscore.lesson.relation -->

Never infer semantic duplication from title similarity. Preview the relation, inspect both histories, then copy its exact token. For `duplicates`, the retained duplicate becomes `Superseded` and points `Duplicate Of` plus `Superseded By` to the unchanged canonical Lesson. The command refuses Enforced duplicates, cycles, conflicting targets, malformed state, and overwrites.

```sh
specscore lesson relation add duplicate-rule canonical-rule --type duplicates
specscore lesson relation add duplicate-rule canonical-rule --type duplicates --confirm TOKEN_FROM_PREVIEW
specscore lesson relation list duplicate-rule --format json
```

## Explicit repository migration

<!-- capability: specscore.spec.lint.open-questions-fix -->

Only the explicit fixer rewrites the obsolete heading. Scaffold/create commands must leave unrelated files byte-identical.

```sh
specscore spec lint --fix
```

Writers and guidance emit `Open Questions`; compatibility parsing may still diagnose `Outstanding Questions` before this explicit migration.

## Durable event delivery

<!-- capability: specscore.event.outbox -->

`event emit` durably records the complete nonempty subscriber set before delivery; an explicitly empty configured set opts out without creating a recipientless ledger. Each sink has an independent acknowledgement; one success never masks another failure. Delivery is at-least-once, so subscriber handlers must deduplicate `event.uuid`. Replay one sink, and explicitly reconcile an interrupted prepared event only after inspecting the preview evidence.

```sh
specscore event emit --name lesson.observed --actor-kind skill --actor-id skill:specscore-lessons --artifact-type feature --artifact-id lesson --artifact-path spec/features/lesson/README.md --payload-json '{}'
specscore event replay --subscriber audit-sink --limit 100
specscore event reconcile 123e4567-e89b-42d3-a456-426614174000 --decision commit
specscore event reconcile 123e4567-e89b-42d3-a456-426614174000 --decision commit --confirm TOKEN_FROM_PREVIEW
```
