# Skills

SpecScore skills fall into two categories: a small set of **infrastructure** skills that handle plugin-level concerns (such as installing the CLI) and a larger set of **CLI-wrapper** skills that expose the `specscore` CLI's command surface to agents. Each CLI-wrapper skill covers a single CLI resource group; per-verb detail lives under `references/` and loads on demand.

See the [agent-skills feature spec](https://github.com/synchestra-io/synchestra/blob/main/spec/features/agent-skills/README.md) for design principles and the progressive-disclosure model. See [ADR-0002](https://github.com/synchestra-io/synchestra/blob/main/spec/decisions/0002-progressive-disclosure-skills.md) and [ADR-0003](https://github.com/synchestra-io/synchestra/blob/main/spec/decisions/0003-skill-naming-plugin-namespace.md) for the structural and naming rules.

## Invocation

All skills are prefixed with the plugin's manifest name. Users invoke them as:

```
/specscore:<skill-name>
```

## Skill categories

- **Infrastructure skills** — CLI/binary-lifecycle actions that bootstrap or maintain the `specscore` binary itself: `install` (bootstrap the CLI when it is missing) and `self-update` (move an existing install to the latest release).
- **CLI-wrapper skills** — one skill per `specscore` CLI resource group (`feature`, `task`, `spec`, `code`, `idea`, `plan`). Each wrapper assumes the CLI is already installed and callable; see the [Pre-flight pattern](#pre-flight-pattern) below for the shared check wrappers must include.
- **Cross-kind action skills** — verb-level skills that span more than one resource group. These exist when an action ("change a status", "search across kinds") has the same shape for every kind it touches and discoverability benefits from one dedicated entry point. Cross-kind skills do not replace the CLI-wrapper skill rows that document the same verb in their own catalog tables — they are additive.

## Available infrastructure skills

| Skill | Purpose |
|---|---|
| [`install/`](install/SKILL.md) | Install the `specscore` CLI via the official `get-cli` installer. Runtime prerequisite for every wrapper skill. |
| [`self-update/`](self-update/SKILL.md) | Update an installed `specscore` CLI to the latest release via `specscore self-update` (self-replace for manual installs; package-manager redirect for Homebrew/Scoop/WinGet). See [`cli/self-update`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/self-update/README.md). |

## Available CLI-wrapper skills

The wrapper catalogue mirrors the `specscore` CLI surface. One resource-level skill per command group, per-verb `references/<verb>.md` inside each. Each row links to both the local skill and the authoritative CLI feature spec that defines the command's contract.

| Skill | Wraps | Verbs | CLI feature spec |
|---|---|---|---|
| [`feature/`](feature/SKILL.md) | `specscore feature ...` | `list`, `info`, `tree`, `deps`, `refs`, `new`, `change-status` | [`cli/feature`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/feature/README.md) |
| [`task/`](task/SKILL.md) | `specscore task ...` | `list`, `info`, `new` | [`cli/task`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/task/README.md) |
| [`spec/`](spec/SKILL.md) | `specscore spec ...` | `lint` | [`cli/spec`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/spec/README.md) |
| [`code/`](code/SKILL.md) | `specscore code ...` | `deps` | [`cli/code`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/code/README.md) |
| [`idea/`](idea/SKILL.md) | `specscore idea ...` | `new`, `change-status` | [`cli/idea`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/idea/README.md) |
| [`plan/`](plan/SKILL.md) | `specscore plan ...` | `list`, `info`, `new` | [`cli/plan`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/plan/README.md) |

## Available cross-kind action skills

Cross-kind skills surface a single action that has the same shape across multiple resource kinds. They dispatch internally by kind to the matching CLI verb. The per-kind `change-status` rows in the CLI-wrapper catalog above remain as deep links for navigation-flow users.

| Skill | Action | Kinds covered | Wraps |
|---|---|---|---|
| [`change-status/`](change-status/SKILL.md) | Transition a SpecScore artifact's `**Status:**` field | Feature, Idea | `specscore feature change-status`, `specscore idea change-status` |

The Synchestra.io CLI ecosystem (synchestra, specscore, rehearse) standardises on a strict `resource + verb` command shape; every skill in the catalogue follows it. The shared exit-code contract, output-format conventions, and `--project` autodetect rules that every wrapper assumes are defined once in the [CLI parent feature](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/README.md).

The exact shape of each resource skill follows the template defined in [`agent-skills` feature spec](https://github.com/synchestra-io/synchestra/blob/main/spec/features/agent-skills/README.md#skill-file-format):

```
<resource>/
  SKILL.md         ← index table: user intent → reference file
  references/
    <verb>.md      ← full instructions for one CLI invocation
```

## Pre-flight pattern

Every CLI-wrapper skill must verify that `specscore` is installed before invoking it. Copy the block below verbatim into the top of any new wrapper `SKILL.md`:

> ### Pre-flight check
>
> Before running any `specscore` command, verify the CLI is installed:
>
> ```bash
> command -v specscore >/dev/null 2>&1
> ```
>
> If this check fails (exit `127` / `command not found`), stop and tell the user exactly:
>
> > The `specscore` CLI is not installed. Either:
> > - invoke `/specscore:install` to see install options, or
> > - install directly from <https://specscore.md/install>.
> >
> > Then retry your command.
>
> Do not proceed with the original command until `specscore --version` succeeds.

Each shipped wrapper SKILL.md embeds this block verbatim at the top. If the snippet ever needs to evolve (e.g., a different install URL), update it once here and propagate the change through every wrapper.

## Status

**Shipped:** `install/` (infrastructure), `feature/`, `task/`, `spec/`, `code/`, `idea/`, `plan/` (CLI wrappers).

All wrappers in the catalogue are now available. New CLI verbs land in the relevant skill's `references/` directory in the same release cycle as the CLI surface change.

## Open Questions
- Whether to ship a `design/` skill in this plugin is **parked**. There is no `specscore design` CLI command today. If one is added later, a wrapping skill lands in the same release. Until then, "design" is purely a methodology concept and lives in the [`spec-studio`](https://github.com/synchestra-io/spec-studio) plugin.
