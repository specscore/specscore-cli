# Feature: Consilium (CLI)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/consilium?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/consilium?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/consilium?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/consilium?op=request-change) |

**Status:** Approved
**Date:** 2026-06-03
**Owner:** alexander.trakhimenok
**Source Ideas:** consilium-command-group
**Supersedes:** —

## Summary

The deterministic consilium engine for the `specscore` CLI. Owns the `pkg/consilium` Go package — the vote-schema types and validator, the resolver and validator for the active roster, the custom-role markdown parser, the gate-knob and roster config loaders for the `consilium:` block in `specscore.yaml`, and the gate-rule arbiter that turns a panel's votes into a deterministic verdict. Also owns the parent `consilium` cobra command. The three user-facing verbs (`verdict`, `roster`, `config`) are owned by child Features and are thin wrappers over this package.

## Contents

| Child | Description |
|---|---|
| [verdict](verdict/README.md) | The `specscore consilium verdict` subcommand: the load-bearing arbiter verb that reads votes, roster, and gate config and emits a deterministic verdict. |
| [roster](roster/README.md) | The `specscore consilium roster` subcommand: resolves and prints the active roster (defaults + custom − exclude) for the consilium skill to snapshot. |
| [config](config/README.md) | The `specscore consilium config` subcommand: prints the active gate-knob configuration the arbiter would apply. |

## Problem

The cross-repo `specstudio:consilium` skill drains queued sidekick ideas through a 5-stage pipeline whose Stage 4 is a **deterministic CLI arbiter**: the LLM expert panel supplies typed votes, but the decision that turns those votes into `should-implement` / `should-not-implement` / `needs-human-review` must be computed by code so it is reproducible, auditable, and unit-testable. The governing principle is "LLMs are bad at gating": letting model judgment leak into the verdict makes it non-reproducible and unfit to gate downstream auto-promotion.

Today `specscore` ships no `consilium` command at all, so the skill's pre-flight fails and the sidekick queue cannot be drained. The skill invokes three subcommands — `consilium verdict`, `consilium roster --resolve`, `consilium config --print-gate` — all of which depend on the same underlying logic: a vote validator, a roster resolver/validator, a gate-knob config loader, and the gate-rule engine. This Feature owns that shared logic as a single Go package so the verbs are thin and the gate rules have one canonical, tested home.

The behavioral contract this Feature implements is defined cross-repo in `specstudio-skills` (`spec/features/sidekick-consilium/README.md`, REQs `specscore-consilium-verdict-subcommand`, `arbiter-gate-rules`, `arbiter-reproducibility`, `roster-validation`, `vote-schema`, `gate-knob-set`, `roster-exclude-and-custom`, `custom-role-markdown-contract`, `default-roster-9-roles`, `abstain-with-confidence`). Cross-repo Source-Idea links are not yet lint-supported, so that contract is cited here in prose; the local front-door is the `consilium-command-group` Idea.

## Behavior

### The `pkg/consilium` package

#### REQ: pkg-consilium-package-location

The engine MUST live at `pkg/consilium/` in the repo, importable as `github.com/specscore/specscore-cli/pkg/consilium`. Per the `pkg/` layout used by `pkg/event`, `pkg/feature`, `pkg/idea`, `pkg/lifecycle`, `pkg/plan`, `pkg/lint`, `pkg/task`, the package name is the singular `consilium`. The verb cobra wiring (defined by the child Features) lives in `internal/cli`; gate logic, vote/roster validation, and config loading MUST NOT live in `internal/cli` or in sibling packages.

### Vote schema and validation

#### REQ: vote-schema-types-and-validation

The package MUST define a `Vote` type with exactly five fields and parse a `--votes` file as a YAML list of votes:

```yaml
verdict: should-implement | should-not-implement | no-opinion | abstain
confidence: low | medium | high
cost: 🟢 | 🟡 | 🔴
complexity: 🟢 | 🟡 | 🔴
argument: <one-sentence strongest argument, ≤ 280 characters>
```

Validation MUST reject a votes bundle that contains any vote which fails YAML parse, omits a required field, carries an out-of-enum value for `verdict` / `confidence` / `cost` / `complexity`, or whose `argument` exceeds 280 characters. Tolerance for malformed votes is **zero**: a single malformed vote fails the entire bundle with a clear error naming the offending vote (by index and/or role) and the violated rule. A malformed bundle MUST NOT yield a partial verdict.

### The deterministic gate engine

#### REQ: gate-engine-rule-order

