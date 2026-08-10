---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Lesson Relations

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/relations?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/relations?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/relations?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/relations?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Summary

Require human-confirmed lesson relationships and duplicate disposition.

## Problem

Two lessons can overlap, but text similarity is neither reliable nor authority to retire somebody else's process record. A reviewed duplicate disposition must preserve the retained Lesson's history while naming one unchanged canonical Lesson, and malformed or conflicting relation state must block publication.

## Behavior

```
specscore lesson relation add <from> --type supersedes|related|duplicates <to> --confirm <token>
specscore lesson relation list <lesson> [--format text|yaml|json]
```

Relations are durable artifact facts. `related` is symmetric; `supersedes` and `duplicates` are directed and name distinct existing Lessons. A command previews the exact change and requires its confirmation token; `--yes` is not accepted. A confirmed `duplicates` relation treats `<from>` as the retained duplicate: it becomes `Superseded`, and its `Duplicate Of` and `Superseded By` fields both name the unchanged canonical `<to>` Lesson. The canonical Lesson's inverse visibility is derived by scanning retained Lessons, so the command does not rewrite it. An Enforced retained duplicate, a cycle, a conflicting existing target, malformed state, or an overwrite race MUST fail without mutation. Lint verifies identity, duplicate disposition, acyclic supersession, and no conflicting edge; it never attempts semantic duplicate detection.

## Acceptance Criteria

### AC: duplicate-is-human-confirmed-and-history-preserving

**Given** two Lessons with similar titles
**When** a caller tries to add `duplicates` without the displayed confirmation token
**Then** it exits `2` and neither changes; after explicit confirmation the retained `<from>` Lesson is `Superseded` with both pointers naming the unchanged canonical `<to>` Lesson, and listing either endpoint exposes the relation.

### AC: malformed-or-conflicting-state-is-write-free

**Given** an Enforced retained Lesson, malformed relation data, or a relation field already naming a different target
**When** a caller confirms a duplicate relation
**Then** validation exits nonzero and every Lesson and relation artifact remains byte-identical.

### AC: supersession-cycle-is-refused

**Given** `a supersedes b`
**When** a caller confirms `b supersedes a`
**Then** validation exits nonzero naming the cycle and writes no relation.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
