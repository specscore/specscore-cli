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

Two lessons can overlap, but text similarity is neither reliable nor authority to retire somebody else's process record. The existing supersession field is one-directional and only checked while a command runs.

## Behavior

```
specscore lesson relation add <from> --type supersedes|related|duplicates <to> --confirm <token>
specscore lesson relation list <lesson> [--format text|yaml|json]
```

Relations are durable artifact facts. `related` is symmetric; `supersedes` and `duplicates` are directed and name distinct existing Lessons. A command previews the exact change and requires its confirmation token; `--yes` is not accepted. `duplicates` never auto-merges or retires either side. A human may then use the existing Superseded transition with a named successor/reason. Lint verifies identity, inverse symmetry, acyclic supersession, and no duplicate edge; it never attempts semantic duplicate detection.

## Acceptance Criteria

### AC: duplicate-is-human-confirmed-not-auto-retired

**Given** two Lessons with similar titles
**When** a caller tries to add `duplicates` without the displayed confirmation token
**Then** it exits `2` and neither changes; after explicit confirmation both expose the relation and retain status.

### AC: supersession-cycle-is-refused

**Given** `a supersedes b`
**When** a caller confirms `b supersedes a`
**Then** validation exits nonzero naming the cycle and writes no relation.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
