---
captured_by: user
status: queued
---
# specscore needs a plan change-status verb (Draft/Under Review/Approved + execution statuses) so plan skills stop hand-editing plan status

## Problem

`specscore plan` exposes only `info`/`list`/`new` — there is no `plan change-status` verb. So the `specstudio:plan` skill's required status transitions (Draft -> Under Review on reviewer dispatch; Under Review -> Approved on user approval) have no CLI path and are done by hand-editing the plan body `**Status:**` line, the same anti-pattern the seed/idea change-status work is eliminating.

## Direction

Add `specscore plan change-status <slug> --to=<status>` inheriting lifecycle-transitions, with the plan status enum (the plan-status-lifecycle Idea: draft/in_review/approved + execution executing/blocked/implemented/failed + disposition withdrawn/superseded). Until it lands, plan skills hand-edit plan status as the only option.

## Provenance

Surfaced 2026-06-17 while planning cli/sidekick/change-status: the plan skill instructed a Draft->Under Review transition with no CLI verb to perform it.
