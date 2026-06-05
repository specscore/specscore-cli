---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Consilium Roster (CLI)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/consilium/roster?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/consilium/roster?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/consilium/roster?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/consilium/roster?op=request-change) |

**Status:** Approved
**Date:** 2026-06-03
**Owner:** alexander.trakhimenok
**Source Ideas:** consilium-command-group
**Supersedes:** —

## Summary

The `specscore consilium roster` subcommand. In Phase 1 it exposes a single mode, `--resolve`, which computes the active roster — defaults minus excludes plus customs — via the `pkg/consilium` resolver (parent Feature `cli/consilium`) and prints it as YAML. The consilium skill calls this to snapshot the exact roster onto the `consilium-review` task before invoking the arbiter, which is part of what makes a verdict reproducible after the fact.

## Problem

The arbiter consumes a roster snapshot, and a verdict is only auditable if the roster it was computed against is recorded. The skill must therefore obtain the resolved roster (after applying `consilium.roster.exclude` and `consilium.roster.custom`) as a concrete artifact it can store. Re-deriving roster resolution inside the skill would duplicate the parent package's logic and risk drift; exposing it as a CLI verb keeps a single source of truth and makes roster validation failures surface before any review proceeds.

## Behavior

### Command surface

#### REQ: roster-resolve-command

`specscore consilium roster --resolve` MUST read the `consilium.roster` configuration from the project's `specscore.yaml` (autodetected per the parent `cli` Feature's project resolution), invoke the `pkg/consilium` resolver and validator, and on success print the active roster to stdout as YAML, exiting `0`. The `--resolve` flag selects the only Phase-1 mode; running `specscore consilium roster` with no mode flag MUST print help and exit per the shared exit-code contract.

#### REQ: roster-output-shape

The resolved-roster YAML MUST be an ordered list of entries, each with `name` and `group`, plus `path` for custom roles:

```yaml
roster:
  - {name: engineer, group: builders}
  - {name: architect, group: builders}
  - {name: accessibility, group: customers, path: .specscore/roles/accessibility.md}
```

The list MUST reflect resolution `(defaults − exclude) ∪ custom` and MUST be the same roster the arbiter would validate and consume.

#### REQ: roster-validation-surfaces

When the resolved roster fails validation per the parent REQ `roster-validation` (empty group, > 12 roles, custom-name collision, or an unparseable custom-role file), the command MUST exit non-zero (`2`) with the validator's single clear error line on stderr and MUST NOT print a roster to stdout.

## Acceptance Criteria

### AC: roster-resolve-prints-active-roster

**Requirements:** cli/consilium/roster#req:roster-resolve-command, cli/consilium/roster#req:roster-output-shape

**Given** a `specscore.yaml` with `consilium.roster.exclude: [marketing]` and one valid custom customer role `accessibility`
**When** `specscore consilium roster --resolve` runs
**Then** exit code MUST be `0`; stdout MUST be a YAML `roster:` list of 9 entries (9 defaults − 1 excluded + 1 custom); `marketing` MUST be absent; the `accessibility` entry MUST carry `group: customers` and its `path`.

### AC: roster-resolve-default-is-nine

**Requirements:** cli/consilium/roster#req:roster-resolve-command, cli/consilium/roster#req:roster-output-shape

**Given** a `specscore.yaml` with no `consilium.roster` configuration
**When** `specscore consilium roster --resolve` runs
**Then** exit code MUST be `0`; stdout MUST list exactly the 9 default roles — `engineer`, `architect`, `qa` (builders); `pm`, `ux`, `marketing` (customers); `yagni-cop`, `skeptic`, `security-ops` (adversaries).

### AC: roster-resolve-invalid-exits-nonzero

**Requirements:** cli/consilium/roster#req:roster-validation-surfaces

**Given** a `specscore.yaml` with `consilium.roster.exclude: [yagni-cop, skeptic, security-ops]` and no custom adversary
**When** `specscore consilium roster --resolve` runs
**Then** exit code MUST be `2`; stderr MUST carry the validator error `adversaries group has 0 members; ≥1 required`; stdout MUST contain no roster.

### AC: roster-no-mode-prints-help

**Requirements:** cli/consilium/roster#req:roster-resolve-command

**Given** a built `specscore` binary
**When** `specscore consilium roster` runs with no mode flag
**Then** the command MUST print the `roster` help text and exit per the shared exit-code contract; it MUST NOT resolve or print a roster.

## Open Questions

- **Future roster modes.** Phase 1 ships only `--resolve`. Other modes (e.g. `--validate` as a standalone check, or `--list-defaults`) are plausible later but unspecified here.

---
*This document follows the https://specscore.md/feature-specification*
