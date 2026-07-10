---
format: https://specscore.md/feature-specification
---
# Feature: contactus-ext

**Status:** Approved

## Summary

The contactus extension library — a declared-status spec entity (an honest
Sneat name) the status-of and what-verifies instances query.

## Acceptance Criteria

### AC: exposes-contract

Scenario: the extension exposes its contract
Given the library
When it is loaded
Then its contract is available
