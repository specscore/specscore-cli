---
type: sidekick-seed
slug: deferred-ac-coverage-regex-truncates-hyphenated-ac-slugs
captured_at: 2026-06-01T14:09:23Z
captured_by: specstudio:specify
captured_during: spec/features/cli/self-update
trigger: heuristic
status: resolved
synchestra_task: null
---
# Deferred AC Coverage regex truncates hyphenated AC slugs

**Resolved 2026-06-03** by commit `ec1cbea` (greedy id capture + whitespace-required separator, plus hyphenated-slug regression tests).

`pkg/plan/parse.go` `deferredEntryRe` = `^[-*]\s+(\S+?#ac:\S+?)\s*(?:[—–-]\s*(.*))?$`.

The AC-id capture `(\S+?#ac:\S+?)` is **non-greedy** and the separator class `[—–-]` includes the ASCII hyphen `-`. So for a real (hyphenated) AC slug like `cli/self-update#ac:version-flag-selects-tag`, the engine prefers the shortest id capture and splits at the first internal hyphen → id becomes `cli/self-update#ac:version`, reason becomes `flag-selects-tag — …`.

Consequences: any hyphenated AC slug listed under `## Deferred AC Coverage` is mis-parsed, firing **P-002** (stale ref `…#ac:version`) and leaving the real slug uncovered under **P-001**. Hyphenated slugs are the norm (e.g. `canonical-and-alias`), so deferral is effectively unusable for real specs. The bug is latent because the only parser test (`pkg/plan/parse_test.go`) uses single-letter slugs (`foo#ac:b`, `foo#ac:c`).

Fix options: make the id capture greedy (`(\S+#ac:\S+)`) and require the separator to be surrounded by whitespace, or restrict the separator to em/en-dash with mandatory spaces (` — `), or anchor the id to the full `\S+` and split reason on the first ` — `/` – `/` - ` with surrounding spaces only. Add a regression test with hyphenated slugs.

Independently reconfirmed in `specscore` 0.5.0 on 2026-06-03 while implementing the specstudio-skills approval-autonomy Plan (`reviewer-gates#ac:when-condition-masks-by-branch` read as `reviewer-gates#ac:when`), forcing a `**Verifies:**` line as the only lint-clean workaround. (Merged from duplicate seed `spec-lint-deferred-ac-coverage-parser-truncates-hyphenated`.)
