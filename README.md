# specscore-cli

[![CI](https://github.com/specscore/specscore-cli/actions/workflows/go-ci.yml/badge.svg)](https://github.com/specscore/specscore-cli/actions/workflows/go-ci.yml)
[![Coverage](https://img.shields.io/badge/coverage-100%25-brightgreen)](https://github.com/specscore/specscore-cli/actions/workflows/go-ci.yml)

CLI for [SpecScore](https://specscore.md) — lint, query, and scaffold SpecScore specifications.

<!-- dev-approach:v1 -->
## Our approach to development

We build with our own tooling:

- **[SpecScore](https://specscore.md)** — specify requirements as `SpecScore.md` artifacts
- **[SpecStudio](https://specscore.studio)** — author & manage specs across their lifecycle
- **[inGitDB](https://ingitdb.com)** — store structured data in Git where applicable
- **[DALgo](https://dalgo.io)** — data access layer for Go
- **[cover100.dev](https://cover100.dev)** — drive toward 100% test coverage
- **[DataTug](https://datatug.io)** — query & explore data
<!-- /dev-approach -->

## Install

### macOS — Homebrew cask (recommended)

```bash
brew install --cask specscore/tap/specscore
```

The Darwin artifact is signed with a Developer ID, notarized by Apple, and the
release fails closed (`require_notarized_macos: true`) if that ever regresses
— see [`spec/features/cli/release-distribution/`](spec/features/cli/release-distribution/README.md)
for the verification evidence. Upgrade with `brew upgrade --cask specscore`.
Do not bypass Gatekeeper or remove quarantine attributes.

### macOS — source build (secondary)

```bash
go install github.com/specscore/specscore-cli/cmd/specscore@latest
```

This compiles the latest published source release locally. Automated or agent
evidence MUST instead pin an exact released tag or merged commit SHA, then
verify the resulting build identity; never use `@main`.

### Linux — curl

```bash
curl -fsSL https://specscore.md/install/get-cli | sh
```

### Windows — PowerShell

```powershell
powershell -c "irm https://specscore.md/install/get-cli.ps1 | iex"
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
specscore rehearse run           # execute markdown acceptance scenarios
specscore version                # full build identity
specscore --version              # bare semver
```

Full command reference: see [`spec/features/cli/`](spec/features/cli/).

### SpecScore Studio — multi-repo fact indexing

`specscore studio` federates per-repo artifacts (spec trees, CodeGrapher `codegraph/` snapshots, `go.mod`/`package.json` manifests, ops registries) across a whole ecosystem into one queryable fact store. Describe the ecosystem in a `studio.yaml` workspace file:

```yaml
name: demo
repos:              # directory paths or globs, absolute or workspace-relative
  - ../repo-a
  - ../org/*        # glob entries expand to existing directories
  - path: ../ops    # mapping form: extra registry files for this repo
    registries: [data/domains.json]
```

Then index and query:

```bash
specscore studio index                          # rebuild the store from ./studio.yaml
specscore studio index --workspace path/to/studio.yaml --strict   # any warning exits 3
specscore studio facts --predicate has-status   # table of matching facts
specscore studio facts --subject 'repo-a#*' --format json         # full fact shape
specscore studio facts --predicate imports --count                # row count only
```

`studio index` rebuilds `<workspace-dir>/.specscore-studio/facts.db` from scratch on every run (override with `--db`) and prints a summary with per-repo and per-adapter fact counts plus every warning. Broken repos, adapters, or files are skipped at the smallest granularity and reported as warnings; the exit code stays 0 unless `--strict` is set.

Every run also exports the facts as [INGR](https://ingitdb.com) recordsets — the same encoding as the committed `codegraph/` snapshots — one directory per repo slug under `<workspace-dir>/.specscore-studio/ingr/<repo-slug>/facts.ingr` (override the root with `--ingr-dir`, skip with `--no-ingr`). Each recordset starts with a fixed header naming the nine fact fields (`subject, predicate, object, evidence_class, evidence_pointer, adapter_id, adapter_version, observed_at, ecosystem`), followed by one JSON value per line (nine lines per record) and a `# <n> records` trailer; the per-repo record count always equals that repo's fact count in the index summary. Full contract: [`spec/features/cli/studio/index/`](spec/features/cli/studio/index/).

### Rehearse — executable acceptance scenarios

`specscore rehearse run <paths...>` executes markdown acceptance scenarios — files carrying `**Verifies:**` AC identity in body metadata plus fenced executable step blocks — and reports per-scenario, per-AC pass/fail:

```bash
specscore rehearse run                                # inside a SpecScore repo: all spec/features/**/_tests/
specscore rehearse run spec/features/cli/studio/index/_tests   # a directory (recursive *.md, excluding README.md)
specscore rehearse run scenario.md --format json      # a single file, machine-readable report
```

Discovery accepts files, directories, and globs; explicit paths work in any directory — no `specscore.yaml` required (standalone mode). A scenario's step blocks run in order in one scenario-scoped temp working dir; the first failing step fails the scenario and the remaining steps are skipped-after-failure.

**Step-block kinds** (v0.3) — one scenario can mix all five:

````markdown
# Rehearse: checkout applies a discount

**Status:** pending
**Verifies:** shop/checkout#ac:discount-applied

```bash
sqlite3 app.db < seed.sql                # runs via bash -euo pipefail
echo "uid=42" >> "$REHEARSE_CAPTURES"    # capture into the context bag
```

```hurl
POST http://127.0.0.1:8080/checkout
{"user": {{uid}}}
HTTP 200
[Captures]
order_id: jsonpath "$.id"
```

```sql dsn=sqlite:app.db
SELECT username FROM orders WHERE id = {{order_id}};
-- assert-rows: 1
-- capture: name = username
```

```dtql db=.specscore-studio/facts.db
from:
  name: facts
-- assert-rows: 128
```

```graphql url=http://127.0.0.1:8080/graphql
query { order(id: {{order_id}}) { ok } }
-- variables: {}
-- assert-jsonpath: $.data.order.ok == true
```
````

`hurl` and `graphql` blocks delegate to the [hurl](https://hurl.dev) binary (`hurl --test`) — the runner ships no HTTP client of its own. When `hurl` is missing from PATH, scenarios containing hurl-derived blocks are reported `skipped` (with a warning naming the binary) rather than failed, and skips never affect the exit code. `sql` runs against a DSN (v0.3 driver: `sqlite:<path>`); `dtql` runs a [dalgo](https://github.com/dal-go/dalgo) DTQL query document against a SQLite store — which makes a Studio fact store (`facts.db`) directly assertable by scenarios.

**Directives** are trailing `-- name: value` comment lines inside declarative blocks: `-- assert-rows: <N>` and `-- assert-row-json: {...}` (sql/dtql), `-- capture: <name> = <column>` (sql/dtql), `-- variables: {...}` and `-- assert-jsonpath: <path> == <json-value>` (graphql), `-- capture-jsonpath: <name> = <path>` (graphql).

**Context bag.** Each scenario owns an ordered map of string variables shared across its steps. Consumption is per block class: `bash`/`sql`/`dtql` bodies and info-string params have `{{name}}` placeholders textually interpolated before execution (an unknown variable fails the step); hurl-derived blocks (`hurl`, `graphql`) get the bag as `--variable name=value` flags instead — Hurl owns the `{{name}}` syntax natively, so multi-request hurl blocks stay verbatim-valid. Captures into the bag: `bash` appends `name=value` lines to `$REHEARSE_CAPTURES`; `hurl` uses native `[Captures]`; `sql`/`dtql`/`graphql` use the capture directives above.

**Reporting & exit codes.** The human report is one line per scenario (status, file, `Verifies:` AC ids, duration) plus totals; `--format json` emits `[{file, status, verifies[], duration_ms, bag{}, steps[{kind, status, detail}]}]`. Exit `0` when no scenario failed, `1` when any failed, `2` on usage/config errors — including when discovery matches zero scenario files.

The runner is self-hosting: the committed acceptance corpus under `spec/features/**/_tests/` (including the Rehearse feature's own scenarios) runs green through `specscore rehearse run`, and CI executes it on every change (the `Rehearse corpus` job in [`go-ci.yml`](.github/workflows/go-ci.yml)). Full contract: [`spec/features/cli/rehearse/run/`](spec/features/cli/rehearse/run/).

## Updating

Bring an existing install to the latest release:

```bash
specscore self-update            # or: specscore update
specscore self-update --check    # report whether a newer release is available, and the next step; change nothing
specscore self-update --yes      # skip the confirmation prompt (non-interactive / CI)
specscore self-update --dry-run  # show what would happen (target version, download URL) without changing anything
```

`self-update` detects how `specscore` was installed. **Package-managed installs** are never overwritten; the command reports the manager-owned next step. **Manual installs** (release-archive download, `go install`) are replaced in place after the downloaded asset's `checksums.txt` entry is verified. On macOS, a Homebrew cask install reports the manager-owned `brew upgrade --cask specscore` next step; the source-build channel above upgrades in place like any other manual install.

The detection, download, verification, and replacement logic all live in the shared [`github.com/strongo/selfupdate`](https://github.com/strongo/selfupdate) module — specscore only supplies its own release identity (binary name, repository, managers) and exit-code contract. See [`spec/features/cli/self-update/`](spec/features/cli/self-update/) for what's specscore's own versus inherited from the library.

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

## Code exploration & spec ↔ code linkage

We use [Codegrapher](https://codegrapher.dev/) for efficient code exploration and bidirectional linkage between specifications and source code. You can [browse this repository's code graph online](https://codegrapher.dev/github.com/specscore/specscore-cli) — the directory tree and quick file search are served from the committed snapshot in [`codegrapher/`](codegrapher/). Codegrapher indexes the codebase into a queryable knowledge graph of symbols and their relationships, letting AI agents navigate and trace code quickly instead of grepping — and connect SpecScore specs to the code that implements them, and back.

## Test coverage

`specscore-cli` maintains **100% statement coverage** across all packages. This is enforced automatically — the CI pipeline and the local pre-push hook both reject any change that drops below 100%.

All contributions are required to maintain 100% coverage. If your change adds or modifies code, include tests that cover every new branch.

## Releasing

> [!IMPORTANT]
> **Do not bump the version manually.** There is no version string to edit in
> source — the version is derived from the git tag at build time. Cut releases
> only through the [Release workflow](.github/workflows/release.yml):
>
> - **Actions → Release → Run workflow**, then pick `auto` (next version from
>   conventional commits since the last tag), `patch` / `minor` / `major`, or an
>   explicit `vX.Y.Z`; or
> - push a `vX.Y.Z` tag.
>
> Or from the CLI: `gh workflow run release.yml -f release_tag=auto`. The
> workflow tags, builds, and publishes the GitHub release; afterwards run
> `specscore self-update` to pull the new binary locally.

## License

Apache License 2.0 — see [LICENSE](LICENSE).

## Related

- [specscore/specscore](https://github.com/specscore/specscore) — the SpecScore format and documentation
- [specscore/ai-plugin-specscore](https://github.com/specscore/ai-plugin-specscore) — agent skills that wrap this CLI
