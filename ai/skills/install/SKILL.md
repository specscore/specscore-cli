---
name: specscore-install
description: |
  Show install instructions for the specscore CLI. Use when `specscore` is not on PATH, when another skill reports `command not found: specscore`, or when the user asks how to install or reinstall the CLI. To UPDATE an already-installed CLI to a newer release, use the `self-update` skill instead.
user-invocable: true
---

# Install the specscore CLI

This skill **shows the user how to install** the [`specscore`](https://github.com/synchestra-io/specscore) CLI — the runtime prerequisite for every other skill in this plugin. It does not execute the installer. The user runs the install command themselves in their own shell.

## When to use

- `specscore` is not on `PATH` (e.g., `command -v specscore` returns nothing).
- Another skill in this plugin reported `command not found: specscore` or a pre-flight failure.
- The user explicitly asked to install, reinstall, or update the `specscore` CLI.

## Behavior

**Do not run `curl … | sh` from this skill.** Piping remote code into a shell is a trust decision that belongs to the user, in their own terminal. Surface the options below, then wait.

## What to show the user

Present both paths and let the user pick:

### 1. One-liner (macOS, Linux, WSL)

```bash
curl -fsSL https://specscore.md/get-cli | sh
```

Re-running is safe — the installer replaces the existing binary in place.

> **Updating an existing install?** Once the CLI is installed, the canonical way to move to the latest release is the CLI's own update command, `specscore self-update` — see the [`self-update`](../self-update/SKILL.md) skill. Re-running the installer above also works for manual installs.

### 2. Manual install (all platforms, including native Windows PowerShell)

Follow the platform-specific instructions at <https://specscore.md/install>, or download the matching archive directly from the [GitHub Releases](https://github.com/synchestra-io/specscore/releases) page.

## Verification

Once the user confirms install is complete, verify with:

```bash
specscore --version
```

If the binary is found and prints a version, continue with the user's original request. If not, ask the user to check their `PATH` and re-run install.

The output shape (bare semver, no `v` prefix) is pinned by the [`cli/version`](https://github.com/synchestra-io/specscore-cli/blob/main/spec/features/cli/version/README.md) feature spec.
