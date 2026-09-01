---
format: https://specscore.md/feature-specification
status: Stable
---
# Feature: debtus

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/answers/benchmark/testdata/fixture/backstage/spec/features/debtus?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/answers/benchmark/testdata/fixture/backstage/spec/features/debtus?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/answers/benchmark/testdata/fixture/backstage/spec/features/debtus?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/answers/benchmark/testdata/fixture/backstage/spec/features/debtus?op=request-change) |
**Status:** Stable
**Source Ideas:** —

## Summary

debtus — a real Sneat backstage entity id mirrored into the hermetic benchmark
fixture so the status-of instances answer identically on both stores.

## Acceptance Criteria

### AC: exists

Scenario: the artifact exists
Given the spec
When it is read
Then its status is declared
