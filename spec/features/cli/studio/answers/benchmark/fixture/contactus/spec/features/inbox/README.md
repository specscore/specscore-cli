---
format: https://specscore.md/feature-specification
---
# Feature: inbox

**Status:** Approved

## Summary

The contactus inbox — a declared-status spec entity with a passing rehearse run,
the freshness-of / what-verifies happy path for the platform vertical.

## Acceptance Criteria

### AC: receives-message

Scenario: a message is received
Given a submitted form
When it is processed
Then it appears in the inbox
