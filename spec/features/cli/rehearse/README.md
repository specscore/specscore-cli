---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: rehearse — acceptance-evidence runner command group

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse?op=request-change) |
**Status:** Stable
**Source Ideas:** —

## Summary

Command group for Rehearse — SpecScore's acceptance-evidence layer. Rehearse executes Markdown **scenario** files that pair an acceptance criterion with the real commands that verify it, reports per-scenario pass/fail, and (via the `evidence` child) feeds the results back into the SpecScore fact store as verified-behavior. This umbrella groups the `run` and `new` command surfaces and their refinements; per-capability detail lives in the child features under [Contents](#contents). A plain-language companion for newcomers lives at [`REHEARSE.md`](../../../../REHEARSE.md). Origin: v0.3 fold-in per the rehearse repo's `spec/ideas/rehearse-evidence-layer.md`.

## Contents

| Child | Description |
|---|---|
| [run](run/README.md) | Execute scenario files — ordered steps (bash/sql/dtql/hurl/graphql) in a sandboxed per-scenario workdir with a shared context bag — and report per-scenario pass/fail as human text or JSON |
| [evidence](evidence/README.md) | Persist rehearse run JSON reports and ingest them at studio index time as verified-behavior facts |
| [new](new/README.md) | Scaffold a Rehearse scenario file pre-populated with Given/When/Then structure and Verifies metadata from a feature's acceptance criterion |
| [new-dry-run](new-dry-run/README.md) | Preview scaffold markdown without writing files or committing to git |
| [file-assertions](file-assertions/README.md) | Verify filesystem state (file existence, content, permissions) in scenario assertions |
| [run-filter](run-filter/README.md) | Run scenarios by acceptance criterion with --filter flag |
| [file-assertions-glob](file-assertions-glob/README.md) | Glob patterns in file assertion paths for set-based matching |
| [file-assertions-glob-recursive](file-assertions-glob-recursive/README.md) | Recursive `**` glob matching in file assertion paths via doublestar |
| [expected-fail](expected-fail/README.md) | `**Expect:** fail` scenario directive so negative scenarios run green in the corpus |
| [reusable-checks](reusable-checks/README.md) | `**Use:**` directive to invoke reusable, parameterized verification units (checks) — write verification once, reuse across scenarios |
| [thin-acs](thin-acs/README.md) | Thin acceptance criteria (`_acs/*.ac.md`) + `rehearse acs` generated `## Acceptance Criteria` summary (read-model) |
| [nested-suites](nested-suites/README.md) | Nested Given/When/Then suites (describe/context/it) — multiple `### When` branches from one shared Given, each run as an isolated case |
| [file-assertions-glob-braces](file-assertions-glob-braces/README.md) | Brace expansion (`{a,b}`) in file assertion glob paths via doublestar |

## Problem

Documentation and acceptance criteria drift away from the software they describe. A feature's README says "the command exits 0 and writes `output.json`", the code changes months later, and the promise silently becomes false — nothing connects the written criterion to the running program. Reviewers and agents then trust claims that no longer hold. Rehearse closes this gap by making each acceptance criterion **executable**: the criterion and the commands that verify it live together, run for real, and produce evidence of whether the promise currently holds.

## Behavior

A **scenario** is a Markdown file that links to an acceptance criterion (`**Verifies:** <feature>#ac:<slug>`) and contains fenced **step** blocks — real commands in `bash`, `sql`, `dtql`, `hurl`, or `graphql` — plus optional `### Assert: file` checks on the resulting filesystem.

`specscore rehearse run <files-or-dirs>` executes each scenario:

- in its own throwaway working directory, so scenarios never see each other's state;
- steps run top-to-bottom, sharing a **context bag** — a step can capture a value and later steps interpolate it via `{{name}}`;
- the first failing step fails the scenario and the rest are skipped-after-failure; a scenario with no step blocks is *no-steps*;
- a scenario whose step needs a binary that is not installed is *skipped* with a warning, not failed;
- `### Assert: file` headings then check existence / absence / content / permissions, with glob support (`*`, `**`, `{a,b}`);
- results are reported per scenario as a human summary or JSON, and `--report-out` writes a JSON envelope the `evidence` feature ingests into the SpecScore fact store as verified-behavior.

Two refinements shape a run: `--filter <ac-ref>` runs only scenarios verifying a given criterion, and `**Expect:** fail` marks a *negative* scenario (one that proves the tool rejects bad input) so its correct failure counts as a pass.

`specscore rehearse new <feature>#ac:<slug>` scaffolds a scenario pre-filled from the criterion's Given/When/Then text (`--dry-run` to preview, `--commit` to stage it), so authoring starts from the promise rather than a blank file.

Rehearse is **self-hosting**: its own acceptance scenarios (the committed corpus) are run by `rehearse run` in CI.

## Design principles

These hold across every Rehearse capability:

- **Real execution, never mock.** Scenarios run the actual commands against the real filesystem. A test that fakes a dependency into succeeding proves nothing — the value of Rehearse is that a green scenario reflects real behavior. When a needed binary is genuinely absent, a scenario is *skipped*, never faked.
- **Deterministic and sandboxed.** Steps run in order, one scenario at a time, each in an isolated throwaway working directory. Determinism is preferred over speed because the output is *evidence*, not merely a signal.
- **Parse/run separation.** Reading a scenario file (the `scenario` package) never executes anything; execution is a separate stage (the `runner`). This keeps parsing safe to reason about and independently testable.
- **Acceptance level, not unit level.** Rehearse verifies user-facing promises (acceptance criteria); Go unit tests cover the internal pieces. The two are complementary, not substitutes.

## Acceptance Criteria

This is a command-group umbrella; acceptance criteria are defined and verified per child feature (see [Contents](#contents)). This feature adds none of its own.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
