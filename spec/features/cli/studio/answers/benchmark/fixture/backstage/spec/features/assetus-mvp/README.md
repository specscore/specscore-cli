---
format: https://specscore.md/feature-specification
---
# Feature: assetus-mvp

**Status:** Approved

## Summary

assetus-mvp — a real Sneat backstage entity id mirrored into the hermetic benchmark
fixture so the status-of instances answer identically on both stores.

## Acceptance Criteria

### AC: exists

Scenario: the artifact exists
Given the spec
When it is read
Then its status is declared
