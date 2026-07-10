---
format: https://specscore.md/feature-specification
---
# Feature: extension-contract-repo

**Status:** Implementing

## Summary

The extension contract repo — a declared-status spec entity (an honest Sneat
name) the status-of instances query.

## Acceptance Criteria

### AC: publishes-contract

Scenario: the contract is published
Given a versioned contract
When it is released
Then consumers can pin it
