---
format: https://specscore.md/plans-index-specification
---

# SpecScore CLI Plans

Canonical index of all plans in this repository. Each plan lives in its own directory under `spec/plans/` and is governed by the [plan Feature](https://specscore.md/plan-specification) in the SpecScore meta-spec.

## Contents

| Plan | Status | Features | Effort | Impact | Author | Approved |
|---|---|---|---|---|---|---|
| [lifecycle-verbs-implementation](lifecycle-verbs-implementation/README.md) | approved | lifecycle-transitions, idea/change-status, feature/change-status | M | high | alexander.trakhimenok | 2026-05-18 |
| [canonical-grade-metadata-field-cli](canonical-grade-metadata-field-cli/README.md) | draft | cli/spec/lint | S | medium | alexander.trakhimenok | — |
| [cli-self-update](cli-self-update.md) | draft | cli/self-update | M | medium | alex | — |

### cli-self-update

Implements the [cli/self-update](../features/cli/self-update/README.md) Feature: the `specscore self-update` command (alias `update`) with install-method detection, package-manager redirect, latest-stable release resolution, checksum-verified atomic self-replace for manual installs, and a read-only `--check` mode. Eleven tasks covering all 15 ACs, zero deferred.

### canonical-grade-metadata-field-cli

Implements the CLI half of the upstream meta-spec
[canonical-grade-metadata-field](https://github.com/specscore/specscore/blob/main/spec/features/canonical-grade-metadata-field/README.md)
Feature: `grade.values` config parsing, header-block parsing of the `**Grade:**`
line (with `--fix` placement normalization), value validation in
`specscore spec lint`, and kind-agnostic / Status-decoupled rule application.
Extends [cli/spec/lint](../features/cli/spec/lint/README.md). Supersedes the
closed trackers specscore-cli#20–23.

### lifecycle-verbs-implementation

Implements the two `change-status` CLI verbs (one per doc kind), the shared `pkg/lifecycle/` package, and the new `feature-index-row-sync` lint rule, parallelizing the work across subagents. Realizes the [lifecycle-verbs-for-idea-and-feature Idea](../ideas/lifecycle-verbs-for-idea-and-feature.md) and the three feature specifications under `spec/features/cli/`.

## Recently Closed

None at this time.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plans-index-specification*
