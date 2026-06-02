```yaml
feature: cli/spec/lint
revision: abe56b2
verdicts:
  - ac: cli/spec/lint#ac:clean-tree-exits-0
    verdict: unmapped
    justification: "no commits reference this AC"
  - ac: cli/spec/lint#ac:violations-exit-1
    verdict: unmapped
    justification: "no commits reference this AC"
  - ac: cli/spec/lint#ac:unknown-rule-name-exits-2
    verdict: unmapped
    justification: "no commits reference this AC"
  - ac: cli/spec/lint#ac:fix-idempotent
    verdict: unmapped
    justification: "no commits reference this AC"
  - ac: cli/spec/lint#ac:adherence-footer-fix-replaces-trailing-wrong-url
    verdict: unmapped
    justification: "no commits reference this AC"
  - ac: cli/spec/lint#ac:index-entries-fix-removes-phantom-row
    verdict: unmapped
    justification: "no commits reference this AC"
  - ac: cli/spec/lint#ac:index-entries-fix-inserts-orphan-row
    verdict: unmapped
    justification: "no commits reference this AC"
  - ac: cli/spec/lint#ac:index-entries-flags-orphan-child
    verdict: unmapped
    justification: "no commits reference this AC"
  - ac: cli/spec/lint#ac:oq-section-missing-flagged
    verdict: unmapped
    justification: "no commits reference this AC"
  - ac: cli/spec/lint#ac:oq-section-legacy-heading-flagged-and-fixed
    verdict: unmapped
    justification: "no commits reference this AC"
  - ac: cli/spec/lint#ac:dogfood-version-bump-flags-stale-pin
    verdict: unmapped
    justification: "no commits reference this AC"
  - ac: cli/spec/lint#ac:missing-specscore-yaml-exits-3
    verdict: unmapped
    justification: "no commits reference this AC"
  - ac: cli/spec/lint#ac:entity-and-property-rules-selectable
    verdict: unmapped
    justification: "no commits reference this AC"
  - ac: cli/spec/lint#ac:managed-section-fix-idempotent
    verdict: unmapped
    justification: "no commits reference this AC"
  - ac: cli/spec/lint#ac:consumer-path-multi-glob-parsed
    verdict: unmapped
    justification: "no commits reference this AC"
  - ac: cli/spec/lint#ac:fix-reports-only-changed-files
    verdict: pass
    justification: "fixed reports exactly one project-relative path (spec/...) and an empty array on the idempotent second run; fixed at abe56b2"
  - ac: cli/spec/lint#ac:fix-json-envelope-only-under-fix
    verdict: pass
    justification: "--fix json|yaml emits single {fixed, violations} object; no-fix emits bare array with no fixed key"
  - ac: cli/spec/lint#ac:fix-text-summary-on-stderr
    verdict: pass
    justification: "Fixed N file(s) summary written to stderr only when >0 files changed; stdout unchanged; idempotent"
  - ac: cli/spec/lint#ac:fix-report-needs-no-flag
    verdict: pass
    justification: "report gated only on --fix (+--format); no new enabling flag added"
```

# Verify Report — cli/spec/lint @ abe56b2

Per-AC verdicts for the Spec Lint Feature at revision `abe56b2`. Re-run after the project-relative path fix that the `bfdabdf` run flagged. The four new *Fixed-files reporting* ACs all pass; the fifteen pre-existing ACs carry no `Verifies:` trailers in branch history and are recorded `unmapped`.

**Tally:** 4 passed, 0 failed, 15 unmapped, 0 errored (19 total).

## AC: clean-tree-exits-0

**Verdict:** unmapped

**Justification:** No commits reference this AC.

**Commits:** No commits reference this AC.

## AC: violations-exit-1

**Verdict:** unmapped

**Justification:** No commits reference this AC.

**Commits:** No commits reference this AC.

## AC: unknown-rule-name-exits-2

**Verdict:** unmapped

**Justification:** No commits reference this AC.

**Commits:** No commits reference this AC.

## AC: fix-idempotent

**Verdict:** unmapped

**Justification:** No commits reference this AC.

**Commits:** No commits reference this AC.

## AC: adherence-footer-fix-replaces-trailing-wrong-url

**Verdict:** unmapped

**Justification:** No commits reference this AC.

**Commits:** No commits reference this AC.

## AC: index-entries-fix-removes-phantom-row

**Verdict:** unmapped

