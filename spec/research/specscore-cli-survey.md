# Survey: SpecScore CLI

**Status:** Current
**Date:** 2026-05-31
**Repo SHA:** 322945d4b641e59a32057bebdf7da260d61be55c
**Scope:** repo root
**JSON:** spec/research/specscore-cli-survey.json

## Summary

SpecScore CLI is a Go module for the SpecScore command line. The repository has 408 tracked files: command entrypoints under `cmd/`, command wiring under `internal/cli/`, reusable domain packages under `pkg/`, and a dogfooded SpecScore tree under `spec/`. This survey is inferred from file paths and allowlisted manifests only; source implementation files were not read.

## Architecture

- `cmd/specscore/` is the CLI entrypoint surface.
- `internal/cli/` is the command-layer area, with tests visible by filename.
- `pkg/` holds reusable packages for linting, artifact parsing, events, task handling, source references, lifecycle operations, and supporting utilities.
- `spec/features/cli/`, `spec/ideas/`, and `spec/plans/` show the repository dogfoods SpecScore for its own CLI behavior.
- GitHub Actions define Go CI, dogfood lint, and release workflows.

## Directory Clusters

| Path | Files | Survey note |
|---|---:|---|
| `cmd/` | 4 | CLI entrypoint surface and command README files. |
| `internal/cli/` | 48 | Command-layer implementation and tests, inferred from filenames. |
| `internal/telemetry/` | 18 | Internal telemetry implementation and tests, inferred from filenames. |
| `pkg/` | 241 | Reusable packages for SpecScore artifact parsing, linting, lifecycle, events, source refs, tasks, and related domains. |
| `spec/features/cli/` | 39 | Canonical Feature specs for CLI commands and lint behavior. |
| `spec/ideas/` | 16 | Idea backlog and archived Ideas for CLI evolution. |
| `spec/plans/` | 13 | Implementation plans for prior and in-flight CLI work. |
| `.github/workflows/` | 3 | CI, dogfood lint, and release automation. |

## Research Zones

| Zone | Roots | Files | Purpose |
|---|---|---:|---|
| `cli-entry-and-command-layer` | `cmd/`, `internal/cli/` | 52 | Retrofit command invocation, argument handling, and user-facing CLI behavior. |
| `lint-and-artifact-parsing` | `pkg/lint/`, `pkg/feature/`, `pkg/idea/`, `pkg/plan/`, `pkg/issue/`, `pkg/property/`, `pkg/entity/` | 176 | Retrofit SpecScore artifact parsers and lint rules. |
| `events-tasks-and-telemetry` | `pkg/event/`, `pkg/task/`, `internal/telemetry/`, `internal/cli/telemetry*` | 45 | Retrofit event transport, task management, and telemetry behavior. |
| `source-and-git-utilities` | `pkg/sourceref/`, `pkg/gitremote/`, `pkg/slug/`, `pkg/lifecycle/`, `pkg/idearelocate/` | 29 | Retrofit supporting repository, source reference, slug, lifecycle, and relocation behavior. |
| `dogfood-spec-tree` | `spec/features/cli/`, `spec/ideas/`, `spec/plans/` | 68 | Use existing SpecScore specs as ground truth and cross-check survey-to-retrofit flow. |
| `release-and-install-surface` | `.github/workflows/`, `scripts/`, `README.md` | 6 | Retrofit install, CI, and release process behavior from manifests and docs. |

## Detected Frameworks

| Signal | Evidence | Confidence |
|---|---|---|
| Go module | `go.mod` | high |
| Cobra CLI | `go.mod dependency github.com/spf13/cobra`, `cmd/specscore/main.go path listed` | high |
| GitHub Actions CI | `.github/workflows/dogfood.yml`, `.github/workflows/go-ci.yml`, `.github/workflows/release.yml` | high |
| SpecScore dogfood repository | `specscore.yaml`, `spec/features/ tree listed` | high |
| GoReleaser release packaging | `.goreleaser.yml filename listed`, `.github/workflows/release.yml` | medium |

## Sensitive Path Inventory

Filename-pattern hints only; not a content secret scan.

