```yaml
feature: cli/spec/lint
revision: bfdabdf
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
    verdict: fail
    justification: "fixed paths are specRoot-relative (features/foo/README.md) not project-relative (spec/...) as the REQ and AC require"
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

# Verify Report — cli/spec/lint @ bfdabdf

Per-AC verdicts for the Spec Lint Feature at revision `bfdabdf`. This run targeted the four new *Fixed-files reporting* ACs (mapped to `Verifies:` commits on branch `lint-fixed-files-report`); the fifteen pre-existing ACs carry no `Verifies:` trailers in branch history and are recorded `unmapped`.

**Tally:** 3 passed, 1 failed, 15 unmapped, 0 errored (19 total).

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

**Verdict:** fail

**Justification:** The two core behaviors hold — `fixed` contains exactly the one changed file and is empty on the idempotent second run — but the path is reported **specRoot-relative** (`features/foo/README.md`) whereas REQ `fix-reports-modified-files` ("MUST report paths relative to the project root") and this AC ("project-relative path") require the `spec/` prefix (`spec/features/foo/README.md`). `snapshotSpecTree` uses `filepath.Rel(specRoot, path)`.

**Commits:**
- 55394a4 — feat(lint): capture fix-modified file set in pkg/lint
- bfdabdf — feat(lint): report fix-modified files via JSON envelope and stderr summary

**Evidence:**
- `pkg/lint/lint.go` — `snapshotSpecTree` keys paths via `filepath.Rel(specRoot, path)` (specRoot-relative)
- `spec/features/cli/spec/lint/README.md` REQ `fix-reports-modified-files` ("relative to the project root") and AC `fix-reports-only-changed-files` ("project-relative path")
- live: `spec lint --fix --format json` emitted `"fixed":["features/foo/README.md"]`; second run emitted `"fixed":[]`

## AC: fix-json-envelope-only-under-fix

**Verdict:** pass

**Justification:** `--fix --format json|yaml` emits a single object with both `fixed` and `violations` keys; `--format json` without `--fix` emits a bare violations array with no `fixed` key (existing contract preserved).

**Commits:**
- bfdabdf — feat(lint): report fix-modified files via JSON envelope and stderr summary

**Evidence:**
- `internal/cli/spec.go` — `runSpecLint` gating (`fix && json|yaml → outputLintFixEnvelope`; else bare array); `lintFixEnvelope` struct
- `internal/cli/spec_lint_fix_envelope_test.go` — `TestSpecLint_FixJSONEnvelope`, `TestSpecLint_NoFixJSONBareArray` pass
- live: `--fix` produced `{fixed,violations}`; no-fix produced bare array

## AC: fix-text-summary-on-stderr

**Verdict:** pass

**Justification:** Default text format under `--fix` writes a `Fixed N file(s):` summary naming each modified path to stderr only when ≥1 file changed; stdout carries only the violations report; the idempotent second run writes no summary.

**Commits:**
- bfdabdf — feat(lint): report fix-modified files via JSON envelope and stderr summary

**Evidence:**
- `internal/cli/spec.go` — text-mode branch writes to `cmd.ErrOrStderr()` when `fix && format=="text" && len(res.Fixed) > 0`
- `internal/cli/spec_lint_fix_text_summary_test.go` — `TestSpecLint_FixTextSummaryOnStderr`, `TestSpecLint_FixTextSummaryIdempotentNoStderr` pass

## AC: fix-report-needs-no-flag

**Verdict:** pass

**Justification:** The report is gated solely on `--fix` (plus optional `--format`); no new enabling flag was added — confirmed by the flag registration, `--help`, and tests that pass using only `--fix`.

**Commits:**
- bfdabdf — feat(lint): report fix-modified files via JSON envelope and stderr summary

**Evidence:**
- `internal/cli/spec.go` — only `--fix`/`--format` gate the report; no `--report-fixed`/`--show-fixed` registered
- `spec lint --help` shows no new report flag
- tests exercise the report with `--fix` (+`--format`) only
