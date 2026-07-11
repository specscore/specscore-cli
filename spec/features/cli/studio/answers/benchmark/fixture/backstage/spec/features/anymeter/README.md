---
format: https://specscore.md/feature-specification
status: Draft
---
# Feature: anymeter

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/answers/benchmark/fixture/backstage/spec/features/anymeter?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/answers/benchmark/fixture/backstage/spec/features/anymeter?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/answers/benchmark/fixture/backstage/spec/features/anymeter?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/answers/benchmark/fixture/backstage/spec/features/anymeter?op=request-change) |
**Status:** Draft
**Source Ideas:** —

## Summary

anymeter — a real Sneat backstage entity id mirrored into the hermetic benchmark
fixture so the status-of instances answer identically on both stores.

## Contents

| Child | Description |
|---|---|
| [standalone-product](standalone-product/README.md) | TODO: Add description. |

## Acceptance Criteria

### AC: exists

Scenario: the artifact exists
Given the spec
When it is read
Then its status is declared
