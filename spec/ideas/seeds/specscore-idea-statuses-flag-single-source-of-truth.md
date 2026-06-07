---
captured_by: specscore:change-status
status: queued
---
# Add `specscore idea --statuses` to expose the canonical idea status set + transitions as the single source of truth

The allowed Idea statuses and legal transitions are validated in the CLI (`pkg/lint/idea.go`) and documented only inside `specscore idea change-status --help`'s matrix — there is no machine-readable way to query them. So every consumer (the `specscore` plugin's `idea`/`change-status` skill docs, the `specstudio` skills) **hardcodes** the status set and transition matrix, and it drifts: when `Specifying` and the forward lifecycle (`Specified → Implementing → Implemented`) were added, the skill docs silently went stale and an agent treated `Specifying` as a bug. Same failure class as the `noop`/`auto-approve` rename: a contract duplicated across CLI + skills with no single source.

Proposed (per a maintainer suggestion): a flag, not a dedicated subcommand — `specscore idea --statuses` that prints the canonical status set and the legal-transition matrix (text + `--format json/yaml`). Skills then query it instead of copying the matrix into prose; a lint/CI check could even diff skill docs against it. Open questions: flag on `idea` vs a shared `specscore status list` (features have their own lifecycle too); include the feature-derived-status rules; whether `--format` should emit the derivation mapping. Needs ideate with options + recommendation.
