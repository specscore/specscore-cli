---
name: specscore-self-update
description: |
  Update the installed specscore CLI to the latest release with `specscore self-update`. Use when the user asks to update, upgrade, or get the newest version of the specscore CLI, when a skill reports the CLI is out of date, or when a feature requires a newer CLI than the one installed.
user-invocable: true
---

# Update the specscore CLI

This skill updates an already-installed [`specscore`](https://github.com/synchestra-io/specscore) CLI to the latest release using the CLI's own `specscore self-update` command (alias `specscore update`). It is the counterpart to [`install`](../install/SKILL.md): `install` bootstraps the CLI when it is missing; `self-update` moves an existing install forward.

**CLI reference:** [`cli/self-update`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/self-update/README.md)

## When to use

- The user asks to update / upgrade the `specscore` CLI, or to get the latest version.
- A skill or command reports a feature needs a newer CLI than the one installed (e.g. an `exit 8` "unknown subcommand" from a recently added verb).
- The user wants to check whether a newer `specscore` release is available.

If `specscore` is **not installed at all** (`command -v specscore` returns nothing), use [`install`](../install/SKILL.md) instead — there is nothing to update.

## How it works

`specscore self-update` first detects **how the binary was installed**, then does the right thing for that channel:

- **Package-managed** (Homebrew, Scoop, WinGet) — the command does **not** overwrite the binary (that would corrupt the manager's bookkeeping). It prints the exact manager upgrade command for the user to run.
- **Manual** (release-archive download or `go install`) — the command updates in place: it downloads the matching release asset, verifies its sha256 against the release `checksums.txt`, and atomically replaces the executable.

A `dev` build (one built from source without release ldflags) reports an *undetermined* version; `self-update` cannot compare it, so it treats it as updatable and installs the latest stable release.

## What to do

1. **Check first (read-only, never mutates):**

   ```bash
   specscore self-update --check
   ```

   Exit `0` = already up to date; a non-zero exit means an update is available or the running version is undetermined. The output names the current and latest versions.

2. **Apply the update.** For an interactive user, have them run:

   ```bash
   specscore self-update          # shows `<current> → <latest>`, prompts before replacing
   ```

   When running non-interactively (no TTY), `--yes` is required or the command refuses to replace the binary:

   ```bash
   specscore self-update --yes
   ```

   - **Package-managed install:** the command prints a manager upgrade command (e.g. `brew upgrade …`) instead of self-replacing. Surface that command and let the user run it in their own shell — do not run a package-manager mutation on their behalf.
   - **Pin a specific release** with `--version <tag>` (leading `v` optional); add `--allow-downgrade` to move to an older release than the running build.

## Verification

After the update, confirm the new version:

```bash
specscore --version
```

The output is a bare semver with no `v` prefix (pinned by the [`cli/version`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/version/README.md) feature spec). If it does not reflect the expected release, check `PATH` (a stale binary earlier on `PATH` can shadow the updated one) and re-run.
