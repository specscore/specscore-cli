---
type: sidekick-seed
slug: idea-sync-autofix-downgrades-implemented-idea-status
captured_at: 2026-06-03T13:00:00Z
captured_by: specstudio:implement
captured_during: null
trigger: heuristic
status: queued
synchestra_task: null
---
# specscore spec lint --fix (idea-sync-lint-strict) wrongly downgrades Implemented ideas to Specified

Running `specscore spec lint --fix` rewrote `**Status:**` on 10 unrelated ideas during unrelated work: ideas at `Implemented` (e.g. `cli-telemetry`, `event-emit-dispatcher`, `entity-and-property-cli-support`, `lifecycle-verbs-for-idea-and-feature`, `ai-agent-configuration-cli`) were downgraded to `Specified`, and several `Specified` ideas were bumped to `Implementing`. The `idea-sync-lint-strict` autofix appears to re-derive each idea's status purely from its referencing features' states, ignoring that an idea legitimately reaches `Implemented` independently — a one-way lifecycle stage should not be silently downgraded by a sync pass.

Impact: a tree-wide `--fix` (even when scoped with `--ignore` to other rules) mutates the status of every idea whose features have advanced, producing incorrect, surprising diffs far outside the edit's intent. Surfaced when a single new idea's reference triggered status churn across the whole ideas index.

Fix options to evaluate: (a) make idea-sync status reconciliation monotonic (never downgrade `Implemented`→`Specified`); (b) gate the status-rewrite behind an explicit flag rather than the default `--fix`; (c) scope `--fix` to only the files passed/changed. Needs ideate with options + recommendation.
