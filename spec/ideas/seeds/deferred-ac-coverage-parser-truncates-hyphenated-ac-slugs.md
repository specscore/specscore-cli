---
type: sidekick-seed
slug: deferred-ac-coverage-parser-truncates-hyphenated-ac-slugs
captured_at: 2026-06-03T14:00:00Z
captured_by: specstudio:specify
captured_during: null
trigger: heuristic
status: queued
synchestra_task: null
---
# Plan lint's Deferred AC Coverage parser truncates hyphenated AC slugs at the first hyphen

When a Plan's `## Deferred AC Coverage` bullet references a multi-hyphen AC slug — e.g. `- <feature>#ac:name-defaults-to-type — <reason>` — lint parses the AC id as only `#ac:name` (truncated at the first hyphen), then fires BOTH `P-001` (the real slug `name-defaults-to-type` is "not covered or deferred") and `P-002` (the truncated `#ac:name` is a "stale reference"). The `**Verifies:**` parser in tasks handles the same multi-hyphen slugs correctly (e.g. `#ac:auto-approve-always-approves` resolves fine), so the bug is specific to the Deferred-AC-Coverage line parser.

Impact: multi-hyphen ACs cannot be deferred at all — the only workaround is to cover them with a task. This is also the root cause of the long-standing 30-violation P-001/P-002 debt in `spec/plans/lint-fixed-files-report.md`, whose Deferred entries use hyphenated slugs (`clean-tree-exits-0` → reported as `#ac:clean`, etc.).

Fix: the Deferred-AC-Coverage AC-id regex should accept the same `[a-z0-9-]+` slug grammar the Verifies parser uses, terminating the id at whitespace (or the ` — ` reason separator), not at the first hyphen. Add a regression fixture with a hyphenated deferred slug. Likely also clears the lint-fixed-files-report.md debt.
