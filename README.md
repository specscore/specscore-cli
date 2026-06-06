# specscore-cli

[![CI](https://github.com/specscore/specscore-cli/actions/workflows/go-ci.yml/badge.svg)](https://github.com/specscore/specscore-cli/actions/workflows/go-ci.yml)
[![Dogfood lint](https://github.com/specscore/specscore-cli/actions/workflows/dogfood.yml/badge.svg)](https://github.com/specscore/specscore-cli/actions/workflows/dogfood.yml)
[![Coverage](https://img.shields.io/badge/coverage-100%25-brightgreen)](https://github.com/specscore/specscore-cli/actions/workflows/go-ci.yml)

CLI for [SpecScore](https://specscore.md) — lint, query, and scaffold SpecScore specifications.

## Install

### macOS / Linux — curl

```bash
curl -fsSL https://specscore.md/install/get-cli | sh
```

### Windows — PowerShell

```powershell
powershell -c "irm https://specscore.md/install/get-cli.ps1 | iex"
```

### macOS — Homebrew

```bash
brew install specscore/tap/specscore
```

### Windows — Scoop / WinGet

```powershell
scoop bucket add specscore https://github.com/specscore/scoop-bucket
scoop install specscore
# or
winget install SpecScore.CLI
```

See [installation docs](https://specscore.md/install) for options (version pinning, custom install dir).

## Usage

```bash
specscore spec lint              # lint the current spec tree
specscore feature list           # list features
specscore feature show <slug>    # inspect a feature
specscore task list              # show the task board
specscore version                # full build identity
specscore --version              # bare semver
```

Full command reference: see [`spec/features/cli/`](spec/features/cli/).

## Updating

Bring an existing install to the latest release:

```bash
specscore self-update            # or: specscore update
specscore self-update --check    # report whether a newer release is available; change nothing
specscore self-update --yes      # skip the confirmation prompt (non-interactive / CI)
```

`self-update` detects how `specscore` was installed. **Package-managed installs** (Homebrew, Scoop, WinGet) are never overwritten — it prints the right manager command to run instead (e.g. `brew upgrade specscore`). **Manual installs** (release-archive download, `go install`) are replaced in place after the downloaded asset's `checksums.txt` entry is verified.

Install a specific release instead of the latest:

```bash
specscore self-update --version v0.6.0                    # leading "v" optional
specscore self-update --version 0.4.0 --allow-downgrade   # an older target requires --allow-downgrade
```

`--check` exit codes: `0` up to date, `10` update available, other non-zero on error — convenient for CI staleness gates. Full contract: [`spec/features/cli/self-update/`](spec/features/cli/self-update/).

## Configure your AI agents

Teach the AI coding agents working in this repo about its SpecScore conventions in one command. `specscore agent setup` writes each agent's instruction file (and, where the agent supports a skills directory, copies the SpecScore skill bundles into it).

Pass agents as a comma-separated list (space-separated and `--all` also work):

```bash
specscore agent setup claude,codex,copilot,cursor,antigravity.google,pi.dev,opencode
```

Supported agents: `claude`, `codex`, `copilot`, `cursor`, `antigravity.google`, `pi.dev`, `opencode`. The command is idempotent (existing files are skipped unless `--force`) and reports every path it adds, modifies, or skips. Full contract: [`spec/features/cli/agent/setup/`](spec/features/cli/agent/setup/).

## AI skills

If you drive `specscore` from inside Claude Code (or any agent host that loads Claude Code plugins), install the [`ai-plugin-specscore`](https://github.com/specscore/ai-plugin-specscore) plugin. It ships agent skills that wrap each CLI resource group — they teach the agent *when* to call which command, *which* flags to pass, and *how* to interpret exit codes, all grounded in the feature specs in this repo.

```
/plugin marketplace add specscore/ai-marketplace
/plugin install specscore@specscore
```

Then bootstrap the CLI itself with `/specscore:install`, or install manually with the one-liner above.

## Test coverage

`specscore-cli` maintains **100% statement coverage** across all packages. This is enforced automatically — the CI pipeline and the local pre-push hook both reject any change that drops below 100%.

All contributions are required to maintain 100% coverage. If your change adds or modifies code, include tests that cover every new branch.

## License

Apache License 2.0 — see [LICENSE](LICENSE).

## Related

- [specscore/specscore](https://github.com/specscore/specscore) — the SpecScore format and documentation
- [specscore/ai-plugin-specscore](https://github.com/specscore/ai-plugin-specscore) — agent skills that wrap this CLI
