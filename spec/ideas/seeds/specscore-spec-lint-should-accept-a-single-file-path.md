---
type: sidekick-seed
slug: specscore-spec-lint-should-accept-a-single-file-path
captured_at: 2026-06-04T00:00:00Z
captured_by: user
captured_during: null
trigger: explicit
status: queued
synchestra_task: null
---

# specscore spec lint should accept a single-file path argument

`specscore spec lint` currently only runs tree-wide; passing a path errors with `Unknown command "<path>" for "specscore spec lint"`. There's no way to lint a single specific file.

Why it's wanted: after writing/editing one artifact (e.g. a new seed), you want to confirm *that file* is clean without scanning the whole tree and grepping the output for the relevant lines.

Proposed: `specscore spec lint <path>...` lints just the named file(s)/dir(s). Tree-wide remains the default when no path is given. Useful for editor/save hooks and tight agent edit loops.

Cross-artifact rule handling (decided): some lint rules are cross-artifact (e.g. idea↔feature sync, dangling refs). Single-file mode loads the relevant/linked artifacts for context so those rules still evaluate correctly, but only reports violations whose **primary location is the named file**. Violations whose primary location is another artifact are suppressed from the single-file run (they'd surface in a tree-wide run or when that file is the target).

Captured during a session where the lack of single-file lint forced a tree-wide run + grep.
