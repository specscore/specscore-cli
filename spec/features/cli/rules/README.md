# Feature: Lint Rule Catalog

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rules?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rules?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rules?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/rules?op=request-change) |
**Status:** Approved
**Date:** 2026-06-03
**Owner:** alexander.trakhimenok
**Source Ideas:** canonical-lint-rule-catalog
**Supersedes:** —

## Summary

A single, always-accurate catalog of every SpecScore lint rule, derived from the code registry. Adds a `specscore rules` command for runtime discovery, a generated `docs/lint-rules.md` reference, and a `--check` drift-guard so the catalog can never silently fall out of sync with the rules the linter actually runs. Serves CLI end users (discover what a rule enforces, what to `--ignore`) and maintainers (one place rules are documented, enforced in CI).

## Synopsis

```
specscore rules                 # list all rules: id, family, description
specscore rules --family idea   # filter to one family
specscore rules --format json   # machine-readable output
specscore rules --write         # (re)generate docs/lint-rules.md from the registry
specscore rules --check         # fail (non-zero) if docs/lint-rules.md is out of sync
```

## Problem

SpecScore ships ~113 lint rules across families (`idea-*`, `D-*`, `DI-*`, `I-*`, plan/feature/misc). The de-facto source of truth is the `allRuleNames` registry in `pkg/lint/lint.go` plus per-family ID slices, but it carries only rule *names* — no descriptions — and human documentation is fragmented: idea rules are partially tabled in the lint README, issue rules elsewhere, and decision rules nowhere. New rules get registered in code with no doc home and no discovery surface, and any hand-maintained list drifts from the registry the moment a rule is added. Users running `specscore spec lint` have no authoritative place to learn what a rule enforces.

## Behavior

### Rule registry (single source of truth)

The registry is the one place a rule is declared, and it carries everything needed to document and discover that rule. Everything else — the command output and the generated catalog — is derived from it.

#### REQ: structured-registry

The lint rule registry MUST declare each rule as a structured entry carrying a non-empty rule `id`, a non-empty one-line human-readable `description`, a `family`, and a `severity`. A rule MUST NOT be registrable without a description.

#### REQ: registry-rule-parity

Every rule ID emitted by any checker MUST have a corresponding registry entry, and every registry entry MUST correspond to a rule a checker can emit. Orphan entries (no emitting checker) and unregistered emitted IDs MUST be detectable and reported as a failure.

### Discovery command

`specscore rules` is the runtime surface for humans and tooling to enumerate the rules.

#### REQ: rules-command-lists

`specscore rules` MUST print every registered rule with its `id`, `family`, and `description`, in a deterministic order, and exit `0`.

#### REQ: rules-filter-and-format

`specscore rules` MUST support filtering to a single family via `--family <name>` and MUST support `--format text` (default) and `--format json`. JSON output MUST expose the same per-rule fields as the registry entry.

### Catalog generation

The committed catalog is a rendering of the registry — never hand-authored rule entries.

#### REQ: catalog-deterministic

The catalog MUST be rendered deterministically from the registry: generating it twice without any registry change MUST produce byte-identical output (stable ordering, no timestamps or environment-dependent content).

#### REQ: catalog-canonical-path

The canonical catalog MUST live at `docs/lint-rules.md`, grouped by family, each rule showing its `id` and `description`.

#### REQ: catalog-write

`specscore rules --write` MUST (re)generate `docs/lint-rules.md` from the current registry, so a registry change is reflected in the committed catalog by re-running the command.

### Drift-guard

The guard is what makes the catalog trustworthy: the committed file is provably the registry's rendering.

#### REQ: rules-check-detects-drift

`specscore rules --check` MUST regenerate the catalog in-memory and compare it to the committed `docs/lint-rules.md`. It MUST exit non-zero and report the divergence when they differ (including when the file is missing), and exit `0` writing nothing when they match.

#### REQ: ci-runs-check

The project CI MUST invoke `specscore rules --check`, so that adding, removing, or renaming a rule, or changing a description, without regenerating the catalog fails the build.

## Acceptance Criteria

### AC: registry-has-metadata (verifies REQ:structured-registry)

**Given** the lint rule registry
**When** its entries are inspected
**Then** every entry exposes a non-empty `id`, a non-empty one-line `description`, a `family`, and a `severity`, and no entry can be added with an empty description.

### AC: registry-parity-enforced (verifies REQ:registry-rule-parity)

**Given** a checker that emits a rule ID absent from the registry, or a registry entry no checker emits
**When** the parity check runs
**Then** it names the offending ID and fails.

### AC: rules-lists-all (verifies REQ:rules-command-lists)

**Given** the specscore CLI
**When** the user runs `specscore rules`
**Then** every registered rule is printed with its `id`, `family`, and `description` in a deterministic order, and the command exits `0`.

### AC: rules-filter-and-json (verifies REQ:rules-filter-and-format)

**Given** the user wants a subset in machine-readable form
**When** they run `specscore rules --family idea --format json`
**Then** only `idea-*` rules are emitted as JSON objects carrying the same fields as the registry entry.

### AC: catalog-deterministic-render (verifies REQ:catalog-deterministic)

**Given** an unchanged registry
**When** the catalog is generated twice
**Then** the two outputs are byte-identical.

### AC: catalog-at-canonical-path (verifies REQ:catalog-canonical-path)

**Given** catalog generation
**When** the catalog is written
**Then** the output file is `docs/lint-rules.md`, grouped by family, each rule showing its `id` and `description`.

### AC: catalog-write-regenerates (verifies REQ:catalog-write)

**Given** a registry that gained, lost, or re-described a rule
**When** the user runs `specscore rules --write`
**Then** `docs/lint-rules.md` is rewritten to reflect the current registry.

### AC: check-detects-drift (verifies REQ:rules-check-detects-drift)

**Given** `docs/lint-rules.md` differs from the registry rendering (or is missing)
**When** the user runs `specscore rules --check`
**Then** the command reports the divergence and exits non-zero; when the file matches, it exits `0` and writes nothing.

### AC: ci-fails-on-stale-catalog (verifies REQ:ci-runs-check)

**Given** the CI pipeline
**When** a rule is added, removed, or renamed, or a description changes, without regenerating the catalog
**Then** the `specscore rules --check` CI step fails the build.

## Rehearse Integration

All ACs are CLI-observable (command output, exit code, generated file) or unit-testable (registry inspection, parity check). Rehearse stubs are deferred to the Plan/Implement phase, where each AC maps to a CLI or unit test; no AC is subjective or doc-only.

## Not Doing / Out of Scope

- **Per-rule long-form docs or rationale pages** — the MVP renders one-line descriptions; deep per-rule documentation can follow.
- **Rule deprecation/versioning lifecycle** — this is a catalog, not a rule-lifecycle system.
- **Changing any rule's behavior, severity, or ID** — purely additive: metadata, surfacing, and the drift-guard only.
- **Migrating/removing the existing partial idea-rules table** in `spec/features/cli/spec/lint/README.md` — tracked as an Open Question, not done in this MVP.
- **A web or hosted catalog surface** — `specscore rules` plus the committed markdown file are the only surfaces.

## Open Questions

- Should `--format` also offer `yaml`, or are `text` and `json` sufficient?
- Once the generated `docs/lint-rules.md` exists, should the partial idea-rules table in `spec/features/cli/spec/lint/README.md` be removed in favor of a pointer to the catalog (separate cleanup)?
- Should `specscore rules --check` be folded into `spec lint` later as a convenience, or stay a distinct command invoked directly in CI?

---
*This document follows the https://specscore.md/feature-specification*
