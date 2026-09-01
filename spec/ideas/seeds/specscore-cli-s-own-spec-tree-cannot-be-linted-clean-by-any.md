---
captured_by: user
status: queued
---
# specscore-cli's own spec tree cannot be linted clean by any released specscore — the benchmark-fixture skip predicate is unreleased, and CI only lints with a source build so it cannot catch this

## Problem

`specscore-cli`'s own spec tree cannot be linted clean by any *released* `specscore`. CI is green only because `dogfood.yml` builds the linter from source, so nothing catches it.

```console
$ specscore --version        # 0.18.0, and 0.19.0 (latest)
$ specscore spec lint        # in specscore-cli
19 violation(s) found        # exit 1

$ go build -o /tmp/head ./cmd/specscore && /tmp/head spec lint
0 violations found           # exit 0
```

All 19 are the linter walking into the hermetic benchmark fixture at `spec/features/cli/studio/answers/benchmark/testdata/fixture/` and judging it as project spec content: `readme-exists` ×9, `oq-section` ×7, `index-entries` ×2, `format-field` ×1.

Release-timing gap, not a code defect. The skip predicate (`isReservedFixturePath`, `pkg/lint/linter.go:282`, matching `reservedFeatureSubtree = "benchmark"`) came in **3523a77** — the commit that added the fixture. `git merge-base --is-ancestor 3523a77 v0.19.0` → **false**. Both landed 2026-07-10; the tag caught the fixture and missed the rule.

No escape hatch exists either: no `.specscoreignore`, no exclude key in `specscore.yaml`, and `--ignore` disables *rules*, not paths. An adopter's only remedy is editing the fixture — which corrupts the benchmark, whose README says its parameters are real entities "so the same file scores against a live Sneat index and the committed fixture alike".

## Suggested direction

The immediate gap closes on the next release; no code change needed.

The durable fix is the CI blind spot: linting with a **source build** structurally cannot detect "the released binary can't lint this repo". A release-time check using the **release artifact** would have caught it at tag time.

Separately worth deciding whether path exclusion should exist at all — a hardcoded `"benchmark"` constant fixes this fixture by name; any other project committing a fixture tree under `spec/` has no recourse.

## Provenance

Surfaced 2026-07-15 dogfooding from an unrelated project. Live failure: it produced a false "the spec tree is red" report, and the obvious remedy — "fix the violations" — would have meant mutating a benchmark fixture to satisfy a linter HEAD already skips. Updating 0.18.0 → 0.19.0 did not help; the fix is unreleased.
