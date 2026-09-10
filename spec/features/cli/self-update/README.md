---
format: https://specscore.md/feature-specification
status: Stable
---

# Feature: Self-Update

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/self-update?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/self-update?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/self-update?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/self-update?op=request-change) |

**Status:** Stable
**Source Ideas:** cli-self-update

## Summary

`specscore self-update` (alias `specscore update`) brings a running `specscore`
binary to the latest released version, regardless of how it was installed. The
behavior is not specified here: specscore binds the shared
[strongo/cli-helpers](https://specscore.studio/app/github.com/strongo/cli-helpers/spec/features/self-update?op=explore)
library (published as `github.com/strongo/cli-helpers/selfupdate` — the module
was renamed and re-homed from its original `github.com/strongo/selfupdate`
path; the behavior contract carried over unchanged), whose Feature owns
install-method detection, release resolution, checksum verification, atomic
replacement, executable package-manager upgrades, the pinned-version rules,
and every failure guarantee. This Feature specifies only what is specscore's
own — the command surface, specscore's configuration of the library, and its
exit-code contract.

## Synopsis

```
specscore self-update                         # detect, then self-replace (manual) or upgrade through its manager (managed)
specscore self-update --check                 # report availability and the next step only; never modifies
specscore self-update --yes                   # skip the confirmation prompt (non-interactive)
specscore self-update --version v0.0.3        # install a specific release (manual installs; refused for a managed install the manager can't pin)
specscore self-update --version 0.3.0 --allow-downgrade   # roll back to an older release (manual installs)
specscore update                              # alias for `self-update`
```

## Problem

The CLI is distributed through several channels — a Homebrew tap, a Scoop
bucket, WinGet, and direct GitHub release archives, plus `go install`. Users had
no first-class way to move to the latest version, and the naive fix — overwrite
the binary — is only correct for manual installs; a package-manager-owned
install must go through its manager instead, or the manager's own bookkeeping
falls out of sync with the file on disk.

This Feature originally specified that whole contract itself. It no longer does.
The same rules were needed verbatim by `wb` and are needed by every other Go CLI
in the fleet, so the behavior moved to `github.com/strongo/cli-helpers/selfupdate`
(née `github.com/strongo/selfupdate`), where it is specified and tested once.
What remains here is the part that was never portable: which package managers
publish specscore, what its version placeholder is, and the exit codes its
callers branch on — which deliberately differ from other consumers of the same
library.

Printing a manager's upgrade command instead of running it shipped a real bug:
specscore is distributed as a Homebrew **cask**, so the once-printed
`brew upgrade specscore` told Homebrew to look for a *formula* named
`specscore`, which fails with "specscore/tap/specscore not installed" — only
`brew upgrade --cask specscore` actually upgrades it. Running the manager
directly, rather than asking the user to transcribe a command, closes that gap
by construction: there is no separately-printed string that can drift from the
one specscore itself was configured to run.

## Behavior

### Command surface

#### REQ: command-and-alias

The CLI MUST expose the command as `specscore self-update`. `specscore update`
MUST be accepted as an alias that resolves to identical behavior. The canonical
name is `self-update` because, in a CLI dominated by artifact verbs (`idea new`,
`feature new`, `spec lint`), a bare `update` is ambiguous about *what* is
updated.

#### REQ: library-provided-behavior

The command MUST obtain its behavior from `github.com/strongo/cli-helpers/selfupdate`
rather than reimplementing it. Install-method detection, stable-release
resolution, version comparison, pinned targets and the downgrade guard, asset
download, sha256 verification before extraction, atomic replacement, the
post-swap version check, the non-interactive refusal, the executable
package-manager runner and its own post-upgrade verification, and the
guarantee that every failure leaves a working binary are inherited from that
library's Feature and MUST NOT be restated or reinterpreted here. specscore
MUST NOT hand-roll its own process-execution or package-manager-invocation
code: every manager command runs through the library's own runner, never a
specscore-local `exec.Command` call or shell string. A behavior change belongs
upstream in the library, not in a specscore-local fork.

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
its exact upgrade command run as structured argv (never a shell string):
Homebrew (`brew upgrade --cask specscore` — specscore ships as a cask, not a
formula), Scoop (`scoop update specscore`), and WinGet
(`winget upgrade SpecScore.CLI`). An install detected under any of them MUST be
upgraded by specscore actually RUNNING that manager's exact command — not
merely printing it for the user to type. specscore itself MUST NOT download or
write the managed binary at any point; the manager remains the install's sole
authority throughout.

Before running the command, self-update MUST ask for confirmation, exactly as
it does before a manual self-replace: `--yes` skips the prompt, and a
non-interactive invocation without `--yes` MUST refuse rather than run the
manager unattended. After the manager's command completes, self-update MUST
verify the installed binary reports the expected version
(cli/self-update#req:specscore-version-identity). `--check` MUST continue to
only report the manager and its command, never invoke it.

A `--version` pin that the manager cannot guarantee — because it is
configured as redirect-only, or because the requested version is not that
manager's own latest published release — MUST be refused with a clear error
rather than silently applying the wrong version or falling back to specscore's
own download-and-replace path.

#### REQ: specscore-version-identity

specscore MUST supply the build-time version pinned by the
[Version](../version/README.md) feature, and MUST declare `dev` as the string
meaning undetermined — the placeholder reported by a binary built without
`-ldflags`. The post-swap version probe MUST use `--version`.

### Exit codes

#### REQ: exit-code-contract

The command MUST map the library's outcomes and typed failure kinds onto
specscore's own exit codes, which the library does not decide. `0` means
success: the self-replace completed, the managed install's manager command ran
successfully, or the binary is already up to date. `10` is reserved for
`--check` reporting that an update is available or that the current version is
undetermined. Operational errors — ambiguous detection, network or download
failure, checksum mismatch, permission denied, non-interactive without
`--yes`, an unknown `--version` tag, a refused downgrade, a `--version` pin a
manager cannot honor, or the manager's own upgrade command failing — MUST use
codes distinct from both `0` and `10`, so an available update can never be
confused with a failure.

## Interaction with Other Features

| Feature | Interaction |
|---|---|
| [strongo/cli-helpers: Self-Update Library](https://specscore.studio/app/github.com/strongo/cli-helpers/spec/features/self-update?op=explore) | Owns the behavior contract this Feature binds. specscore is a consumer; behavior changes belong there. |
| [CLI](../README.md) | Parent feature. Inherits shared CLI conventions and the shared error path. |
| [Version](../version/README.md) | Supplies the running version and the `dev` placeholder this Feature declares as undetermined. |

## Amendment note

Before the first amendment, this Feature carried the full behavior contract —
20 acceptance criteria covering detection, download, verification, replacement,
and every failure mode. Those moved to the shared library's Feature, which
specifies and tests them once for all consumers. The `_verify` and `_recap`
reports in this directory are point-in-time records against that earlier
acceptance criteria and are retained as history, not as claims about the
criteria below.

A second amendment (2026-09) changed what "managed" means in practice. The
library was renamed and re-homed from `github.com/strongo/selfupdate` to
`github.com/strongo/cli-helpers/selfupdate` (same behavior contract, new
import path), and its new executable-manager mode lets specscore actually run
a package manager's upgrade command instead of only printing it — closing a
real bug where the previously-printed Homebrew command
(`brew upgrade specscore`) failed against specscore's actual cask install. The
AC that specified the old print-only behavior, `managed-is-redirected`, was
replaced by `managed-is-upgraded-through-its-manager` below; nothing in the
`_verify`/`_recap` history above was rewritten to match, for the same reason
the first amendment's history wasn't.

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
**Then** detection, release resolution, verification, replacement, and executable package-manager upgrades come from `github.com/strongo/cli-helpers/selfupdate`, specscore supplies only its release identity, version and `dev` placeholder, and no copy of that logic (including no specscore-local process execution) remains in specscore's own tree.

### AC: managed-is-upgraded-through-its-manager

**Requirements:** cli/self-update#req:specscore-managers

**Given** a `specscore` binary whose resolved path is inside a Homebrew, Scoop, or WinGet layout, and a newer release available
**When** the user runs `specscore self-update`
**Then** specscore asks for confirmation naming that manager and its exact upgrade command, and — once confirmed, or immediately with `--yes` — runs that exact command (as argv, never a shell string), verifies the resulting binary reports the expected version, and exits `0`, at no point downloading or writing the managed binary itself.

**Given** the same managed install
**When** the user runs `specscore self-update` without `--yes` and without an interactive terminal attached
**Then** specscore refuses with a non-zero exit distinct from `0` and `10`, and never runs the manager unattended.

**Given** the same managed install
**When** the user runs `specscore self-update --version <tag>` for a tag that is not the manager's own latest published release, or whose manager is not configured for executable upgrades
**Then** specscore refuses with a clear error and a non-zero exit distinct from `0` and `10`, without running the manager or falling back to a download.

**Given** the same managed install
**When** the user runs `specscore self-update --check`
**Then** specscore reports the manager and its exact upgrade command as the next step without running it, exactly as before this amendment.

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
