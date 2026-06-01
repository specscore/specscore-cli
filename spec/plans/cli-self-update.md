# Plan: CLI Self-Update

**Status:** Implementing
**Mode:** full
**Source Feature:** cli/self-update
**Date:** 2026-06-01
**Owner:** alex
**Supersedes:** —

## Summary

Implementation plan for the `cli/self-update` Feature — the `specscore self-update` cobra command (alias `update`) with a read-only `--check` mode. Lands install-method detection, package-manager redirect, latest-stable release resolution, checksum-verified atomic self-replace for manual installs, confirmation/`--yes` handling, and explicit failure modes. Eleven tasks covering all 15 ACs in the source Feature, zero deferred.

## Approach

Foundation-first linear decomposition. The cobra surface (Task 1) is registered before any path logic so every later task wires into a live command. The two independent foundations — release resolution (Task 2) and install-method detection (Task 3) — precede the behaviors that compose them. Decision paths come next: managed redirect (Task 4) and ambiguous fallback (Task 5) depend only on detection; `--check` (Task 6) composes detection + release resolution and is fully read-only, so it lands before any code that writes to disk. The write path is built bottom-up: download+verify (Task 7) produces the artifact that atomic-replace (Task 8) consumes, which the confirmation guard (Task 9) wraps. No-op short-circuit (Task 10) and the failure modes (Task 11) close out the safety contract. `**Depends-On:**` lines encode the order so `specstudio:implement` can batch correctly.

No ACs are deferred. All 15 ACs in `cli/self-update` are covered.

## Tasks

### Task 1: Register `self-update` command with `update` alias and flags

**Status:** done
**Depends-On:** —
**Verifies:** cli/self-update#ac:canonical-and-alias

Add `internal/cli/self_update.go` registering the `self-update` cobra command with `Aliases: ["update"]`, wired into the root command. Define the `--check` (bool) and `--yes`/`-y` (bool) flags. `RunE` dispatches to the path logic added in later tasks; in this task it is a thin shell so the canonical name and alias resolve identically. Reject extra positional arguments to keep the call shape stable.

### Task 2: Resolve latest stable release and compare against build version

**Status:** done
**Depends-On:** 1
**Verifies:** cli/self-update#ac:latest-stable-only, cli/self-update#ac:dev-build-is-undetermined

Implement latest-version resolution from the GitHub releases of `specscore/specscore-cli`, selecting only the newest non-prerelease (stable) release and ignoring prereleases. Compare against the build-time `version` value from `internal/cli`. Treat the `dev` placeholder as *undetermined* (not "up to date"). Expose a small result type (current, latest, comparison verdict ∈ {up-to-date, available, undetermined}) consumed by `--check`, the self-replace path, and the no-op short-circuit.

### Task 3: Install-method detection from the resolved executable path

**Status:** done
**Depends-On:** 1
**Verifies:** cli/self-update#ac:manual-is-eligible

Add a pure-function detector that resolves the running executable's path and classifies the install as `managed` (Homebrew Cellar/prefix, Scoop `apps`/`shims`, WinGet `Packages`/`Links`), `manual` (release-archive locations such as `/usr/local/bin` or `~/bin`, and `go install` targets under `GOBIN`/`GOPATH/bin`), or `ambiguous`. Managed results carry which manager was detected. Unit-test classification against representative path strings per OS.

### Task 4: Package-managed redirect path

**Status:** done
**Depends-On:** 3
**Verifies:** cli/self-update#ac:managed-is-redirected

When detection returns `managed`, print the detected manager and its exact upgrade command (`brew upgrade specscore`, `scoop update specscore`, or `winget upgrade SpecScore.CLI`), make no filesystem writes, and exit `0`. This path must be unreachable for the download/replace code.

### Task 5: Ambiguous-detection safe fallback

**Status:** done
**Depends-On:** 3
**Verifies:** cli/self-update#ac:ambiguous-falls-back-safe

When detection returns `ambiguous`, refuse to self-replace, print that the install method is ambiguous along with manual-update guidance, and exit non-zero. Ambiguity must never resolve to the manual self-replace path.

### Task 6: `--check` read-only mode and exit-code contract

**Status:** done
**Depends-On:** 2, 3, 4, 5
**Verifies:** cli/self-update#ac:check-is-readonly, cli/self-update#ac:check-exit-code-contract

Implement `--check`: run detection (Task 3) and release resolution (Task 2), report availability and the appropriate next step (manager command for managed, self-update note for manual), and perform no download or write for any install method. Map exit codes: `0` up-to-date, `10` update-available-or-undetermined, and a third distinct code for operational errors (e.g. release-lookup failure). The available code must not collide with error codes.

### Task 7: Download matching asset and verify sha256 against `checksums.txt`

**Status:** done
**Depends-On:** 2
**Verifies:** cli/self-update#ac:checksum-mismatch-aborts

For the manual path, download the release asset matching host OS/arch from the latest stable release, then verify its sha256 against the release `checksums.txt`. On mismatch — or a missing/unfetchable checksum entry — abort before touching the executable, report the verification failure, and exit non-zero. Use a release-aware updater library if it cleanly consumes the GoReleaser archive + checksum layout (see Feature Open Questions).

### Task 8: Atomic self-replace of the running executable

**Status:** done
**Depends-On:** 7
**Verifies:** cli/self-update#ac:replace-is-atomic

Swap the verified binary into the executable's location atomically (stage + rename), handling replacement of a running executable on Windows. An interrupted or failed swap must leave a complete, runnable binary in place — never a partial or truncated file. After a successful swap, sanity-check that the new binary reports the expected version.

### Task 9: Confirmation prompt with `--yes` and non-interactive guard

**Status:** pending
**Depends-On:** 8
**Verifies:** cli/self-update#ac:confirm-prompt-and-yes, cli/self-update#ac:noninteractive-without-yes-refuses

Before invoking the swap (Task 8) on a manual install, print the `<current> → <latest>` transition and require interactive confirmation. `--yes`/`-y` skips the prompt. When not attached to an interactive terminal and `--yes` was not given, refuse to replace and exit non-zero rather than blocking on input.

### Task 10: Already-current no-op short-circuit

**Status:** pending
**Depends-On:** 2
**Verifies:** cli/self-update#ac:already-current-noop

When the resolved comparison (Task 2) reports the running version already equals the latest stable release, report up-to-date and exit `0` without downloading or replacing anything. This short-circuits before the download path (Task 7).

### Task 11: Network and permission failure modes

**Status:** pending
**Depends-On:** 7, 8
**Verifies:** cli/self-update#ac:network-failure-is-safe, cli/self-update#ac:permission-denied-is-safe

Handle the two write-path failure modes explicitly: a release lookup / asset download that fails (network error, rate limit, missing asset) prints a clear error, exits non-zero, and modifies nothing; a permission-denied replacement reports the failure with the executable path and a suggested remedy (elevated permissions or the package manager), exits non-zero, and leaves the original binary intact.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
