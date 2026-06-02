# Feature: CLI Publication Policy

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/publication-policy?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/publication-policy?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/publication-policy?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/publication-policy?op=request-change) |

**Status:** Draft
**Date:** 2026-06-01
**Owner:** alex
**Source Ideas:** —
**Supersedes:** —

## Summary

Adds `specscore` CLI commands and helpers to mutate publication policy config, resolve effective policy, validate branch guards, and support manifest-based publication operations. The CLI owns deterministic config writes and machine-readable resolution output; caller skills own user intent, approval, and deciding which paths belong in a publication manifest.

## Problem

SpecStudio skills need a shared way to save and read publication preferences without hand-editing YAML. They also need deterministic helpers for branch policy checks and manifest-safe git operations. If each skill parses and mutates config independently, user defaults, project defaults, and branch safety will drift across commands.

The CLI should not infer user intent from arbitrary git state, though. It can operate safely when given explicit inputs: scope, event or command, action list, branch, and path manifest. The Feature defines that boundary so the CLI is useful without becoming an accidental publisher of unrelated work.

## Behavior

### Command Surface

#### REQ: publication-command-group

The CLI MUST provide a `specscore publication` command group for publication policy helpers. The group MUST include, at minimum, commands to set policy, resolve policy, validate branch push eligibility, and inspect current config.

#### REQ: set-policy-command

`specscore publication set` MUST mutate durable user or project config. It MUST accept scope (`user` or `project`), target kind (`default`, `event`, `command`, or command-scoped event/milestone), target name when applicable, and actions. The command MUST normalize user-facing shorthand into canonical `actions: [...]` config before writing.

The command MUST preserve unrelated config fields and MUST use the `specscore` repo-config writer so formatting, schema header, and unknown-field preservation follow the canonical config contract.

#### REQ: resolve-policy-command

`specscore publication resolve` MUST output the effective policy for a supplied context. Inputs MUST include enough fields to resolve specificity: project path, optional command, optional event, optional milestone, optional task policy, optional session policy, and current branch. Output MUST be machine-readable in YAML and JSON.

#### REQ: branch-check-command

`specscore publication branch-check` MUST evaluate whether `push` is allowed for a branch under the effective branch safety rules. It MUST refuse detached HEAD, missing branch, denied branches, and branches outside an allow list when one is configured.

Unless project config explicitly sets `publication.push.deny_branches` (including an empty list), the command MUST apply the built-in deny list `main`, `master`, and `release/*`. This default matches SpecStudio's safe publication posture while still giving project config a deliberate override point.

### Config Mutation

#### REQ: durable-config-writes

When asked to save user or project policy, the CLI MUST write the durable config itself. Agent skills SHOULD call this command instead of editing `specscore.yaml` or user config directly.

#### REQ: action-normalization

The CLI MUST accept the first-run workflow labels or equivalent flags for `just edit`, `stage`, `commit`, and `commit & push`, and MUST persist them as canonical action lists:

| Input | Persisted actions |
|---|---|
| `just-edit` | `[]` |
| `stage` | `[stage]` |
| `commit` | `[stage, commit]` |
| `commit-and-push` | `[stage, commit, push]` |

Invalid action sequences MUST be rejected before writing.

### Resolution Output

#### REQ: resolution-output-shape

Policy resolution output MUST include:

- `actions_resolved`
- `actions_allowed`
- `actions_blocked`
- `policy_sources`
- `branch`
- `branch_push_allowed`
- `branch_block_reason`

`actions_resolved` is the requested action list before safety filtering. `actions_allowed` is the action list the caller may execute. `actions_blocked` lists actions removed or refused with reasons.

#### REQ: no-user-prompting

CLI publication commands MUST NOT prompt for user approval. They are deterministic helpers. Interactive preference collection belongs to calling skills or frontends, which then pass explicit arguments to the CLI.

### Manifest Helpers

#### REQ: touched-path-output

Any CLI publication command or status-transition helper that edits files MUST output the touched paths in a machine-readable field when `--format yaml` or `--format json` is requested. Agent callers use this output to maintain their publication manifest.

