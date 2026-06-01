# Feature: Self-Update

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/self-update?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/self-update?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/self-update?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/self-update?op=request-change) |

**Status:** Approved
**Source Ideas:** cli-self-update

## Summary

`specscore self-update` (alias `specscore update`) brings a running `specscore` binary to the latest released version. It first detects how the binary was installed: package-managed installs (Homebrew, Scoop, WinGet) are never overwritten — the command prints the exact manager upgrade command instead — while manual installs (release-archive downloads, `go install`) are updated in place by downloading the matching release asset, verifying its sha256 against the release `checksums.txt`, and atomically replacing the executable. A `--check` mode reports update availability for any install method without modifying anything.

## Synopsis

```
specscore self-update            # detect, then self-replace (manual) or redirect (managed)
specscore self-update --check    # report availability only; never modifies
specscore self-update --yes      # skip the confirmation prompt (non-interactive)
specscore update                 # alias for `self-update`
```

## Problem

The CLI is distributed through several channels — a Homebrew tap, a Scoop bucket, WinGet, and direct GitHub release archives (plus `go install`). Users have no first-class way to move to the latest version, and the naive fix — "just overwrite the binary" — is only correct for manual installs. Overwriting a package-managed binary corrupts the manager's bookkeeping: the next `brew upgrade` sees an unexpected file, and the user ends up with two conflicting notions of "installed version." Conversely, telling a manual-install user to go re-download a tarball is exactly the friction a self-update command exists to remove.

The hard part is therefore not the file swap; it is **deciding whether a swap is even allowed**. Install-method detection is part of the product contract, not an implementation detail. When detection is uncertain, the safe outcome (do not self-replace; guide the user) must be the default.

## Behavior

### Command surface

The capability is a single command with one canonical name, one alias, and two modifier flags.

#### REQ: command-and-alias

The CLI MUST expose the command as `specscore self-update`. `specscore update` MUST be accepted as an alias that resolves to identical behavior. The canonical name is `self-update` because, in a CLI dominated by artifact verbs (`idea new`, `feature new`, `spec lint`), a bare `update` is ambiguous about *what* is updated.

#### REQ: check-flag

