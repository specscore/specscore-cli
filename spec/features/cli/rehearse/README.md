---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: rehearse — acceptance-evidence runner command group

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse?op=request-change) |
**Status:** Draft
**Source Ideas:** —

## Summary

Command group for the Rehearse acceptance-evidence layer (v0.3 fold-in per rehearse repo spec/ideas/rehearse-evidence-layer.md).

## Contents

| Child | Description |
|---|---|
| [run](run/README.md) | TODO: Add description. |
| [evidence](evidence/README.md) | Persist rehearse run JSON reports and ingest them at studio index time as verified-behavior facts |
| [new](new/README.md) | Scaffold a Rehearse scenario file pre-populated with Given/When/Then structure and Verifies metadata from a feature's acceptance criterion |
| [new-dry-run](new-dry-run/README.md) | Preview scaffold markdown without writing files or committing to git |
| [file-assertions](file-assertions/README.md) | Verify filesystem state (file existence, content, permissions) in scenario assertions |
| [run-filter](run-filter/README.md) | Run scenarios by acceptance criterion with --filter flag |
| [file-assertions-glob](file-assertions-glob/README.md) | Glob patterns in file assertion paths for set-based matching |
| [file-assertions-glob-recursive](file-assertions-glob-recursive/README.md) | Recursive `**` glob matching in file assertion paths via doublestar |
| [expected-fail](expected-fail/README.md) | `**Expect:** fail` scenario directive so negative scenarios run green in the corpus |

## Problem

TODO: What problem does this feature solve?

## Behavior

TODO: How does this feature work?

## Acceptance Criteria

TODO: Define acceptance criteria.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