**Justification:** No commits reference this AC.

**Commits:** No commits reference this AC.

## AC: index-entries-fix-inserts-orphan-row

**Verdict:** unmapped

**Justification:** No commits reference this AC.

**Commits:** No commits reference this AC.

## AC: index-entries-flags-orphan-child

**Verdict:** unmapped

**Justification:** No commits reference this AC.

**Commits:** No commits reference this AC.

## AC: oq-section-missing-flagged

**Verdict:** unmapped

**Justification:** No commits reference this AC.

**Commits:** No commits reference this AC.

## AC: oq-section-legacy-heading-flagged-and-fixed

**Verdict:** unmapped

**Justification:** No commits reference this AC.

**Commits:** No commits reference this AC.

## AC: dogfood-version-bump-flags-stale-pin

**Verdict:** unmapped

**Justification:** No commits reference this AC.

**Commits:** No commits reference this AC.

## AC: missing-specscore-yaml-exits-3

**Verdict:** unmapped

**Justification:** No commits reference this AC.

**Commits:** No commits reference this AC.

## AC: entity-and-property-rules-selectable

**Verdict:** unmapped

**Justification:** No commits reference this AC.

**Commits:** No commits reference this AC.

## AC: managed-section-fix-idempotent

**Verdict:** unmapped

**Justification:** No commits reference this AC.

**Commits:** No commits reference this AC.

## AC: consumer-path-multi-glob-parsed

**Verdict:** unmapped

**Justification:** No commits reference this AC.

**Commits:** No commits reference this AC.

## AC: fix-reports-only-changed-files

**Verdict:** pass

**Justification:** `fixed` reports exactly one **project-relative** path (`spec/features/foo/README.md`) and an empty array on the idempotent second run. The `bfdabdf` specRoot-relative defect was fixed at `abe56b2` by prepending the project→specRoot prefix in the CLI layer.

**Commits:**
- 55394a4 — feat(lint): capture fix-modified file set in pkg/lint
- bfdabdf — feat(lint): report fix-modified files via JSON envelope and stderr summary
- abe56b2 — fix(lint): report fix-modified files relative to the project root

**Evidence:**
- `internal/cli/spec.go` — `fixedRel` computed via `filepath.Rel(root, specRoot)` + `filepath.Join` + `filepath.ToSlash` (commit abe56b2)
- live: run 1 → `"fixed":["spec/features/foo/README.md"]`; run 2 → `"fixed":[]`
- `internal/cli/spec_lint_fix_envelope_test.go`, `spec_lint_fix_text_summary_test.go` assert the `spec/`-prefixed path; pass

## AC: fix-json-envelope-only-under-fix

**Verdict:** pass

**Justification:** `--fix --format json|yaml` emits a single object with both `fixed` and `violations` keys; `--format json` without `--fix` emits a bare violations array with no `fixed` key (existing contract preserved).

**Commits:**
- bfdabdf — feat(lint): report fix-modified files via JSON envelope and stderr summary

**Evidence:**
- `internal/cli/spec.go` — `runSpecLint` gating; `lintFixEnvelope` struct
- `internal/cli/spec_lint_fix_envelope_test.go` — `TestSpecLint_FixJSONEnvelope`, `TestSpecLint_NoFixJSONBareArray` pass

## AC: fix-text-summary-on-stderr

**Verdict:** pass

**Justification:** Default text format under `--fix` writes a `Fixed N file(s):` summary naming each modified path to stderr only when ≥1 file changed; stdout carries only the violations report; the idempotent second run writes no summary.

**Commits:**
- bfdabdf — feat(lint): report fix-modified files via JSON envelope and stderr summary

**Evidence:**
- `internal/cli/spec.go` — text-mode branch writes to `cmd.ErrOrStderr()` when `fix && format=="text" && len(fixedRel) > 0`
- `internal/cli/spec_lint_fix_text_summary_test.go` — both tests pass

## AC: fix-report-needs-no-flag

**Verdict:** pass

**Justification:** The report is gated solely on `--fix` (plus optional `--format`); no new enabling flag was added.

**Commits:**
- bfdabdf — feat(lint): report fix-modified files via JSON envelope and stderr summary

**Evidence:**
- `internal/cli/spec.go` — only `--fix`/`--format` gate the report; no new flag registered
- `spec lint --help` shows no new report flag; tests exercise the report with `--fix` (+`--format`) only
