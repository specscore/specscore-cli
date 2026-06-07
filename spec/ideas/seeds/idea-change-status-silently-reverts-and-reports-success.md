---
captured_by: user
status: queued
---

# `specscore idea change-status` reports a successful transition to stdout but its internal `lint --fix` silently reverts the status, leaving the file unchanged

## Why

Running `specscore idea change-status <slug> --to=Specifying` on an Idea
with an empty `**Promotes To:**` printed the success line
`<slug>: Approved → Specifying` and exited 0 — but the Idea file remained at
`Status: Approved`. The verb rewrote the status, then ran `specscore spec
lint --fix` (to sync the ideas index), and that fix reverted the status to
`Approved` because `Specifying` requires a non-empty `Promotes To`. The
caller is told the transition succeeded when it did not. (Contrast the
`--to=Archived` path, which correctly detected the missing `Archive Reason`,
rolled back, and reported `lint failed after status rewrite (rollback
applied)` — the honest behavior.)

## Proposal

`change-status` should validate the target status's preconditions (e.g.
`Promotes To` required for `Specifying`+) BEFORE rewriting, OR treat any
post-rewrite `lint --fix` that mutates the status it just set as a failure:
roll back and exit non-zero with a clear message — never print a
`from → to` success line for a transition that did not persist.

## Repro

1. An Idea at `Status: Approved` with `**Promotes To:** —`.
2. `specscore idea change-status <slug> --to=Specifying`.
3. Observe: stdout `Approved → Specifying`, exit 0; file still `Approved`.

## Context

Hit while transitioning the `plan-status-lifecycle` Idea (specscore repo).
The silent revert made it look like the transition worked; only a follow-up
`grep` of the status line revealed it had not.
