---
format: https://specscore.md/idea-specification
status: Specified
---

# Idea: CLI Self-Update

**Status:** Specified
**Date:** 2026-06-01
**Owner:** alex
**Promotes To:** cli/self-update
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

How might we let specscore-cli check for updates and update itself when appropriate, while detecting package-manager installs and redirecting those users to the correct upgrade path instead of replacing managed binaries?

## Context

SpecScore CLI now has enough breadth that users will expect a first-class update path. There are established Go libraries for release-aware self-update, but blindly replacing the executable is only correct for manual installs. Package-managed installs such as Homebrew, Scoop, Nix, apt, or similar should usually be detected and routed to the package manager update command instead. The install-method detection boundary is therefore part of the product, not an implementation detail.

## Recommended Direction

Add a specscore update surface that first detects the install method, then chooses one of two paths. For unmanaged or manual installs (release-archive downloads and `go install`), the CLI performs in-place binary replacement using a release-aware updater library, verifying the downloaded artifact against the release `checksums.txt` before atomically swapping the executable. For package-managed installs (Homebrew, Scoop, WinGet), the CLI must not overwrite the executable; it reports the detected manager and prints the exact upgrade command the user should run.

Treat install-method detection as a first-class contract that must serve both paths equally well. The command should prefer explicit signals such as known installation prefixes (Homebrew Cellar, Scoop shims, WinGet links), package-manager metadata, and executable-path heuristics over guesswork. When detection is ambiguous, default to the safe path: do not self-replace, surface the ambiguity, and print manual-update guidance.

The command also supports a check-only mode (e.g. `specscore update --check`) that runs the version check and install-method detection, reports whether a newer version is available and the appropriate next step, and exits without modifying anything. This is the primary interaction for package-managed users — who cannot self-update regardless — and gives CI a scriptable staleness signal.

Keep the MVP focused: version check, install-method detection, safe decisioning, the check-only mode, package-manager redirect guidance, and a working library-based self-replace path for manual installs with sha256 checksum verification against the release `checksums.txt`. Cryptographic signature verification (cosign/sigstore), rollback, channel pinning, and background auto-update are follow-on scope unless the chosen library and release pipeline make them nearly free.

## Alternatives Considered

**Detect-and-guide only (no self-replace).** The smallest, safest MVP: print the correct upgrade command for *every* install method, including manual ones, and never touch the binary. Lost because the headline success criterion is a manual-install user reaching the latest version in one step — telling them to go re-download a tarball is exactly the friction this command exists to remove.

**Roll our own GitHub-release fetch and atomic-replace.** Query the releases API, download the matching archive, verify the checksum, and swap the file ourselves with no new dependency. Lost for the MVP because a maintained release-aware updater library already encapsulates the per-OS atomic-replace and rollback edge cases (Windows file-lock semantics, in-place replacement of a running binary) that are easy to get subtly wrong. Revisit only if the library cannot consume GoReleaser's archive/checksum layout.

**Always self-replace regardless of install method.** Treat the binary as ours to overwrite everywhere. Lost because it corrupts package-manager bookkeeping: a Homebrew/Scoop/WinGet user would end up with a binary the manager no longer tracks, breaking their next `brew upgrade` and leaving two notions of "installed version." Detection-then-redirect is non-negotiable, which is why the detection contract is part of the product.

## MVP Scope

A timeboxed `specscore update` command that, end-to-end, lets a manual-install user run one command and land on the latest version: detect the install method, redirect package-managed installs to the correct manager command, and for manual installs download the matching release archive, verify it against `checksums.txt`, and atomically self-replace via a release-aware library. A `--check` mode reports update availability and the right next step for any install method without modifying anything. Ambiguous detection falls back to safe manual guidance. Success is measured against the manual self-update path working on macOS, Linux, and Windows — not just against correct decisioning.

## Not Doing (and Why)

- Background auto-update daemon — updates run only when the user invokes the command
- Overwriting package-managed binaries — managed installs should be redirected to the package manager
- Multiple release channels at launch — stable-only until release management needs more
- Silent self-replacement on ambiguous detection — ambiguity falls back to manual guidance
- Rolling our own download/atomic-replace logic — a maintained release-aware library owns the per-OS edge cases for the MVP
- Cryptographic signature verification (cosign/sigstore) — MVP verifies the sha256 `checksums.txt` only; signing is follow-on
- Full dry-run that downloads and verifies but discards — `--check` reports availability only; a stage-and-discard rehearsal of the real swap is follow-on

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | Managed installs (Homebrew, Scoop, WinGet) are reliably distinguishable from manual installs using path/prefix and metadata signals, with no false "manual" that would overwrite a managed binary. | Install via each channel on its target OS; assert detection classifies every one as managed and a downloaded tarball / `go install` as manual. |
| Must-be-true | A release-aware Go updater library can atomically replace the *running* specscore binary on macOS, Linux, and Windows given GoReleaser's archive layout. | Spike the library against a real published release archive on each OS; confirm the swapped binary runs and reports the new version. |
| Should-be-true | The per-release `checksums.txt` is fetchable and parseable to verify the downloaded archive before replacement. | Fetch a real release's `checksums.txt`, match the archive's sha256, and assert a tampered archive is rejected. |
| Should-be-true | Users want a built-in `specscore update` rather than always deferring to their package manager. | Lightweight signal: existing usage telemetry / issue requests; revisit if managed installs dominate and rarely hit the manual path. |
| Might-be-true | Install-method detection is reusable enough to justify a shared package (consumed by `version`, telemetry, future commands). | Sketch a second concrete consumer; if only `update` needs it, inline the logic instead of extracting a package. |


## SpecScore Integration

- **New Features this would create:** TBD at design time
- **Existing Features affected:** none
- **Dependencies:** none

## Open Questions

- Which release-aware updater library do we adopt (e.g. `minio/selfupdate`, `creativeprojects/go-selfupdate`), and does it consume GoReleaser's archive + `checksums.txt` layout without custom glue?
- Does install-method detection ship as a shared, reusable package now, or stay inlined in the `update` command until a second consumer exists? (See the Might-be-true assumption.)
- What exit-code contract should `--check` expose for scripting/CI (e.g. 0 = up to date, distinct non-zero = update available), and does that conflict with normal error-code conventions?

---
*This document follows the https://specscore.md/idea-specification*
