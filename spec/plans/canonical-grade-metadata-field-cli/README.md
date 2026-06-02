# Plan: Grade Lint Support (CLI)

**Status:** draft
**Features:**
  - [cli/spec/lint](../../features/cli/spec/lint/README.md)
**Source type:** feature
**Source:** [cli/spec/lint](../../features/cli/spec/lint/README.md)
**Author:** alexander.trakhimenok
**Created:** 2026-05-29
**Effort:** S
**Impact:** medium

## Context

Implements the CLI half of the upstream meta-spec Feature
[canonical-grade-metadata-field](https://github.com/specscore/specscore/blob/main/spec/features/canonical-grade-metadata-field/README.md)
(see its [Plan](https://github.com/specscore/specscore/blob/main/spec/plans/canonical-grade-metadata-field.md)).
That Feature promotes `**Grade:**` into the canonical SpecScore schema as an
optional, single-value quality grade validated against a configurable value set
(default `A, B, C, D, F`), decoupled from reviewer-gates and from an artifact's
`Status`. The spec repo owns the canonical-doc edit; this plan delivers the
`specscore` CLI support and extends this repo's
[cli/spec/lint](../../features/cli/spec/lint/README.md) feature: config parsing
of `grade.values`, header-block parsing of the `**Grade:**` line, value
validation in `specscore spec lint`, and uniform / decoupled rule application.

This Plan supersedes the closed per-task trackers specscore-cli#20–23, which are
folded into the four tasks below.

## Acceptance criteria

Coverage mirrors the upstream Feature's eight ACs (`canonical-grade-metadata-field#ac:*`):

- `grade-values-shape-errors` — malformed `grade.values` (empty list, scalar, empty entry) is a hard error; no implicit set is substituted.
- `grade-absent-is-valid` — a gradeable artifact with no `**Grade:**` line lints clean.
- `grade-single-token-enforced` — an empty or multi-token `**Grade:**` value is a hard error.
- `grade-placement-enforced` — `**Grade:**` must be the last header-block line after `**Status:**`; misplaced is a hard error; `--fix` normalizes it.
- `grade-default-scale-validated` — value validated against the default `A, B, C, D, F` when no config is present; out-of-set is a hard error.
- `grade-custom-scale-validated` — value validated against a repo-configured `grade.values` set.
- `grade-on-any-header-block-kind` — the rule applies to every header-block artifact kind, with no per-kind exemption.
- `grade-decoupled-from-workflow-and-status` — a valid grade on a `Draft` artifact in a repo with no reviewer gates passes.

## Tasks

### 1. Config: parse `grade.values` and resolve the effective set

Add `grade.values` parsing to the config loader (`pkg/projectdef/` or equivalent). Resolve the effective value set: the configured list when present, otherwise the built-in default `A, B, C, D, F`. Validate shape — a non-empty list of non-empty scalar tokens; an empty list, a scalar, or an empty/non-scalar entry is a hard error naming the `grade-values-shape` rule with no implicit set substituted; duplicate tokens emit an advisory and are de-duplicated. Covers `grade-values-shape-errors`.

### 2. Header-block parse of `**Grade:**` plus `--fix` placement normalization

Extend the body-metadata / header-block parser to recognize an optional `**Grade:**` line: absence is valid (no error); exactly one non-empty token is required (empty or multi-token is a hard error); placement must be the last line of the contiguous header block, after `**Status:**` and any `**Source Ideas:**` / `**Supersedes:**` lines. A misplaced or out-of-block Grade is a hard error, and `specscore spec lint --fix` normalizes it to the canonical last position. Covers `grade-absent-is-valid`, `grade-single-token-enforced`, `grade-placement-enforced`.

### 3. Lint value validation against the effective set

Add the lint check that validates a parsed `**Grade:**` value against the effective set resolved in Task 1: in-set passes; out-of-set is a hard error naming the artifact, the offending value, and the effective set. Exercise both the built-in default `A, B, C, D, F` (no `grade:` block) and a repo-configured set (e.g. `grade.values: [1, 2, 3, 4, 5]`). Covers `grade-default-scale-validated`, `grade-custom-scale-validated`.

### 4. Kind-agnostic and Status/workflow-decoupled application

Register and apply the Grade lint rule uniformly for every artifact kind that has a header block (Feature, Plan, Idea, Decision, Task), reusing the shared header-block parser rather than re-deriving placement logic, with no per-kind allow-list or exemption. Ensure the rule never gates on `Status` or on any reviewer-gate / workflow artifact: a valid grade on a `Draft` artifact in a repository that declares no reviewer gates passes. Covers `grade-on-any-header-block-kind`, `grade-decoupled-from-workflow-and-status`.

## Open Questions

None at this time. (Resolved upstream: the default value set is `A, B, C, D, F` — no `E`. Task 1 implements that default; repos needing a different scale declare `grade.values`.)

---
*This document follows the https://specscore.md/plan-specification*
