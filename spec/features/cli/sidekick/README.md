---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: Sidekick (CLI)

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/sidekick?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/sidekick?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/sidekick?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/sidekick?op=request-change) |
**Status:** Approved
**Source Ideas:** —

## Summary

`specscore sidekick` commands work with sidekick-seed artifacts in `spec/ideas/seeds/` — the lint-clean, scaled-down Idea one-pagers captured by the `specstudio:sidekick` skill. The group's only verb today is `new`, which scaffolds a fresh seed from a one-liner so that seed creation flows through the CLI as the single source of canonical seed structure, the same way `idea new` and `plan new` already are for their types.

## Contents

| Child | Description |
|---|---|
| [new](new/README.md) | Scaffold a lint-clean sidekick-seed at `spec/ideas/seeds/<slug>.md` from a one-liner, deriving the slug |
| [change-status](change-status/README.md) | specscore sidekick change-status <slug> --to=<status> transitions a sidekick-seed to a terminal status (Implemented, Rejected, or Archived), relocating it to spec/ideas/archived/ and adding type: sidekick-seed. Implements the lifecycle-transitions shared contract. |

## Problem

A sidekick-seed is a scaled-down Idea: the cheapest durable artifact in the SpecScore graph, captured mid-task to park a sideline thought without derailing. Its on-disk shape is fixed by the `sidekick-seed` lint rule (the minimal frontmatter keys `captured_by` and `status: queued`, an H1 body, and a status-dependent body cap (a queued seed: 3000-char hard cap with a 2500-char advisory warning; a closed/terminal seed: 5000-char cap); captured seeds are identified by location, so `type: sidekick-seed` is added only at archive time). The `specstudio:sidekick` skill used to hand-write the seed from an embedded template, which duplicated the seed schema and drifted from the lint rule as the rule evolved. A `sidekick new` verb that scaffolds a lint-clean seed makes the CLI the single source of canonical seed structure, so the skill can shell out to it instead of carrying a template.

## Behavior

### Command group

The `sidekick` group is additive. It introduces no changes to how existing seeds are stored or discovered; its single create verb scaffolds new seeds only.

#### REQ: subcommands

`specscore sidekick` MUST expose the `new` subcommand. Invoking `specscore sidekick` with no subcommand MUST print the group help and exit `0` (not error as an unknown command).

#### REQ: mutation-scope

The `new` subcommand (see [new](new/README.md)) MAY create a new seed file but MUST NOT edit, transition, or delete existing seeds or Ideas.

### Shared flags

Every command in this group accepts the shared `--project` flag defined in the [CLI parent](../README.md): the project root, autodetected from the current directory when omitted.

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [CLI](../README.md) | Inherits the shared exit-code contract and project autodetection. |
| [cli/idea/new](../idea/new/README.md) | Sibling create verb whose slug/no-clobber/ancestor-index/lint-clean contract this group mirrors. The seed is a scaled-down Idea, so `sidekick new` is the lightweight twin of `idea new`. |

## Acceptance Criteria

### AC: group-exposes-subcommands (verifies REQ:subcommands)

**Given** a project with a `spec/ideas/seeds/` directory
**When** the user runs `specscore sidekick`
**Then** the group help is printed listing `new`, and the command exits `0`.

### AC: group-is-read-only-except-new (verifies REQ:mutation-scope)

**Given** a project with an existing seed
**When** the user runs any `specscore sidekick` verb other than `new`
**Then** no existing seed or Idea file is edited, transitioned, or deleted.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
