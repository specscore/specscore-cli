# Feature: studio index — multi-repository workspace indexing

## Summary

`specscore studio index` scans a workspace defined by `studio.yaml` and builds
a local index of all repositories, their manifests, and detected languages.
This feature covers the core indexing flow for multi-repository workspaces.

## Acceptance Criteria

### AC: index-two-repos

Scenario: index two repos
Given a studio.yaml with two repos
When I run specscore studio index
Then both repos are listed in the index
