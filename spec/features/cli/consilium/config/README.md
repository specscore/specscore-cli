---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Consilium Config (CLI)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/consilium/config?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/consilium/config?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/consilium/config?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/consilium/config?op=request-change) |

**Status:** Approved
**Date:** 2026-06-03
**Owner:** alexander.trakhimenok
**Source Ideas:** consilium-command-group
**Supersedes:** —

## Summary

The `specscore consilium config` subcommand. In Phase 1 it exposes a single mode, `--print-gate`, which prints the active gate-knob configuration the arbiter would apply — the strict baseline merged with any `consilium.gate` overrides from `specscore.yaml` — via the `pkg/consilium` gate-knob loader (parent Feature `cli/consilium`). The consilium skill calls this to snapshot the gate config alongside the roster so a verdict is fully reconstructable.

## Problem

A verdict is only auditable if the gate knobs it was computed against are recorded. Because individual knobs fall back to the strict baseline when absent, the *effective* gate is not literally what is written in `specscore.yaml` — it is the merge of overrides over defaults. The skill needs that effective configuration as a concrete artifact, and an operator needs a way to see what gate their project actually enforces without reading the loader's merge logic by hand. Exposing the resolved gate as a read-only verb keeps the merge semantics in one place and lets config errors surface early.

## Behavior

### Command surface

#### REQ: config-print-gate-command

`specscore consilium config --print-gate` MUST read the `consilium.gate` configuration from the project's `specscore.yaml` (autodetected per the parent `cli` Feature's project resolution), resolve it against the strict baseline per the parent REQ `gate-knob-config-schema`, and print the effective gate knobs to stdout as YAML, exiting `0`. The `--print-gate` flag selects the only Phase-1 mode; running `specscore consilium config` with no mode flag MUST print help and exit per the shared exit-code contract.

#### REQ: config-output-shape

The printed YAML MUST contain every gate knob with its effective value (override where present, baseline otherwise):

```yaml
gate:
  adversary_veto_confidence: high
  cost_ceiling: medium
  complexity_ceiling: medium
  min_median_confidence: medium
  require_all_builders: true
  require_all_customers: true
```

This output MUST be acceptable verbatim as a `--gate <file>` input to `specscore consilium verdict`.

#### REQ: config-invalid-gate-surfaces

When `consilium.gate` contains an unknown knob key or an out-of-enum value, the command MUST exit non-zero (`2`) with the loader's error line on stderr naming the offending key path and accepted enum, and MUST NOT print a gate configuration to stdout.

## Acceptance Criteria

### AC: config-print-gate-default-baseline

**Requirements:** cli/consilium/config#req:config-print-gate-command, cli/consilium/config#req:config-output-shape

**Given** a `specscore.yaml` with no `consilium.gate` mapping
**When** `specscore consilium config --print-gate` runs
**Then** exit code MUST be `0`; stdout MUST be a YAML `gate:` mapping equal to the strict baseline (`adversary_veto_confidence: high`, `cost_ceiling: medium`, `complexity_ceiling: medium`, `min_median_confidence: medium`, `require_all_builders: true`, `require_all_customers: true`).

### AC: config-print-gate-merges-overrides

**Requirements:** cli/consilium/config#req:config-print-gate-command, cli/consilium/config#req:config-output-shape

**Given** a `specscore.yaml` with `consilium: { gate: { require_all_builders: false } }` and no other gate keys
**When** `specscore consilium config --print-gate` runs
**Then** exit code MUST be `0`; stdout MUST show `require_all_builders: false` with every other knob at its baseline value.

### AC: config-print-gate-feeds-verdict

**Requirements:** cli/consilium/config#req:config-output-shape

**Given** the stdout of a `specscore consilium config --print-gate` run captured to a file `gate.yaml`
**When** `specscore consilium verdict --gate gate.yaml ...` runs with otherwise valid inputs
**Then** the verdict command MUST accept `gate.yaml` without a gate-config error.

### AC: config-invalid-gate-exits-2

**Requirements:** cli/consilium/config#req:config-invalid-gate-surfaces

**Given** a `specscore.yaml` containing `consilium: { gate: { cost_ceiling: extreme } }`
**When** `specscore consilium config --print-gate` runs
**Then** exit code MUST be `2`; stderr MUST name the key path (`consilium.gate.cost_ceiling`) and the accepted enum (`low`, `medium`); stdout MUST contain no gate configuration.

## Open Questions

- **Other config views.** Phase 1 ships only `--print-gate`. A companion `--print-roster` would duplicate `consilium roster --resolve`; whether to consolidate roster and gate views under one `config` verb later is unspecified here.

---
*This document follows the https://specscore.md/feature-specification*
