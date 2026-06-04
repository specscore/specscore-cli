# Plan: Plan List (CLI)

**Status:** Completed
**Source Feature:** cli/plan/list
**Date:** 2026-06-04
**Owner:** alexander.trakhimenok
**Supersedes:** —

## Summary

Implementation plan for the `cli/plan/list` Feature — `specscore plan list`, with its text default, `--status` filter, and structured `--fields` output. Builds on the `plan` command group and plan-level parsing delivered by the `cli-plan` plan.

## Approach

Two tasks, linear. Task 1 delivers the default text listing and the `--status` filter; Task 2 adds the structured `--fields` output path. Both depend on the `plan` group and the plan-level `**Status:**` field from the `cli-plan` plan (Task 1 there). All five `list` ACs are covered; none deferred.

## Tasks

### Task 1: Text default and `--status` filter

**Verifies:** cli/plan/list#ac:default-listing-pipeable, cli/plan/list#ac:status-filter-selects, cli/plan/list#ac:empty-match-exits-zero
**Status:** done

Implement `plan list` default output: one plan slug per line, alphabetically sorted, no headers or trailing blank, exit `0` even when the result is empty. Add `--status <value>` as a case-insensitive exact match against each plan's parsed `**Status:**`, including only matching plans and still exiting `0` on an empty match.

### Task 2: Structured `--fields` output

**Verifies:** cli/plan/list#ac:fields-returns-yaml, cli/plan/list#ac:unknown-field-exits-2
**Status:** done

Add `--fields` support: a comma-separated selector over the recognized set (`status`, `source-feature`, `mode`, `date`, `owner`) producing a YAML (default) or JSON list of `{slug, …fields}` entries. `--format text` alongside `--fields` auto-upgrades to YAML rather than erroring; an unrecognized field name exits `2` naming the offending field.

## Open Questions

- Should `plan list` gain a `--source-feature <id>` filter, or is `--fields source-feature` plus `grep` adequate for now?

---
*This document follows the https://specscore.md/plan-specification*
