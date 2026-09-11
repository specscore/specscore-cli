# specscore-cli — Maintainer Notes

Internal notes for anyone (human or AI agent) working on the CLI itself. User-facing docs live in [`README.md`](README.md). The companion spec repo is [`specscore`](https://github.com/specscore/specscore) — its `AGENTS.md` covers the SpecScore **format**; this file covers the **tool**.

## Repository scope

Go CLI that implements `specscore` — lint, query, scaffold, and lifecycle-transition commands over SpecScore-formatted specifications. Spec for each subcommand lives in `spec/features/cli/<verb>/`; implementation in `cmd/`, `internal/cli/`, `pkg/`.

## Build, test, lint

- `go build ./...` — sanity check the build.
- `go test ./...` — full test suite (≈10–15s).
- `gofmt -l .` — every commit MUST be gofmt-clean (Go CI gates on this).
- `go build -o /tmp/specscore ./cmd/specscore && /tmp/specscore spec lint` — dogfood against the repo's own spec tree.

## Release workflow

Releases are continuous. Every merge to `main` runs `release.yml`, which delegates to the shared `strongo/cicd` release workflow: it infers the bump from the conventional commits since the last tag (`feat:` → minor, `fix:` → patch; `chore:`/`ci:`/`docs:`/`refactor:` release nothing; a breaking change is capped to a minor bump while pre-1.0), then tags, builds, signs, and publishes via GoReleaser — gated on `Go CI` being green for that commit. Pushing an explicit `vX.Y.Z` tag releases exactly that version. There is no dispatch input to pick a bump level or version: `gh workflow run release.yml` only re-runs the merge path for the head of `main`, i.e. it is a retry, not a release trigger. Details in the README's [Releasing](README.md#releasing) section.

After release, install with `curl -fsSL https://specscore.md/install/get-cli | sh` and verify `specscore --version` matches.

## Shipping a convention change

Convention changes (renamed sections, tightened rules, new required fields) coordinate across two repos and have a load-bearing gotcha: **CI in downstream repos pins `SPECSCORE_VERSION`**. If the pin lags behind a convention change, CI silently runs the old CLI against new artifacts. The `dogfood-version-bump` lint rule (`pkg/lint/dogfood_version.go`, warning severity) surfaces this drift, but the workflow below avoids it in the first place.

When changing a convention:

1. **Spec the change in the meta-spec** (`specscore` repo). Update the entity Features, indexes, AGENTS.md if relevant. PR + merge to main.
2. **Implement in the CLI** (this repo). Update lint rules, scaffolders, parsers, tests. Include spec edits to `spec/features/cli/spec/lint/README.md` if a new REQ or AC is added.
3. **Release the CLI** — merging step 2 to `main` with a `feat:`/`fix:` commit releases it automatically. Wait for the new tag to publish.
4. **Bump the pin** — every downstream repo's `.github/workflows/dogfood.yml` needs `SPECSCORE_VERSION: vNEW`. Start with `specscore` itself; sibling repos (`specstudio-skills`, `ai-plugin-specscore`, etc.) follow. Each bump is its own commit per the "`# bump intentionally via PR`" convention.
5. **Migrate existing artifacts** by running `specscore spec lint --fix` in each touched repo. Commit the mechanical rewrites.

Steps 1–3 are the same-PR critical path inside this repo + spec repo. Steps 4–5 are repeated per downstream consumer.

## Lint rules and the spec

Lint rules in `pkg/lint/*.go` are spec-anchored: every rule's behavior is described as REQs and ACs in `spec/features/cli/spec/lint/README.md` (or a child Feature like `plan-rules/`). When changing rule behavior, update the spec in the same commit. The dogfood lint job will fail if the spec and the rule disagree about, e.g., severity or message text — that's intentional.
