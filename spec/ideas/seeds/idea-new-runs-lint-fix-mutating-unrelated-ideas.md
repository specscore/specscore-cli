---
captured_by: claude
status: queued
---
# specscore idea new runs lint --fix, mutating unrelated idea files

Observed 2026-06-03 while capturing the `canonical-lint-rule-catalog` idea. `specscore idea new <slug>` (0.5.0) runs an internal `spec lint --fix` after scaffolding, which reconciles the derived `**Status:**` / `**Promotes To:**` of EVERY idea in the tree — not just the new one. Creating one idea rewrote the Status of 9 unrelated ideas (e.g. `cli-self-update` Specified→Implementing, `ai-agent-configuration-cli` Implemented→Specified) plus their `spec/ideas/README.md` index rows.

Two compounding problems:

1. **Stale-binary drift.** The installed 0.5.0 binary's idea-sync derivation lags HEAD, so its `--fix` rewrote ideas into a state HEAD's linter then flags as drift (9 `idea-sync-lint-strict` violations against a HEAD-built binary). A current binary would not produce that specific drift — but:
2. **Surprising collateral.** Even a current binary mutating 9 unrelated files as a side effect of creating ONE idea is unexpected. A create verb should scaffold the new artifact and add its index row; whole-tree status reconciliation belongs to an explicit `lint --fix`, not `idea new`.

Impact: an agent/user creating an idea unknowingly stages cross-cutting status changes; against an older binary they are actively wrong. Workaround used here: revert the 9 collateral files, keep only the new idea, add its index row manually.

Suggested fix: scope `idea new`'s post-scaffold fix to the new artifact + its index row only (or drop the implicit fix and print a hint to run `spec lint --fix`).
