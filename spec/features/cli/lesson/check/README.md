---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Lesson Check

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/check?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/check?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/check?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/lesson/check?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Summary

Make recurrence and improvement policy CI-enforceable.

## Problem

`lesson list` can reveal recurring unenforced gaps but always succeeds, forcing brittle shell plumbing in CI and making empty/malformed outcomes too easy to misread.

## Behavior

```
specscore lesson check [--not-enforced] [--min-recurred N] [--max N] [--format text|yaml|json]
```

The command uses `lesson list`'s validated filters and ordering, prints every match, and exits `1` if matches exceed `--max` (default `0`); it exits `0` otherwise. Invalid filters exit `2`; an empty result is success. It never promotes a Lesson. CI may adopt `--not-enforced --min-recurred=1` only after reviewed baseline cleanup.

## Acceptance Criteria

### AC: recurrence-gate-is-deterministic

**Given** one Recorded Lesson with one occurrence and one Enforced Lesson with one occurrence
**When** CI runs `lesson check --not-enforced --min-recurred=1`
**Then** only the Recorded Lesson renders and it exits `1`; after promotion, the same command is empty and exits `0`.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
