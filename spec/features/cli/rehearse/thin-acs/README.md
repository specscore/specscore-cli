---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: rehearse thin ACs + generated summary — `rehearse acs`

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/thin-acs?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/thin-acs?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/thin-acs?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rehearse/thin-acs?op=request-change) |
**Status:** Draft
**Source Ideas:** —
**Supersedes:** —

## Summary

A **thin acceptance criterion** is a standalone Markdown file (`_acs/<slug>.ac.md`) that states *what must be true* (intent) — no verification inside; proof is a scenario's job (checks + `**Use:**`). `specscore rehearse acs <feature-dir>` reads all the thin ACs in a feature's `_acs/` directory and generates the feature's `## Acceptance Criteria` **summary** — a denormalized read-model that embeds each AC's statement inline. This is the AC side of the shared-verification model: single source of truth in `_acs/`, a generated summary for readability, and (via the sibling reusable-checks feature) reusable proof.

## Problem

SpecScore's ACs are inline `### AC:` sections; Rehearse's are richer files. Neither gives, in one place, a *thin* AC (intent only) that both a spec system and a test runner share, plus a cheap way to read every AC at a glance. Two needs collide: **single source of truth** (one file per AC, reusable) argues for many small files; **readability** (see all ACs without opening N files — a real cost for humans *and* AI agents, which would otherwise spend one tool call per AC) argues for one place. A generated summary resolves the tension: author in `_acs/`, read from a committed projection.

## Behavior

### The thin AC artifact

#### REQ: thin-ac-format

A thin AC is `_acs/<slug>.ac.md` with a `# AC: <slug>` heading (its id), optional `**Verifies:**` (a URL to the feature/requirement, per Decision 0010), optional `**Status:**` and `**Applies-to:**` metadata, and a `## Statement` section (the "what must be true" prose). It carries **no verification** — that separation (intent vs proof) is the point.

### Generated summary

#### REQ: acs-generate

`specscore rehearse acs <feature-dir>` parses `<feature-dir>/_acs/*.ac.md` and renders a `## Acceptance Criteria` table — one row per AC (slug linked to its file, one-line statement, verifies target, status), sorted by slug for stable output. Non-`.ac.md` entries and subdirectories are ignored.

#### REQ: acs-read-model

The summary is a denormalized projection of the `_acs/` source of truth: it embeds each AC's *statement* (not just a link), so a reader gets every AC's intent inline in one read. By default the table prints to stdout; `--write` regenerates the `## Acceptance Criteria` section in `<feature-dir>/README.md` in place (replacing an old inline AC section if present — a migration in one pass), leaving the rest of the README untouched.

#### REQ: acs-errors

A missing `_acs/` directory, or a `.ac.md` file with no `# AC:` heading, fails with a clear, actionable error (exit 2). `--write` with no `README.md` present likewise errors.

## Architecture & Components

- `internal/rehearse/scenario/ac.go` — `AC` struct, `ParseAC`/`ParseACBytes`, and `GenerateACSummary` (the table renderer).
- `internal/cli/rehearse_acs.go` — the `rehearse acs` subcommand and `injectACSummary` (idempotent section replacement).

## Testing Strategy

Unit tests for parsing (all metadata, statement, missing heading), summary generation (sorting, one-lining/escaping, empty-field dashes, linking), the subcommand (print, write, migrate-inline, all error paths), and `injectACSummary` (append/replace/end-of-file). One e2e scenario runs the real CLI against a fixture feature. 100% coverage of touched packages; wired into the corpus.

## Not Doing / Out of Scope

- **Proven-by backlinks** in the summary (which scenarios/checks prove each AC) — a v2 enhancement once checks carry `Proves:`.
- **Drift-check mode** (fail CI if the committed summary is stale) — follow-up; `--write` + review suffices for the MVP.
- **Ecosystem migration** of every inline `### AC:` — the `specscore migrate` work; `--write` migrates one feature at a time.

## Acceptance Criteria

### AC: thin-ac-parses

Scenario: a thin AC file parses
Given `_acs/session-hardened.ac.md` with a `# AC:` heading, `**Status:**`, and a `## Statement`
When it is parsed
Then the slug, status, and statement are extracted; a file with no `# AC:` heading errors

### AC: summary-generated

Scenario: the AC summary is generated
Given a feature with two thin ACs in `_acs/`
When I run `specscore rehearse acs <feature-dir>`
Then a `## Acceptance Criteria` table is printed, one row per AC, sorted by slug, each embedding the AC's statement

### AC: summary-written

Scenario: `--write` regenerates the README section
Given a feature README with an existing `## Acceptance Criteria` section
When I run `specscore rehearse acs <feature-dir> --write`
Then the section is replaced with the generated table and the rest of the README is preserved

### AC: acs-missing-dir-errors

Scenario: missing `_acs/` errors
When I run `specscore rehearse acs <dir>` where `<dir>/_acs` does not exist
Then the command exits 2 with a clear error

## Open Questions

- Should the summary include a "proven by" column now, or wait until checks carry `Proves:`? (Leaning: wait — keep this feature to generation.)

## Autonomous Decisions

- Homed on the **Rehearse** side (`rehearse acs`), not a SpecScore lint rule, consistent with Rehearse owning the AC format; a lint-integrated drift check can follow.
- The generated section is delimited by the `## Acceptance Criteria` H2 heading (to the next H2), so regeneration also migrates an old inline `### AC:` (H3) section in one pass.

---
*This document follows the https://specscore.md/feature-specification*
