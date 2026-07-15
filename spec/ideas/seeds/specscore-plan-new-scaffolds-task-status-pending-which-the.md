---
captured_by: user
status: queued
---
# specscore plan new scaffolds task Status: pending, which the linter immediately rejects as P-004 legacy — a fresh untouched plan does not lint clean

## Problem

`specscore plan new <slug> --feature <f>` emits `**Status:** pending` on every scaffolded task. `specscore spec lint` then rejects exactly that value. A freshly-created, untouched plan is not lint-clean.

Reproduced on CLI 0.18.0:

```console
$ specscore plan new scratch-repro --feature chat-messenger
$ grep -c '^\*\*Status:\*\* pending' spec/plans/scratch-repro.md
2
$ specscore spec lint
plans/scratch-repro.md:27 [error] P-004: Task 1: legacy **Status:** value "pending"; use canonical "planning" (run --fix to migrate)
plans/scratch-repro.md:35 [error] P-004: Task 2: legacy **Status:** value "pending"; use canonical "planning" (run --fix to migrate)
```

The scaffold is the CLI's own single source of truth for plan structure — SpecStudio's `plan` skill treats it as required-CLI with no hand-authored fallback — so the producer and the validator disagree on a value the producer itself writes.

## Suggested direction

Emit `planning` from the `plan new` scaffold. One-line fix at the template, and it removes a mandatory `--fix` round-trip from every plan authored through the skill.

Worth checking whether `pending` is still accepted anywhere as an input alias; if it is, the scaffold should still prefer the canonical spelling it wants back.

## Provenance

Surfaced 2026-07-15 dogfooding `specstudio:plan` in `sneat-co/sneat-cli` (CLI 0.18.0). The skill mandates `specscore plan new` as the only creation path and permits exactly one `--fix` pass; that budget was spent entirely on the scaffold's own output rather than on anything the author wrote.
