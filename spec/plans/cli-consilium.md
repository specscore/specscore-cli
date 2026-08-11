---
format: https://specscore.md/plan-specification
status: Implemented
---

# Plan: Consilium Engine (CLI) — pkg/consilium

**Status:** Implemented
**Mode:** full
**Source Feature:** cli/consilium
**Date:** 2026-06-03
**Owner:** alexander.trakhimenok
**Supersedes:** —

## Summary

Implementation plan for the parent `cli/consilium` Feature — the `pkg/consilium` Go package that the three verb child Features (`verdict`, `roster`, `config`) all depend on. Lands vote-schema validation, the gate-knob and roster config loaders, default-roster resolution and validation, the custom-role markdown parser, the deterministic gate engine with abstain/veto semantics, a reproducibility snapshot harness, and the parent `consilium` cobra command. Eight tasks covering all 14 ACs in the source Feature, zero deferred.

## Approach

Bottom-up library decomposition, foundation → consumers. The package's pure data and config layers come first (Vote types, gate knobs, roster) because the gate engine composes all three: it scores votes (task 1) against group denominators from the resolved roster (tasks 3–4) using the resolved knobs (task 2). The engine therefore lands after its inputs (task 5), with abstain/veto semantics split into a second engine task (task 6) so the rule-order AC and the exclusion/veto ACs verify independently rather than as one fat task. Determinism + the snapshot harness (task 7) wraps the completed engine. The parent cobra command (task 8) only depends on the package existing, so it is ordered last and off the engine critical path. The three verb Features are out of scope here — each gets its own Plan and consumes this package; this Plan ships no `internal/cli` verb logic beyond the parent command stub. No ACs are deferred; all 14 are covered.

## Tasks

### Task 1: Scaffold `pkg/consilium` and implement the Vote schema + validator

**Status:** complete
**Depends-On:** —
**Verifies:** cli/consilium#ac:pkg-consilium-package-shape, cli/consilium#ac:vote-bundle-valid-parses, cli/consilium#ac:vote-bundle-malformed-rejected

Create the `pkg/consilium` package (singular, at the repo's `pkg/` layout). Define the `Vote` type with the five fields and parse a `--votes` file as a YAML list. Implement zero-tolerance validation: reject any vote on YAML-parse failure, missing field, out-of-enum `verdict`/`confidence`/`cost`/`complexity`, or `argument` > 280 chars, with an error naming the offending vote and rule. This task establishes the package and the first pure layer the engine consumes.

### Task 2: Gate-knob config loader — strict baseline, override merge, enum validation

**Status:** complete
**Depends-On:** 1
**Verifies:** cli/consilium#ac:gate-knobs-default-to-strict-baseline, cli/consilium#ac:gate-knob-invalid-enum-rejected

Implement the `consilium.gate` loader: parse the six knobs from `specscore.yaml`, apply the strict baseline knob-by-knob when absent, and merge a `--gate` override file over the baseline. Reject unknown knob keys and out-of-enum values with an error naming the key path and accepted enum. Reuse the existing `specscore.yaml` loader pattern (as the `events:` block did).

### Task 3: Default roster, roster config schema, and resolution

**Status:** complete
**Depends-On:** 1
**Verifies:** cli/consilium#ac:roster-resolves-defaults-minus-exclude-plus-custom

Embed the 9 default slug→group pairs (builders/customers/adversaries). Parse the `consilium.roster` block (`exclude`, `custom`) from `specscore.yaml`. Implement resolution `(defaults − exclude) ∪ custom` preserving group membership, producing the ordered roster entries (`name`, `group`, and `path` for customs) that both validation and the engine's denominators consume.

### Task 4: Custom-role markdown parser + roster validation

**Status:** complete
**Depends-On:** 3
**Verifies:** cli/consilium#ac:roster-group-floor-violation-rejected, cli/consilium#ac:custom-role-missing-field-rejected

Parse a custom-role markdown file (`**Name:**` matching filename, `**Group:**` in the three-value enum, `**Output Schema Version:** 1`, a `## Role Prompt` section, a `## Example Vote` section with a valid vote). Implement roster validation: each group ≥1, total ≤12, no case-insensitive name collision with a default, every custom path resolving to a conforming file. Reject with a single clear error naming the violation.

### Task 5: Gate engine core — ordered rule evaluation and the three terminal verdicts

**Status:** complete
**Depends-On:** 1, 2, 3
**Verifies:** cli/consilium#ac:gate-engine-applies-rules-in-order

Implement the gate algorithm steps in exact order, producing `verdict`, `rule_trace` (rule-name per fired step), `excluded_votes`, and `denominators`. This task lands the non-abstain happy path: approval counting per group against the builder/customer gates, the confidence/cost/complexity median gates, and the three terminal outcomes (`should-implement`, `should-not-implement`, `needs-human-review`). Abstain and adversary-veto branches land in task 6.

### Task 6: Abstain semantics and adversary veto

**Status:** complete
**Depends-On:** 5
**Verifies:** cli/consilium#ac:high-confidence-abstain-excluded-from-denominator, cli/consilium#ac:low-confidence-abstain-caps-verdict, cli/consilium#ac:adversary-veto-blocks

Extend the engine with the early-exit branches that precede the approval count: high-confidence abstain removes the voter from its group denominator and records it in `excluded_votes`; low-confidence (and medium-as-low) abstain caps the verdict at `needs-human-review` via `low-abstain-veto`; an adversary `should-not-implement` at or above `adversary_veto_confidence` fires `adversary-veto`. Each branch records its rule name in `rule_trace`.

### Task 7: Determinism guarantee + snapshot-test harness

**Status:** complete
**Depends-On:** 5, 6
**Verifies:** cli/consilium#ac:gate-engine-deterministic

Guarantee the engine is a pure function of its inputs — no clock, no randomness, no I/O beyond reading declared inputs — and stabilize iteration order so output is byte-identical across runs and processes. Build the fixture-based snapshot suite that CI gates on, with at least one fixture per terminal verdict plus the abstain/veto cases from task 6.

### Task 8: Parent `consilium` cobra command

**Status:** complete
**Depends-On:** 1
**Verifies:** cli/consilium#ac:consilium-parent-prints-help

Register the `consilium` parent cobra command on the root `specscore` command. It carries no run behavior of its own: a bare `specscore consilium` prints help (listing the `verdict`, `roster`, `config` subcommands once their child Features land) and exits per the shared exit-code contract. This is the attachment point the three verb Features wire into.

## Open Questions

- **Default-roster source of truth.** The embedded 9 default slug→group pairs must track the cross-repo skill's role set; whether to share a fixture or generated constant is carried from the Feature's Open Questions and resolved during task 3.

---
*This document follows the https://specscore.md/plan-specification*
