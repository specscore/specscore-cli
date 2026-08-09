---
format: https://specscore.md/feature-specification
status: Amending
---

# Feature: Self-Update

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/self-update?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/self-update?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/self-update?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/self-update?op=request-change) |

**Status:** Amending
**Source Ideas:** cli-self-update

## Summary

`specscore self-update` (alias `specscore update`) brings a running `specscore`
binary to the latest released version. The behavior is not specified here:
specscore binds the shared
[strongo/selfupdate](https://specscore.studio/app/github.com/strongo/selfupdate/spec/features/self-update?op=explore)
library, whose Feature owns install-method detection, release resolution,
checksum verification, atomic replacement, the pinned-version rules, and every
failure guarantee. This Feature specifies only what is specscore's own — the
command surface, specscore's configuration of the library, and its exit-code
contract.

## Synopsis

```
specscore self-update                         # detect, then self-replace (manual) or redirect (managed)
specscore self-update --check                 # report availability only; never modifies
specscore self-update --yes                   # skip the confirmation prompt (non-interactive)
specscore self-update --version v0.0.3        # install a specific release (manual installs)
specscore self-update --version 0.3.0 --allow-downgrade   # roll back to an older release
specscore update                              # alias for `self-update`
```

## Problem

The CLI is distributed through several channels — a Homebrew tap, a Scoop
bucket, WinGet, and direct GitHub release archives, plus `go install`. Users had
no first-class way to move to the latest version, and the naive fix — overwrite
the binary — is only correct for manual installs.

This Feature originally specified that whole contract itself. It no longer does.
The same rules were needed verbatim by `wb` and are needed by every other Go CLI
in the fleet, so the behavior moved to `github.com/strongo/selfupdate`, where it
is specified and tested once. What remains here is the part that was never
portable: which package managers publish specscore, what its version placeholder
is, and the exit codes its callers branch on — which deliberately differ from
other consumers of the same library.

## Behavior

### Command surface

#### REQ: command-and-alias

The CLI MUST expose the command as `specscore self-update`. `specscore update`
MUST be accepted as an alias that resolves to identical behavior. The canonical
name is `self-update` because, in a CLI dominated by artifact verbs (`idea new`,
`feature new`, `spec lint`), a bare `update` is ambiguous about *what* is
updated.

#### REQ: library-provided-behavior

The command MUST obtain its behavior from `github.com/strongo/selfupdate` rather
than reimplementing it. Install-method detection, stable-release resolution,
version comparison, pinned targets and the downgrade guard, asset download,
sha256 verification before extraction, atomic replacement, the post-swap version
check, the non-interactive refusal, and the guarantee that every failure leaves a
working binary are inherited from that library's Feature and MUST NOT be
restated or reinterpreted here. A behavior change belongs upstream in the
library, not in a specscore-local fork.

#### REQ: flag-surface

The command MUST expose `--check`, `--yes` (short `-y`), `--version <tag>`, and
`--allow-downgrade`, bound to the library's corresponding options. `--version`
here is `self-update`-local and distinct from the root `specscore --version`,
which prints build identity.

### specscore's configuration of the library

#### REQ: specscore-release-identity

specscore MUST configure the library with its own release identity: the GitHub
repository `specscore/specscore-cli`, the binary name `specscore`, and release
assets named as this project's GoReleaser publishes them
(`specscore_<version>_<os>_<arch>` archives with a
`specscore_<version>_checksums.txt` checksums file).

#### REQ: specscore-managers

specscore MUST configure the three package managers that publish it, each with
its exact upgrade command: Homebrew (`brew upgrade specscore`), Scoop
(`scoop update specscore`), and WinGet (`winget upgrade SpecScore.CLI`). An
install detected under any of them MUST follow the library's managed-redirect
path and MUST NOT be overwritten.

#### REQ: specscore-version-identity

specscore MUST supply the build-time version pinned by the
[Version](../version/README.md) feature, and MUST declare `dev` as the string
meaning undetermined — the placeholder reported by a binary built without
`-ldflags`. The post-swap version probe MUST use `--version`.

### Exit codes

#### REQ: exit-code-contract

The command MUST map the library's outcomes and typed failure kinds onto
specscore's own exit codes, which the library does not decide. `0` means
success: the self-replace completed, the redirect was printed, or the binary is
already up to date. `10` is reserved for `--check` reporting that an update is
available or that the current version is undetermined. Operational errors —
ambiguous detection, network or download failure, checksum mismatch, permission
denied, non-interactive without `--yes`, an unknown `--version` tag, or a
refused downgrade — MUST use codes distinct from both `0` and `10`, so an
available update can never be confused with a failure.

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [strongo/selfupdate: Self-Update Library](https://specscore.studio/app/github.com/strongo/selfupdate/spec/features/self-update?op=explore) | Owns the behavior contract this Feature binds. specscore is a consumer; behavior changes belong there. |
| [CLI](../README.md) | Parent feature. Inherits shared CLI conventions and the shared error path. |
| [Version](../version/README.md) | Supplies the running version and the `dev` placeholder this Feature declares as undetermined. |

## Amendment note

Before this amendment, this Feature carried the full behavior contract — 20
acceptance criteria covering detection, download, verification, replacement, and
every failure mode. Those moved to the shared library's Feature, which specifies
and tests them once for all consumers. The `_verify` and `_recap` reports in this
directory are point-in-time records against the earlier acceptance criteria and
are retained as history, not as claims about the criteria below.

## Acceptance Criteria

### AC: canonical-and-alias

**Requirements:** cli/self-update#req:command-and-alias, cli/self-update#req:flag-surface

**Given** an installed `specscore` binary
**When** the user runs `specscore self-update --check` and, separately, `specscore update --check`
**Then** both invocations execute the same command and produce identical output and exit code, and the full flag surface (`--check`, `--yes`/`-y`, `--version`, `--allow-downgrade`) is accepted by both.

### AC: behavior-comes-from-the-library

**Requirements:** cli/self-update#req:library-provided-behavior, cli/self-update#req:specscore-release-identity, cli/self-update#req:specscore-version-identity

**Given** the specscore-cli source tree
**When** the self-update command is built
**Then** detection, release resolution, verification, and replacement come from `github.com/strongo/selfupdate`, specscore supplies only its release identity, version and `dev` placeholder, and no copy of that logic remains in specscore's own tree.

### AC: managed-is-redirected

**Requirements:** cli/self-update#req:specscore-managers

**Given** a `specscore` binary whose resolved path is inside a Homebrew, Scoop, or WinGet layout
**When** the user runs `specscore self-update`, including with `--yes` and with `--version <tag>`
**Then** specscore prints that manager's exact upgrade command, exits `0`, and performs no download, no write, and no replacement.

### AC: check-exit-code-contract

**Requirements:** cli/self-update#req:exit-code-contract

**Given** an up-to-date binary, a binary with a newer release available, and a release lookup that fails
**When** the user runs `specscore self-update --check` in each case
**Then** the exit codes are `0`, `10`, and a code distinct from both, so "an update is available" is never confused with an operational failure.

## Open Questions

- Should a cached availability check surface a stale binary during unrelated
  commands? The question now belongs to the shared library, which carries it for
  every consumer.

---
*This document follows the https://specscore.md/feature-specification*
