# Idea: Consilium Command Group (Deterministic Arbiter + Roster + Gate Config)

**Status:** Implementing
**Date:** 2026-06-03
**Owner:** alexander.trakhimenok
**Promotes To:** cli/consilium, cli/consilium/config, cli/consilium/roster, cli/consilium/verdict
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we give the consilium skill a deterministic, reproducible, auditable arbiter — plus the roster and gate-config surfaces it depends on — so an LLM expert panel's votes are turned into a verdict by code, not by a model?

## Context

The `specstudio:consilium` skill drains queued sidekick ideas through a 5-stage pipeline: CLI gather → researcher → a parallel LLM expert panel (~9 roles) → **CLI arbiter** → scribe. Stages 3 and 5 are LLM calls that produce *judgment* (structured votes and a prose summary). Stages 1 and 4 are meant to be deterministic CLI calls. The skill already invokes three subcommands under a `consilium` command group that do not yet exist in `specscore-cli`:

- `specscore consilium verdict --votes … --roster … --gate … --seed …` — applies the gate rules to the votes and returns the verdict.
- `specscore consilium roster --resolve` — resolves the active roster (9-role default, or `specscore.yaml` overrides: defaults + customs − excludes).
- `specscore consilium config --print-gate` — prints the active gate-knob configuration.

Today `specscore` (v0.5.0) has no `consilium` command at all, so the skill's pre-flight fails and the queue cannot be drained. The governing principle behind splitting this out of the skill is **"LLMs are bad at gating"**: the panel should supply judgment, but the decision must be computed by code so it is deterministic, reproducible, and auditable. The source contract lives cross-repo in `specstudio-skills` (`spec/features/sidekick-consilium/README.md`, REQs `specscore-consilium-verdict-subcommand`, `arbiter-gate-rules`, `arbiter-reproducibility`, `roster-validation`, `vote-schema`, `gate-knob-set`) and is tracked here as upstream issue specscore-cli#8 and the skills-repo stub `spec/plans/sidekick-consilium-arbiter-companion.md`. (Note: cross-repo Source-Idea links are not yet lint-supported, so the contract is cited here as prose.)

## Recommended Direction

Ship a `consilium` command group in `specscore-cli` covering the three subcommands the skill depends on, with `verdict` as the load-bearing core. `verdict` accepts `--votes`, `--roster`, `--gate` (optional, defaults to a strict baseline), and `--seed`; validates the roster (≥1 role per group post-exclude/add, ≤12 total, no name collisions, custom-role files parse) and each vote against the vote schema (5 required fields, valid enums, argument ≤280 chars), rejecting malformed input with a clear non-zero exit; applies the ordered gate algorithm (exclude high-confidence abstains from the denominator, low-confidence-abstain veto → needs-human-review, adversary veto above a confidence knob → needs-human-review, then builder/customer approval thresholds); and emits YAML to stdout with `verdict`, `rule_trace`, `excluded_votes`, and `denominators`. Exit code is `0` for any successfully computed verdict — including `should-not-implement` and `needs-human-review` — and non-zero only on validation failure.

The decision logic must be **purely deterministic and snapshot-testable**: identical inputs always produce byte-identical stdout, with no model call inside the CLI. `roster --resolve` and `config --print-gate` are the supporting surfaces that let the skill snapshot the exact roster and gate knobs it ran against, which both feeds the arbiter and makes a run reconstructable after the fact. Gate knobs (e.g. `adversary_veto_confidence`, `require_all_builders`) come from a `consilium:` block in `specscore.yaml` with a strict default when absent, mirroring the established pattern from the `events:` block.

## Alternatives Considered

**LLM-as-arbiter (let an agent read the votes and decide).** Smallest change — no CLI work, the skill just adds one more agent call. Lost because it violates the core principle: a model's verdict is non-deterministic and non-auditable, the scribe could silently override the gate, and "why was this rejected?" would have no reproducible `rule_trace`. The whole point is to move gating out of the model.

