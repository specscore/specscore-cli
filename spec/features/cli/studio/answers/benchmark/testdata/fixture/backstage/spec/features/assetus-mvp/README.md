---
format: https://specscore.md/feature-specification
status: Approved
---
# Feature: assetus-mvp

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/answers/benchmark/testdata/fixture/backstage/spec/features/assetus-mvp?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/answers/benchmark/testdata/fixture/backstage/spec/features/assetus-mvp?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/answers/benchmark/testdata/fixture/backstage/spec/features/assetus-mvp?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/answers/benchmark/testdata/fixture/backstage/spec/features/assetus-mvp?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Summary

assetus-mvp — a real Sneat backstage entity id mirrored into the hermetic benchmark
fixture so the status-of instances answer identically on both stores.

## Acceptance Criteria

### AC: exists

Scenario: the artifact exists
Given the spec
When it is read
Then its status is declared
