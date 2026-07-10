---
format: https://specscore.md/feature-specification
---
# Feature: sports

**Status:** Implementing

## Summary

Sports boards — a spec entity carrying a declared status and a passing rehearse
run (the what-verifies happy path).

## Acceptance Criteria

### AC: scores-track

Scenario: scores track
Given a live match
When a point is scored
Then the score updates