A `--check` boolean flag MUST be accepted. In check mode the command performs install-method detection and the version check, reports the result, and exits without downloading or modifying anything (see [REQ: check-no-mutation](#req-check-no-mutation)).

#### REQ: confirm-before-replace

For the self-replace path, the command MUST show the version transition (`<current> → <latest>`) and require interactive confirmation before replacing the executable. A `--yes` flag (short `-y`) MUST skip the prompt for non-interactive use. When the command is not attached to an interactive terminal and `--yes` was not given, it MUST refuse to replace and exit non-zero rather than block on input.

### Install-method detection

Detection chooses between two mutually exclusive outcomes — *managed* (redirect) or *manual* (self-replace eligible) — preferring explicit signals over guesswork.

#### REQ: detect-managed

The command MUST classify the running binary as package-managed when its resolved executable path matches a known manager layout: a Homebrew Cellar/prefix path, a Scoop `apps`/`shims` path, or a WinGet `Packages`/`Links` path. A managed classification MUST route to the redirect path and MUST NOT self-replace.

#### REQ: detect-manual

The command MUST classify the running binary as manual when it is not recognized as managed and the path is a plausible user/Go install location (e.g., a release archive extracted to `~/bin` or `/usr/local/bin`, or a `go install` target under `GOBIN`/`GOPATH/bin`). A manual classification is eligible for the self-replace path.

#### REQ: ambiguous-safe-default

When detection cannot confidently classify the install method, the command MUST default to the safe outcome: do not self-replace, surface the ambiguity, and print manual-update guidance. Ambiguity MUST NOT resolve to "manual."

### Package-managed redirect

Managed installs are guided, never modified.

#### REQ: managed-no-overwrite

For a managed classification the command MUST NOT download, write, or replace the executable under any flag combination except `--check` (which never writes regardless).

#### REQ: managed-redirect-command

For a managed classification the command MUST print the detected manager and the exact upgrade command for it (`brew upgrade specscore`, `scoop update specscore`, or `winget upgrade SpecScore.CLI`) and exit `0`.

### Version check

Both the action and `--check` compare the running version against the latest release.

#### REQ: latest-release-source

The command MUST determine the latest version from the published GitHub releases of `specscore/specscore-cli`, considering only the latest non-prerelease (stable) release.

#### REQ: dev-build-undetermined

When the running binary reports the `dev` version placeholder (a build without `-ldflags`, e.g. `go install` of an untagged tree), the command MUST treat the current version as undetermined: `--check` reports it as undetermined (not "up to date"), and the self-replace path MAY offer to install the latest stable release subject to the normal confirmation in [REQ: confirm-before-replace](#req-confirm-before-replace).

### Self-replace (manual installs)

The manual path downloads, verifies, and atomically swaps the binary.

#### REQ: download-matching-asset

For an eligible self-replace the command MUST download the release asset matching the host OS and architecture from the latest stable release.

#### REQ: checksum-verification

Before replacing the executable the command MUST verify the downloaded asset's sha256 against the release `checksums.txt`. On mismatch (or a missing/unfetchable checksum entry) the command MUST abort with a non-zero exit and MUST NOT modify the existing binary.

#### REQ: atomic-replace

The executable MUST be replaced atomically: the verified new binary is staged and swapped into place such that an interrupted or failed operation leaves the original binary intact and runnable (no partial or truncated executable). This holds across macOS, Linux, and Windows (including replacing a running executable on Windows).

#### REQ: no-op-when-current

When the running version already equals the latest stable release, the command MUST report that it is up to date and exit `0` without downloading or replacing anything.

### Check-only mode

`--check` is read-only and scriptable.

#### REQ: check-no-mutation

With `--check` the command MUST NOT download an asset or modify the executable for any install method; it performs detection and the version check only.

#### REQ: check-exit-codes

`--check` MUST use exit code `0` when the binary is up to date, `10` when an update is available (or the current version is undetermined per [REQ: dev-build-undetermined](#req-dev-build-undetermined)), and a code distinct from `0` and `10` for operational errors (e.g., release lookup failure). The "update available" code MUST NOT collide with the error codes.

### Failure modes

Failures are explicit and never leave a broken binary.

#### REQ: network-failure-clear

When the release lookup or asset download fails (network error, rate limit, missing asset), the command MUST print a clear error, exit non-zero, and MUST NOT modify the existing binary.

#### REQ: permission-failure-clear

When the command lacks permission to replace the executable in its install location, it MUST report the failure with the path and a suggested remedy (e.g., re-run with elevated permissions or use the package manager), exit non-zero, and leave the original binary intact.

## Exit codes

| Exit code | Meaning |
|---|---|
| `0` | Success: self-replace completed, redirect printed, or already up to date |
| `10` | `--check` only — an update is available, or the current version is undetermined |
| non-zero (other) | Operational error: detection-ambiguous refusal, network/download failure, checksum mismatch, permission denied, or non-interactive without `--yes` |

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [CLI](../README.md) | Parent feature. Inherits shared CLI conventions and the shared error path. |
| [Version](../version/README.md) | The running version read by `self-update` is the build-time value pinned by `version`, including its `dev` placeholder behavior consumed by [REQ: dev-build-undetermined](#req-dev-build-undetermined). |

## Rehearse Integration

Every acceptance criterion below is testable through the CLI surface or as a pure-function unit (install-method detection from a path string; checksum comparison; exit-code mapping). Rehearse scenario stubs are **deferred to the Plan phase** to match the current repo convention (no sibling Feature carries a `_tests/` tree yet). This is a recorded scope decision, not a claim of untestability; the user may override and scaffold stubs now.

## Acceptance Criteria

### AC: canonical-and-alias

**Requirements:** cli/self-update#req:command-and-alias

**Given** an installed `specscore` binary
**When** the user runs `specscore self-update --check` and, separately, `specscore update --check`
**Then** both invocations execute the same command and produce identical output and exit code.

### AC: check-is-readonly

**Requirements:** cli/self-update#req:check-flag, cli/self-update#req:check-no-mutation

**Given** any install method and an available newer release
**When** the user runs `specscore self-update --check`
**Then** the command prints availability and the appropriate next step, and the on-disk executable is byte-for-byte unchanged (no download, no replace).

### AC: confirm-prompt-and-yes

**Requirements:** cli/self-update#req:confirm-before-replace

**Given** a manual install with a newer release available, attached to an interactive terminal
**When** the user runs `specscore self-update` without `--yes`
**Then** the command prints `<current> → <latest>` and waits for confirmation; running it with `--yes` performs the replacement without prompting.

### AC: noninteractive-without-yes-refuses

**Requirements:** cli/self-update#req:confirm-before-replace

**Given** a manual install with a newer release, running without an interactive terminal
**When** the user runs `specscore self-update` without `--yes`
**Then** the command refuses to replace, prints that `--yes` is required for non-interactive use, and exits non-zero, leaving the binary unchanged.

### AC: managed-is-redirected

**Requirements:** cli/self-update#req:detect-managed, cli/self-update#req:managed-no-overwrite, cli/self-update#req:managed-redirect-command

**Given** a `specscore` whose executable path is a Homebrew, Scoop, or WinGet managed location
**When** the user runs `specscore self-update`
**Then** the command prints the detected manager and its exact upgrade command, exits `0`, and the executable is unchanged.

### AC: manual-is-eligible

**Requirements:** cli/self-update#req:detect-manual

**Given** a `specscore` extracted from a release archive into `/usr/local/bin` (or installed via `go install`)
**When** the user runs `specscore self-update --check`
**Then** the command classifies the install as manual and reports the self-update path as available.

### AC: ambiguous-falls-back-safe

**Requirements:** cli/self-update#req:ambiguous-safe-default

**Given** a `specscore` whose install method cannot be confidently classified
**When** the user runs `specscore self-update`
**Then** the command does not replace the binary, states that the install method is ambiguous, prints manual-update guidance, and exits non-zero.

### AC: latest-stable-only

**Requirements:** cli/self-update#req:latest-release-source

**Given** the project's GitHub releases where the newest tagged release is a prerelease and the newest stable release is older
**When** the user runs `specscore self-update --check`
**Then** the "latest" the command compares against is the newest stable (non-prerelease) release, ignoring the prerelease.

### AC: dev-build-is-undetermined

**Requirements:** cli/self-update#req:dev-build-undetermined

**Given** a binary built without `-ldflags` (version reports `dev`)
**When** the user runs `specscore self-update --check`
**Then** the command reports the current version as undetermined (not "up to date") and exits `10`.

### AC: checksum-mismatch-aborts

**Requirements:** cli/self-update#req:download-matching-asset, cli/self-update#req:checksum-verification

**Given** a manual install where the downloaded asset's sha256 does not match the release `checksums.txt`
**When** `specscore self-update` runs the replacement
**Then** the command aborts before touching the executable, reports the verification failure, exits non-zero, and the original binary remains in place and runnable.

### AC: replace-is-atomic

**Requirements:** cli/self-update#req:atomic-replace

**Given** a manual install on macOS, Linux, or Windows
**When** a verified asset is swapped in and the operation is interrupted or fails mid-way
**Then** the install location still contains a complete, runnable `specscore` binary (either the original or the new version) — never a partial or truncated file.

### AC: already-current-noop

**Requirements:** cli/self-update#req:no-op-when-current

**Given** a manual install already on the latest stable release
**When** the user runs `specscore self-update`
**Then** the command reports it is up to date and exits `0` without downloading or replacing anything.

### AC: check-exit-code-contract

**Requirements:** cli/self-update#req:check-exit-codes

**Given** three scenarios — up to date, update available, and a release-lookup error
**When** the user runs `specscore self-update --check` in each
**Then** the exit codes are `0`, `10`, and a third code distinct from both, respectively.

### AC: network-failure-is-safe

**Requirements:** cli/self-update#req:network-failure-clear

**Given** a manual install and an unreachable release source
**When** the user runs `specscore self-update`
**Then** the command prints a clear error, exits non-zero, and does not modify the existing binary.

### AC: permission-denied-is-safe

**Requirements:** cli/self-update#req:permission-failure-clear

**Given** a manual install where the executable's directory is not writable by the current user
**When** the user runs `specscore self-update --yes`
**Then** the command reports the permission failure with the path and a suggested remedy, exits non-zero, and leaves the original binary intact.

## Open Questions

- Which release-aware updater library backs the atomic-replace path (e.g. `minio/selfupdate`, `creativeprojects/go-selfupdate`), and does it consume GoReleaser's archive + `checksums.txt` layout without custom glue? (Implementation choice; does not change this contract.)
- Should install-method detection be extracted into a shared package for reuse by future commands, or stay internal to `self-update` until a second consumer appears? (Inherited from the source Idea.)
- Should a later iteration add cryptographic signature verification (cosign/sigstore) on top of the sha256 check? (Idea marks this follow-on.)

---
*This document follows the https://specscore.md/feature-specification*