The engine MUST apply the gate rules in exactly this order, using the resolved gate-knob values (REQ `gate-knob-config-schema`):

```
1.  Validate inputs (vote schema, roster validation).
2.  Exclude high-confidence abstain votes from the consensus denominator.
3.  If any vote is a low-confidence abstain:
      → verdict = needs-human-review (rule: low-abstain-veto) → STOP
4.  If any adversary returns should-not-implement
      AND confidence ≥ adversary_veto_confidence:
        → verdict = needs-human-review (rule: adversary-veto) → STOP
5.  Count non-abstain approvals per group (builders, customers).
6.  Builder gate: require_all_builders=true → all approve; else ⅔ supermajority (ceil).
7.  Customer gate: same logic via require_all_customers.
8.  Confidence gate: median confidence across non-abstain votes ≥ min_median_confidence.
9.  Cost gate: median cost across non-abstain votes ≤ cost_ceiling.
10. Complexity gate: median complexity ≤ complexity_ceiling.
11. If all of (builder, customer, confidence, cost, complexity) gates pass → verdict = should-implement.
12. Else if builders AND customers both vote majority should-not-implement → verdict = should-not-implement.
13. Otherwise → verdict = needs-human-review.
```

The engine MUST produce a result containing: the `verdict` enum (`should-implement | should-not-implement | needs-human-review`); a `rule_trace` — the ordered list of rule-name strings for the steps that fired, where each name reflects exactly which step produced the outcome; `excluded_votes` — the role slugs excluded in step 2; and `denominators` — `{builders, customers, adversaries}` after exclusion.

#### REQ: abstain-semantics

The `abstain` verdict MUST be interpreted by confidence:

- **High-confidence abstain** ("not my domain") — the voter is removed from its group's denominator and listed in `excluded_votes`; it counts toward neither approval nor rejection.
- **Low-confidence abstain** ("I can't tell if this matters") — caps the verdict at `needs-human-review` via the `low-abstain-veto` rule (step 3); the strict-gate path is not evaluated.
- **Medium-confidence abstain** — treated as low-confidence (the cautious default).

#### REQ: gate-engine-determinism

Gate evaluation MUST be a pure function of its inputs (votes, roster snapshot, gate knobs). It MUST NOT read the clock, consult any randomness source, or perform I/O beyond reading the declared input files. The same `(votes, roster, gate)` inputs MUST always produce byte-identical `(verdict, rule_trace, excluded_votes, denominators)` output. The engine MUST be exercised by a fixture-based snapshot test suite in this repo that CI gates on.

### Configuration (`specscore.yaml` → `consilium:`)

#### REQ: gate-knob-config-schema

The CLI MUST accept an optional `consilium.gate` mapping in `specscore.yaml` with exactly these knobs, each a discrete enum (continuous numeric thresholds are NOT supported):

```yaml
consilium:
  gate:
    adversary_veto_confidence: high     # high | medium | low
    cost_ceiling: medium                # low | medium
    complexity_ceiling: medium          # low | medium
    min_median_confidence: medium       # low | medium | high
    require_all_builders: true          # true | false
    require_all_customers: true         # true | false
```

The values shown are the **strict baseline default**, applied knob-by-knob when `consilium.gate` (or any individual knob) is absent. A `--gate <file>` supplied to the arbiter overrides the baseline. An unknown knob key, or an out-of-enum value, MUST cause a config-load error naming the offending key path (e.g. `consilium.gate.cost_ceiling`) and the accepted enum; no verdict is produced.

#### REQ: roster-config-schema

The CLI MUST accept an optional `consilium.roster` mapping in `specscore.yaml` with two optional sub-keys:

```yaml
consilium:
  roster:
    exclude: [marketing, security-ops]          # default role slugs to drop
    custom:                                      # custom roles to add
      - name: accessibility
        group: customers                         # builders | customers | adversaries
        path: .specscore/roles/accessibility.md  # resolved relative to repo root
```

Both sub-keys are optional; the absence of either means "use defaults". An unknown sub-key, a `custom` entry missing `name`/`group`/`path`, or a `group` outside the three-value enum MUST cause a config-load error naming the offending key path.

### Roster resolution and validation

#### REQ: default-roster-9-roles

The package MUST embed the default roster as exactly 9 role slugs in three groups: builders (`engineer`, `architect`, `qa`); customers (`pm`, `ux`, `marketing`); adversaries (`yagni-cop`, `skeptic`, `security-ops`). The CLI owns only the slug→group mapping used for resolution and validation; the role **prompt** files ship with the cross-repo consilium skill and are out of scope here.

