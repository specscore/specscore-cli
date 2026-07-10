---
format: https://specscore.md/feature-specification
---
# Feature: chess-mvp

**Status:** Stable

## Summary

A minimal chess board — the benchmark fixture's shipped-implying feature whose
latest rehearse run fails, exercising the status-drift lifecycle-vs-verification
class (Stable declared vs a `fail` has-verification-status).

## Acceptance Criteria

### AC: board-renders

Scenario: the board renders
Given a new game
When the board loads
Then eight ranks are shown
