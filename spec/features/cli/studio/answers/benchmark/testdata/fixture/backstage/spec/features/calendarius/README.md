---
format: https://specscore.md/feature-specification
status: Draft
---
# Feature: calendarius

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/answers/benchmark/testdata/fixture/backstage/spec/features/calendarius?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/answers/benchmark/testdata/fixture/backstage/spec/features/calendarius?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/answers/benchmark/testdata/fixture/backstage/spec/features/calendarius?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/answers/benchmark/testdata/fixture/backstage/spec/features/calendarius?op=request-change) |
**Status:** Draft
**Source Ideas:** —

## Summary

calendarius — a real Sneat backstage entity id mirrored into the hermetic benchmark
fixture so the status-of instances answer identically on both stores.

## Contents

| Child | Description |
|---|---|
| [calendarius-rename](calendarius-rename/README.md) | TODO: Add description. |

## Acceptance Criteria

### AC: exists

Scenario: the artifact exists
Given the spec
When it is read
Then its status is declared