#### REQ: roster-resolution

The package MUST resolve the active roster as `(defaults − exclude) ∪ custom`, preserving group membership. The resolved roster is an ordered list of entries, each carrying `name` and `group` (and `path` for custom roles). Resolution is the input both to roster validation and to the gate engine's denominators.

#### REQ: roster-validation

Before any verdict is computed, the package MUST validate the resolved roster and reject an invalid one:

- After exclude and add, each of the three groups (builders / customers / adversaries) MUST contain ≥ 1 role.
- Total active roster size MUST be ≤ 12 roles.
- No custom role name MUST collide with any default role name (case-insensitive).
- Every custom-role `path` MUST resolve to an existing markdown file that parses per REQ `custom-role-markdown-contract`.

A validation failure MUST produce a single clear error message naming the specific violation (e.g. `adversaries group has 0 members; ≥1 required`); the caller exits non-zero and computes no verdict.

#### REQ: custom-role-markdown-contract

A custom-role markdown file MUST contain: body-metadata lines `**Name:** <kebab-case-slug>` (matching the filename without `.md`), `**Group:** builders | customers | adversaries`, and `**Output Schema Version:** 1`; a `## Role Prompt` H2 section; and a `## Example Vote` H2 section containing one valid vote per REQ `vote-schema-types-and-validation`. The package MUST validate these at roster-load time and reject a non-conforming file with an error naming the missing/invalid field and the file path.

### The parent command

#### REQ: consilium-parent-command

A `consilium` parent cobra command MUST attach to the root `specscore` command. It carries no run behavior of its own: `specscore consilium` with no subcommand MUST print help and exit per the parent `cli` Feature's shared exit-code contract. The `verdict`, `roster`, and `config` subcommands attach under it (wiring owned by the child Features).

## Acceptance Criteria

### AC: pkg-consilium-package-shape

**Requirements:** cli/consilium#req:pkg-consilium-package-location

**Given** a fresh checkout after this Feature is implemented
**When** `go list ./pkg/consilium` and `go vet ./pkg/consilium/...` run from the repo root
**Then** both MUST exit `0`; the package path MUST be exactly `github.com/specscore/specscore-cli/pkg/consilium` (singular `consilium`); no gate-evaluation, vote-validation, roster-resolution, or config-loading symbol MUST be exported from `internal/cli` or any sibling package.

### AC: vote-bundle-valid-parses

**Requirements:** cli/consilium#req:vote-schema-types-and-validation

**Given** a YAML votes file whose entries each carry a valid `verdict`, `confidence`, `cost`, `complexity`, and an `argument` ≤ 280 characters
**When** the package parses and validates the bundle
**Then** validation MUST succeed and yield one `Vote` value per entry with fields populated verbatim.

### AC: vote-bundle-malformed-rejected

**Requirements:** cli/consilium#req:vote-schema-types-and-validation

**Given** a YAML votes file where exactly one vote has `verdict: maybe` (out of enum) and another has an `argument` of 281 characters
**When** the package validates the bundle
**Then** validation MUST fail with a non-nil error naming the offending vote and the violated rule (out-of-enum `verdict`; over-length `argument`); no partial verdict MUST be produced.

### AC: gate-engine-applies-rules-in-order

**Requirements:** cli/consilium#req:gate-engine-rule-order

**Given** a 9-role default roster snapshot, the strict-baseline gate knobs, and a vote bundle where all builders and customers vote `should-implement` at `medium`+ confidence with `cost`/`complexity` at or under the ceilings and no abstains or adversary vetoes
**When** the engine evaluates the gate
**Then** the result MUST be `verdict: should-implement`; `rule_trace` MUST list the gates that fired ending in the should-implement rule (step 11); `excluded_votes` MUST be empty; `denominators` MUST be `{builders: 3, customers: 3, adversaries: 3}`.

### AC: high-confidence-abstain-excluded-from-denominator

**Requirements:** cli/consilium#req:abstain-semantics, cli/consilium#req:gate-engine-rule-order

**Given** a panel where one customer votes `abstain` at `high` confidence and the other two customers vote `should-implement`, with `require_all_customers: true`
**When** the engine evaluates the gate
**Then** `denominators.customers` MUST be `2`; the customer gate MUST pass (both remaining customers approve); `excluded_votes` MUST contain the abstaining role's slug.

### AC: low-confidence-abstain-caps-verdict