#### REQ: manifest-commit-helper

If the CLI provides a helper to run `git commit`, that helper MUST require an explicit path manifest or an explicit "use current index" flag. Without one of those inputs, it MUST refuse to commit. The helper MUST report unrelated staged paths before committing when a manifest is supplied.

#### REQ: manifest-push-helper

If the CLI provides a helper to run `git push`, that helper MUST first run the branch-check logic and MUST push only the current branch to its configured upstream in MVP. Creating upstream branches is out of scope unless a future Feature adds an explicit opt-in.

## Acceptance Criteria

### AC: set-project-event-policy (verifies REQ: publication-command-group, REQ: set-policy-command, REQ: durable-config-writes)

**Given** a user wants project policy `idea.approved` to run stage, commit, and push,
**When** the caller runs `specscore publication set --scope project --event idea.approved --actions stage,commit,push`,
**Then** the CLI writes canonical publication config to `specscore.yaml` without modifying unrelated config fields.

### AC: shorthand-normalized (verifies REQ: action-normalization)

**Given** the caller runs `specscore publication set --scope user --default commit-and-push`,
**When** the CLI writes user config,
**Then** it persists `actions: [stage, commit, push]`.

### AC: resolve-machine-readable (verifies REQ: resolve-policy-command, REQ: resolution-output-shape)

**Given** user and project publication config both exist,
**When** the caller runs `specscore publication resolve --command ideate --event idea.approved --format yaml`,
**Then** stdout contains the resolved and allowed actions, policy sources, branch decision, and any blocked action reasons.

### AC: branch-check-denies-main (verifies REQ: branch-check-command)

**Given** branch policy denies `main` either through config or through the built-in default deny list,
**When** the caller runs `specscore publication branch-check --branch main`,
**Then** the command exits non-zero and reports that push is denied for `main`.

### AC: project-can-override-default-deny (verifies REQ: branch-check-command)

**Given** project config explicitly sets `publication.push.deny_branches: []`,
**When** the caller runs `specscore publication branch-check --branch main`,
**Then** the built-in deny list is disabled for that project and the branch is allowed unless another configured rule blocks it.

### AC: no-interactive-prompts (verifies REQ: no-user-prompting)

**Given** any publication command is invoked in a non-interactive script,
**When** required arguments are missing or invalid,
**Then** the command exits with the standard invalid-arguments code and writes an error to stderr rather than prompting.

### AC: touched-paths-returned (verifies REQ: touched-path-output)

**Given** a CLI helper mutates `specscore.yaml`,
**When** it exits successfully with `--format yaml`,
**Then** stdout includes `touched_paths` containing `specscore.yaml`.

### AC: manifest-required-for-commit-helper (verifies REQ: manifest-commit-helper)

**Given** the CLI commit helper is invoked without a manifest and without an explicit use-current-index flag,
**When** it runs,
**Then** it refuses to commit and explains the missing intent boundary.

### AC: push-current-upstream-only (verifies REQ: manifest-push-helper)

**Given** branch policy allows push and the current branch has an upstream,
**When** the CLI push helper runs,
**Then** it pushes only the current branch to that configured upstream.

## Open Questions

- Should `publication set` use repeated flags (`--action stage --action commit`) or comma-separated `--actions stage,commit`? Lean: support both if parser cost is low; document comma-separated first.
- Should config mutation live under `specscore publication set`, or under a broader `specscore config set publication...` namespace? Lean: `publication` group because resolve and branch-check are not generic config writes.
- Should the CLI implement commit/push helpers in MVP, or only config/resolve/branch-check? Lean: implement config/resolve/branch-check first; add commit/push helpers only when a skill is ready to consume them.

## Implementation Notes

- MVP implements `publication set`, `publication resolve`, and `publication branch-check`.
- Manifest-safe commit and push helpers are future work. This repo currently has no natural manifest-safe git helper surface for publication operations, so the MVP intentionally leaves those helpers out rather than adding a second git-operation boundary.

---
*This document follows the https://specscore.md/feature-specification*