| Path | Reason |
|---|---|
| `pkg/entity/_testdata/duplicate-property-name.entity.md` | fixture-or-testdata-path |
| `pkg/entity/_testdata/email.property.md` | fixture-or-testdata-path |
| `pkg/entity/_testdata/frontmatter-missing-required-fields.entity.md` | fixture-or-testdata-path |
| `pkg/entity/_testdata/frontmatter-not-first-block.entity.md` | fixture-or-testdata-path |
| `pkg/entity/_testdata/id-mismatch-slug.entity.md` | fixture-or-testdata-path |
| `pkg/entity/_testdata/malformed-yaml.entity.md` | fixture-or-testdata-path |
| `pkg/entity/_testdata/missing-frontmatter.entity.md` | fixture-or-testdata-path |
| `pkg/entity/_testdata/title-mismatch-singular.entity.md` | fixture-or-testdata-path |
| `pkg/entity/_testdata/valid-minimal.entity.md` | fixture-or-testdata-path |
| `pkg/entity/_testdata/valid-with-inherits.entity.md` | fixture-or-testdata-path |
| `pkg/entity/_testdata/valid-with-ref-property.entity.md` | fixture-or-testdata-path |
| `pkg/lint/_testdata/property/frontmatter-missing-required-fields.property.md` | fixture-or-testdata-path |
| `pkg/lint/_testdata/property/hand-edited-referenced-by.property.md` | fixture-or-testdata-path |
| `pkg/lint/_testdata/property/id-mismatch-slug.property.md` | fixture-or-testdata-path |
| `pkg/lint/_testdata/property/inapplicable-check.property.md` | fixture-or-testdata-path |
| `pkg/lint/_testdata/property/invalid-data-type.property.md` | fixture-or-testdata-path |
| `pkg/lint/_testdata/property/missing-frontmatter.property.md` | fixture-or-testdata-path |
| `pkg/lint/_testdata/property/missing-sections.property.md` | fixture-or-testdata-path |
| `pkg/lint/_testdata/property/title-mismatch.property.md` | fixture-or-testdata-path |
| `pkg/lint/_testdata/property/unknown-check.property.md` | fixture-or-testdata-path |
| `pkg/lint/_testdata/property/valid-clean.property.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/affected-component-ref-resolves/spec/features/example/README.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/affected-component-ref-resolves/spec/issues/README.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/affected-component-ref-resolves/spec/issues/foo.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/affected-component-ref/spec/issues/README.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/affected-component-ref/spec/issues/foo.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/body-section-order/spec/issues/README.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/body-section-order/spec/issues/foo.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/bugs-non-string/spec/issues/README.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/bugs-non-string/spec/issues/foo.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/default-suite/spec/issues/README.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/default-suite/spec/issues/foo.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/dual-location/spec/random-dir/foo.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/feature-index-required/spec/features/example/README.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/feature-index-required/spec/features/example/issues/foo.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/h1-prefix/spec/issues/README.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/h1-prefix/spec/issues/foo.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/index-columns/spec/issues/README.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/index-columns/spec/issues/foo.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/invalid-severity/spec/issues/README.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/invalid-severity/spec/issues/foo.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/invalid-status/spec/issues/README.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/invalid-status/spec/issues/foo.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/missing-field/spec/issues/README.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/missing-field/spec/issues/foo.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/rejection-reason-enum/spec/issues/README.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/rejection-reason-enum/spec/issues/foo.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/root-index-required/spec/issues/foo.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/severity-required-transition/spec/issues/README.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/severity-required-transition/spec/issues/foo.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/slug-globally-unique/spec/features/example/README.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/slug-globally-unique/spec/features/example/issues/README.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/slug-globally-unique/spec/features/example/issues/foo.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/slug-globally-unique/spec/issues/README.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/slug-globally-unique/spec/issues/foo.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/slug-mismatch/spec/issues/README.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/slug-mismatch/spec/issues/foo.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/unknown-key/spec/issues/README.md` | fixture-or-testdata-path |
| `pkg/lint/rules/issue/testdata/unknown-key/spec/issues/foo.md` | fixture-or-testdata-path |
| `pkg/property/_testdata/frontmatter-not-first.property.md` | fixture-or-testdata-path |
| `pkg/property/_testdata/id-mismatch-slug.property.md` | fixture-or-testdata-path |
| `pkg/property/_testdata/invalid-data-type.property.md` | fixture-or-testdata-path |
| `pkg/property/_testdata/missing-frontmatter.property.md` | fixture-or-testdata-path |
| `pkg/property/_testdata/unknown-check-key.property.md` | fixture-or-testdata-path |
| `pkg/property/_testdata/valid-minimal.property.md` | fixture-or-testdata-path |
| `pkg/property/_testdata/valid-with-checks.property.md` | fixture-or-testdata-path |
| `pkg/property/_testdata/walktree/features/.hidden/secret.property.md` | fixture-or-testdata-path |
| `pkg/property/_testdata/walktree/features/_tests/fixture.property.md` | fixture-or-testdata-path |
| `pkg/property/_testdata/walktree/features/shared/email.property.md` | fixture-or-testdata-path |
| `pkg/property/_testdata/walktree/features/shared/money.property.md` | fixture-or-testdata-path |

## Warnings

- Sensitive path inventory is filename-pattern hints only; no content secret scan was performed.
- Source implementation files were counted by path only and were not opened.
- .goreleaser.yml was listed but not read because it is not in the v1 content-read allowlist.

## Open Questions

- Should future survey allowlist include release manifests such as `.goreleaser.yml` for content reads?
- Should fixture-heavy testdata paths be grouped separately in retrofit research zones to reduce accidental sensitive-data exposure?

---
*This document follows the https://specscore.md/research-artifact-specification*
