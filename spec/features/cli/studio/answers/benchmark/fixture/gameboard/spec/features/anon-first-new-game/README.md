---
format: https://specscore.md/feature-specification
---
# Feature: anon-first-new-game

**Status:** Approved

## Summary

Anonymous-first new game creation — a spec entity with a declared status the
benchmark's status-of instances query.

## Acceptance Criteria

### AC: creates-anon

Scenario: an anonymous visitor creates a game
Given no account
When they start a game
Then a game exists
