---
name: specscore-task
description: Correct Task lifecycle annotations with a truthful singleton record and append-only audit provenance.
---

# SpecScore Task annotations

<!-- capability: specscore.task.annotation-amend -->

Use `task amend` only to repair Note/Evidence context. It never transitions a
task and never changes implementation provenance. Supply a truthful actor and
reason; replacement and removal are explicit, and each successful correction
adds an immutable audit line with a UTC time and prior-content digest. Read the
artifact first; if duplicate or malformed singleton fields are found, repair
the ambiguity deliberately rather than asking the command to guess.
The command uses one fail-fast artifact transaction. On contention or a
changed preimage, re-read the artifact and decide again; never hand-apply the
amendment over another writer's bytes.

```sh
specscore task amend land-plan --plan release --note "merged to main" --evidence "834392f,https://github.com/specscore/specscore-cli/pull/140" --actor codex --reason "replace stale awaiting-merge record"
```

Use `--clear-note` or `--clear-evidence` only when that annotation should no
longer exist. Do not use an empty replacement as an implicit clear.
