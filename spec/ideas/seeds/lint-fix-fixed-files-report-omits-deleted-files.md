---
captured_by: user
status: queued
---

# lint --fix fixed-files report omits files deleted by a fixer

`diffSnapshots` in `pkg/lint/lint.go` computes the fix-modified file set by diffing a content-hash snapshot of the spec tree taken before and after the fix pass. It reports files that were **added or modified**, but not files a fixer **deleted** (present in the before-snapshot, absent in the after-snapshot).

Harmless today: no SpecScore fixer deletes spec files — they rewrite or append in place (even "delete phantom index row" edits a file rather than removing one). So the `fixed` report is complete for every current fixer, and the producer Idea's assumption ("every fixer writes only inside the spec root") holds.

Revisit if a future fixer ever deletes a file: the `--fix` fixed-files report would silently miss the deletion, and a consumer staging `.fixed[]` would not `git add` (well, `git rm`) the removed path. Fix would be to also diff for keys present in before-snapshot but absent in after-snapshot and report them (possibly distinguished as deletions).
