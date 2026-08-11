---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Lesson Occurrences

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/occurrence?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/occurrence?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/occurrence?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/occurrence?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Summary

Capture and query normalized occurrences of a process gap.

## Problem

The mutable `**Recurred:**` counter and prose recurrence list lose the actor, source, evidence, and context needed to decide whether a process gap really recurred. Concurrent agents also rewrite the same Lesson file. An occurrence is an immutable observation attached to one Lesson, not a lifecycle promotion or Issue.

## Behavior

### Directory form and compatibility

New Lessons are `spec/lessons/<slug>/README.md`; normalized occurrences live under `spec/lessons/<slug>/occurrences/`. Legacy flat `spec/lessons/<slug>.md` remains valid, readable, linted, and addressable. No read, status transition, or `recur` call relocates it; only an explicit layout migration does. Both layouts with the same slug are invalid.

### Commands and context

```
specscore lesson occurrence add <lesson> [--summary <text>] [--context-json <object> | --context-file <path> | --context-stdin] [--capture-context]
specscore lesson occurrence list <lesson> [--format text|yaml|json]
specscore lesson occurrence info <lesson> <occurrence-id> [--format text|yaml|json]
```

`add` persists a lowercase hyphenated UUID v4, RFC-3339 UTC time, optional summary, and opaque JSON-object context. The occurrence schema has no top-level `actor` field; an optional safe execution identifier may appear inside `context.execution`. Explicit JSON inputs are mutually exclusive; malformed/non-object JSON exits `2` before a write. If none is supplied, `--capture-context` defaults on and *lazily* collects only safe local facts (project root and git revision/branch if available). It MUST NOT touch Synchestra, browser state, credentials, remote APIs, or git when explicit context is present. `--capture-context=false` writes `{}`. `list` is chronological and `info` is read-only; structured output preserves unknown JSON keys.

After exclusive child publication, the command upserts the one derived index row and durably fences the child, index, and their parent directories before committing its prepared event. Any post-publication index, read-back, or fence failure retains the immutable child, current index state, and prepared event for explicit reconciliation. It MUST NOT unlink the child as compensation because a path is not an ownership token and may have been replaced by a concurrent writer.

### `recur` compatibility

For directory Lessons, `lesson recur <slug> --note` delegates to `occurrence add`, maps `--note` to summary, updates the denormalized count, emits the historical success line, and never changes status. For flat files it retains the existing rewrite until explicit migration.

## Acceptance Criteria

### AC: occurrence-journey-preserves-context

**Given** a directory Lesson with no occurrences
**When** an agent adds `--context-json '{"run":"42","files":["x.go"]}'`, lists it, and inspects the returned ID
**Then** the record exists, its context is preserved, list orders it chronologically, and the Lesson lifecycle is unchanged.

### AC: lazy-default-capture-does-not-touch-live-state

**Given** explicit `--context-json` and a Synchestra adapter that would fail if called
**When** the caller adds an occurrence
**Then** no adapter, credential, browser, or ambient-session read occurs; only supplied JSON is persisted.

### AC: recur-remains-compatible

**Given** a directory Lesson in `Stated`
**When** an existing script runs `lesson recur <slug> --note "seen again"`
**Then** it exits `0` with historical output, records exactly one occurrence with that summary, increments the rollup, and leaves `Stated` unchanged.

### AC: post-publication-failure-retains-foreign-state

**Given** an occurrence child is visible and a concurrent writer changes the index or replaces the child path
**When** index reconciliation, read-back, or a durability fence fails
**Then** the command exits `10`, retains the child path and every concurrent index row byte-for-byte, leaves the prepared event inspectable, and performs no compensating unlink or whole-index restore.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
