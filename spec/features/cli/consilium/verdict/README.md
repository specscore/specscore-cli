# Feature: Consilium Verdict (CLI)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/consilium/verdict?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/consilium/verdict?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/consilium/verdict?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/consilium/verdict?op=request-change) |

**Status:** Approved
**Date:** 2026-06-03
**Owner:** alexander.trakhimenok
**Source Ideas:** consilium-command-group
**Supersedes:** —

## Summary

The `specscore consilium verdict` subcommand — the load-bearing arbiter verb. It reads a panel's votes, the active roster snapshot, and an optional gate-knob override file, delegates to the `pkg/consilium` gate engine (parent Feature `cli/consilium`), and emits the deterministic verdict as machine-readable YAML on stdout. This verb is Stage 4 of the cross-repo `specstudio:consilium` pipeline; it is a thin wrapper that carries no gate logic of its own.

## Problem

The consilium skill's Stage 4 must turn the panel's typed votes into a verdict by code, not by a model. It needs a CLI entry point with a stable I/O contract: file inputs in, machine-readable verdict out, exit code that distinguishes "verdict computed" (always success, including a rejection) from "input was invalid". Without this verb the pipeline cannot complete and the sidekick queue cannot be drained. The gate algorithm, vote validation, and roster validation already live in `pkg/consilium`; this Feature only wires them to a cobra command and fixes the CLI surface.

## Behavior

### Command surface

#### REQ: verdict-command-io

`specscore consilium verdict` MUST accept exactly these flags:

- `--votes <file>` (required) — YAML file containing the panel's votes.
- `--roster <file>` (required) — YAML file containing the active roster snapshot (role slugs + groups).
- `--gate <file>` (optional) — YAML file of gate-knob overrides; when omitted, the strict baseline of parent REQ `gate-knob-config-schema` applies.
- `--seed <file>` (required) — path to the seed file (carried for content-hash extraction and audit).

On a successfully computed verdict the command MUST write to stdout exactly this YAML shape and nothing else:

```yaml
verdict: should-implement | should-not-implement | needs-human-review
rule_trace: [<rule-name strings that fired, in order>]
excluded_votes: [<role slugs excluded for high-confidence abstain>]
denominators: {builders: <int>, customers: <int>, adversaries: <int>}
```

#### REQ: verdict-delegates-to-engine

The verb MUST be a thin wrapper: parse flags, read the input files, invoke `pkg/consilium` (vote-schema validation → roster validation → gate evaluation), and serialize the engine's result. The verb MUST NOT implement, duplicate, or re-order any gate rule; the parent package is the sole owner of the decision logic.

#### REQ: verdict-exit-codes

The command MUST map outcomes to exit codes consistent with the parent `cli` Feature's shared exit-code contract:

| Code | Condition |
|---|---|
| `0` | A verdict was computed — including `should-not-implement` and `needs-human-review`. These are valid verdicts, not errors. |
| `2` | Invalid input: a malformed vote (REQ `vote-schema-types-and-validation`), an invalid roster (REQ `roster-validation`), or a malformed gate file (REQ `gate-knob-config-schema`). |
| `3` | A required input file (`--votes`, `--roster`, or `--seed`) was not found. |

On any non-zero exit the command MUST write a single clear error line to stderr naming the violation, and MUST NOT write a verdict to stdout.

#### REQ: verdict-reproducibility

For the same `--votes`, `--roster`, and `--gate` inputs, the command MUST produce byte-identical stdout YAML and the same exit code across invocations. The verb MUST be covered by a fixture-based snapshot suite in this repo — a set of `(votes, roster, gate)` triples mapping to fixed stdout — that CI gates on, with at least one fixture per terminal verdict.

## Acceptance Criteria

### AC: verdict-emits-yaml-on-success

**Requirements:** cli/consilium/verdict#req:verdict-command-io

**Given** fixture files `votes.yaml`, `roster.yaml`, `gate.yaml`, `seed.md` that the gate rules deterministically resolve to `should-implement`
**When** `specscore consilium verdict --votes votes.yaml --roster roster.yaml --gate gate.yaml --seed seed.md` runs
**Then** exit code MUST be `0`; stdout MUST be YAML with `verdict: should-implement`, a non-empty `rule_trace` list, an `excluded_votes` list, and a `denominators` mapping with integer `builders`/`customers`/`adversaries`; no other content MUST appear on stdout.

### AC: verdict-rejection-is-exit-zero

**Requirements:** cli/consilium/verdict#req:verdict-exit-codes, cli/consilium/verdict#req:verdict-command-io

**Given** fixture inputs that the gate rules resolve to `should-not-implement`, and a separate set that resolve to `needs-human-review`
**When** `specscore consilium verdict` runs against each
**Then** both invocations MUST exit `0`; stdout MUST carry the corresponding `verdict` value; neither is treated as an error.

### AC: verdict-malformed-vote-exits-2

**Requirements:** cli/consilium/verdict#req:verdict-exit-codes, cli/consilium/verdict#req:verdict-delegates-to-engine

**Given** a `--votes` file containing a vote with `verdict: maybe`
**When** `specscore consilium verdict` runs with otherwise valid `--roster` and `--seed`
**Then** exit code MUST be `2`; stderr MUST name the malformed vote and the violated rule; stdout MUST contain no verdict YAML.

### AC: verdict-missing-file-exits-3

**Requirements:** cli/consilium/verdict#req:verdict-exit-codes

**Given** a `--votes` path that does not exist
**When** `specscore consilium verdict` runs
**Then** exit code MUST be `3`; stderr MUST name the missing file; no verdict MUST be written.

### AC: verdict-snapshot-reproducible

**Requirements:** cli/consilium/verdict#req:verdict-reproducibility

**Given** a fixture `(votes, roster, gate, seed)` set
**When** `specscore consilium verdict` is invoked twice with identical arguments
**Then** both invocations MUST produce byte-identical stdout YAML (verdict, rule_trace, excluded_votes, denominators) and exit code `0`.

## Open Questions

- **Votes on stdin.** Whether `verdict` should also accept the votes bundle on stdin (in addition to `--votes <file>`), mirroring the input-mode question raised for `event emit`, is deferred until the skill's invocation ergonomics are settled.

---
*This document follows the https://specscore.md/feature-specification*