**Requirements:** cli/consilium#req:abstain-semantics, cli/consilium#req:gate-engine-rule-order

**Given** a panel where one role votes `abstain` at `low` confidence and every other vote is `should-implement` at `high` confidence
**When** the engine evaluates the gate
**Then** the `verdict` MUST be `needs-human-review`; `rule_trace` MUST contain `low-abstain-veto`; the strict-gate path (steps 5–11) MUST NOT be evaluated.

### AC: adversary-veto-blocks

**Requirements:** cli/consilium#req:gate-engine-rule-order

**Given** strict-baseline knobs (`adversary_veto_confidence: high`) and a panel where one adversary votes `should-not-implement` at `high` confidence while all builders and customers approve
**When** the engine evaluates the gate
**Then** the `verdict` MUST be `needs-human-review`; `rule_trace` MUST contain `adversary-veto`; evaluation MUST STOP before the approval-count steps.

### AC: gate-engine-deterministic

**Requirements:** cli/consilium#req:gate-engine-determinism

**Given** a fixed `(votes, roster, gate)` input triple
**When** the engine evaluates the gate twice in the same process and once in a fresh process
**Then** all three runs MUST produce byte-identical `verdict`, `rule_trace`, `excluded_votes`, and `denominators`; no output MUST vary with wall-clock time or run order.

### AC: gate-knobs-default-to-strict-baseline

**Requirements:** cli/consilium#req:gate-knob-config-schema

**Given** a `specscore.yaml` with no `consilium.gate` mapping
**When** the gate-knob loader runs
**Then** the resolved knobs MUST equal the strict baseline (`adversary_veto_confidence: high`, `cost_ceiling: medium`, `complexity_ceiling: medium`, `min_median_confidence: medium`, `require_all_builders: true`, `require_all_customers: true`).

### AC: gate-knob-invalid-enum-rejected

**Requirements:** cli/consilium#req:gate-knob-config-schema

**Given** a `specscore.yaml` containing `consilium: { gate: { cost_ceiling: extreme } }`
**When** the gate-knob loader runs
**Then** loading MUST fail with an error naming the key path (`consilium.gate.cost_ceiling`), the offending value (`extreme`), and the accepted enum (`low`, `medium`); no verdict MUST be computed.

### AC: roster-resolves-defaults-minus-exclude-plus-custom

**Requirements:** cli/consilium#req:roster-resolution, cli/consilium#req:roster-config-schema, cli/consilium#req:default-roster-9-roles

**Given** a `specscore.yaml` with `consilium.roster.exclude: [marketing]` and one valid custom customer role `accessibility`
**When** the package resolves the active roster
**Then** the resolved roster MUST contain 9 roles (9 defaults − 1 excluded + 1 custom); `marketing` MUST be absent; `accessibility` MUST be present in the `customers` group with its `path`.

### AC: roster-group-floor-violation-rejected

**Requirements:** cli/consilium#req:roster-validation

**Given** a `specscore.yaml` with `consilium.roster.exclude: [yagni-cop, skeptic, security-ops]` and no custom adversary added
**When** the package validates the resolved roster
**Then** validation MUST fail with the error `adversaries group has 0 members; ≥1 required`; no verdict MUST be computed.

### AC: custom-role-missing-field-rejected

**Requirements:** cli/consilium#req:custom-role-markdown-contract, cli/consilium#req:roster-validation

**Given** a custom-role markdown file referenced by `consilium.roster.custom` that omits the `**Group:**` metadata line
**When** the package loads the roster
**Then** validation MUST fail with an error naming the missing field (`Group`) and the file path; no verdict MUST be computed.

### AC: consilium-parent-prints-help

**Requirements:** cli/consilium#req:consilium-parent-command

**Given** a built `specscore` binary
**When** `specscore consilium` is run with no subcommand
**Then** the command MUST print the `consilium` help text listing the `verdict`, `roster`, and `config` subcommands, and exit per the shared exit-code contract.

## Open Questions

- **Default-roster source of truth.** The CLI embeds the 9 default slug→group pairs for resolution/validation while the role prompt files live in the skills repo. If the skills repo changes the default roster, the CLI's embedded list must be updated in lockstep. Worth a shared fixture or a generated constant? Resolve at plan time.
- **`consilium.auto_promote` namespace.** Phase 2 will additively add this block to `specscore.yaml`. Whether to pre-reserve the key (so lint does not warn on it early) or wait for Phase 2 is deferred, matching the source contract's tentative "wait".

---
*This document follows the https://specscore.md/feature-specification*