**Keep the gate logic inside the skill (bash/inline).** Avoids a cross-repo dependency. Lost because the algorithm is a 13-step ordered rule set with configurable knobs that must be identical across every skill invocation and snapshot-testable in CI; encoding that in skill bash is unmaintainable, untestable, and would drift per-skill. The CLI is the right home for deterministic, versioned, tested logic.

**Ship `verdict` only; defer `roster` and `config`.** Narrowest unblock matching the arbiter companion stub. Lost (per scope decision) because the skill's Stage 3 and Stage 4 already call `roster --resolve` and `config --print-gate`; without them the pipeline still cannot run end-to-end, and roster/gate snapshots are part of what makes a verdict reproducible. Scoping the whole group keeps the surfaces coherent.

## MVP Scope

Make a real consilium pipeline run drain a queued seed end-to-end without an LLM touching the verdict. Concretely: `specscore consilium verdict` turns a fixture vote bundle + resolved roster + gate config into a correct verdict with a `rule_trace`, and `roster --resolve` / `config --print-gate` produce the snapshots the skill feeds in. Proof point: a golden-fixture snapshot suite where each `(votes, roster, gate)` triple maps to a fixed stdout, covering at least one case per terminal verdict (`should-implement`, `should-not-implement`, `needs-human-review`) and the key veto rules. Done when the skills-repo verify step `specscore consilium verdict --votes votes-unanimous-strong.yaml … ` prints `verdict: should-implement`.

## Not Doing (and Why)

- The LLM expert-panel roles themselves (researcher, scribe, 9 voters) — those live in the specstudio-skills consilium skill, not the CLI
- Orchestrator task lifecycle and the consilium-review task type — a separate cross-repo dependency tracked in the skills repo's task companion plan
- Auto-promotion of should-implement verdicts into Features — explicitly Phase 2+ scope

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | The gate algorithm in the cross-repo source contract (`arbiter-gate-rules`, 13 ordered steps) is complete and stable enough to implement against without further skill-side changes. | Read all 13 steps + `gate-knob-set` + `vote-schema` + `roster-validation` from `specstudio-skills/spec/features/sidekick-consilium/README.md` at specify time and diff against this summary. |
| Must-be-true | The arbiter can be made fully deterministic and snapshot-testable — no input requires a model call or non-reproducible ordering. | Build the golden-fixture snapshot suite; run twice in CI and assert byte-identical stdout. |
| Should-be-true | The `consilium:` config block can reuse the existing `specscore.yaml` loader pattern (as `events:` did) without new infrastructure. | Prototype `config --print-gate` reading a `consilium.gate` block with strict defaults-when-absent; confirm loader reuse. |
| Should-be-true | `roster --resolve` (defaults + customs − excludes, with custom-role file parsing) belongs in the CLI rather than the skill. | Confirm the skill currently shells to `specscore consilium roster --resolve` and consumes its YAML directly. |
| Might-be-true | The whole group can ship in one minor release without blocking on the separate orchestrator / `consilium-review` task-type dependency. | Confirm `verdict`/`roster`/`config` have no runtime dependency on the orchestrator CLI (they operate on files + stdin), only the skill's task lifecycle does. |


## SpecScore Integration

- **New Features this would create:** `cli/consilium` (group) with `cli/consilium/verdict`, `cli/consilium/roster`, `cli/consilium/config` — exact split decided at specify time.
- **Existing Features affected:** none directly; the `consilium:` config block extends the `specscore.yaml` loader alongside the existing `events:` block.
- **Dependencies:** cross-repo source contract in `specstudio-skills` (`sidekick-consilium` Feature; arbiter companion plan stub; upstream issue specscore-cli#8). Independent of the separate orchestrator / `consilium-review` task-type dependency.

## Open Questions

- Should `verdict` accept votes on stdin in addition to `--votes <file>`, matching the input-mode question raised for `event emit`?
- Feature split: one `cli/consilium` Feature with three subcommand sections, or a parent + three child Features?
- Where do gate-knob defaults live canonically — hardcoded strict baseline in the CLI, or a checked-in default `gate.yaml` fixture the CLI ships?

---
*This document follows the https://specscore.md/idea-specification*
