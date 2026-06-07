---
captured_by: user
status: queued
---

# specscore CLI should expose a plan verb with list and query subcommands

Plans (`spec/plans/*.md`) are not a first-class CLI artifact. The top-level verbs include `idea`, `feature`, `task`, `issue`, `proposal` — but **no `plan`**. `specscore plan` errors with `Unknown command "plan"`.

Consequence: there is no way to list plans or read their lifecycle status via the CLI. Answering "what plans are open?" requires `ls spec/plans/ && grep '^\*\*Status:\*\*'` by hand, and stale plan statuses (e.g. a plan still `Approved` while its Feature is already `Stable`) can't be surfaced programmatically.

Proposed scope (mirror the existing `feature`/`idea` verbs):
- `specscore plan list` — slugs, one per line; `--fields status`, `--format json|yaml`, `--status <X>` filter.
- `specscore plan info <slug>` — metadata for one plan.
- Consider `plan new` / lifecycle transitions for parity, if plans are meant to be CLI-managed rather than plain markdown.

Open design question: are plans *intended* to be CLI-queryable, or deliberately just files? Resolve that before adding the verb.

Captured from a session reviewing open artifacts where the missing `plan` verb forced manual grepping.
