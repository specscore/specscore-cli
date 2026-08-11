---
format: https://specscore.md/feature-specification
status: Implementing
---

# Feature: CLI Release Distribution

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/release-distribution?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/release-distribution?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/release-distribution?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/release-distribution?op=request-change) |

**Status:** Implementing
**Source Ideas:** —

## Summary

Publishes release archives, signs and notarizes the Darwin artifact, and
retains dormant Homebrew-cask packaging while blocking every cask install,
upgrade, and runtime-evidence path until an operator explicitly verifies
signing, notarization, and Gatekeeper validation are fail-closed end to end.

## Problem

Homebrew casks quarantine downloaded macOS binaries. An ad-hoc-signed or
unsigned binary fails Gatekeeper assessment for an interactive user even when
it ran fine in a build job. Without fail-closed Developer ID signing, Apple
notarization, and Gatekeeper validation, neither an automated smoke nor a
user-facing cask recommendation can establish that an interactive user can
safely run the installed binary. The release still needs raw archive
validation, and the cask remains dormant packaging metadata, but no cask
install, upgrade, or runtime evidence may run prematurely — and a release must
not claim the cask is installable unless its Darwin artifact is signed with a
real Developer ID and notarized by Apple.

## Behavior

### macOS signed and notarized artifact

#### REQ: macos-signed-and-notarized-artifact

The GoReleaser configuration MUST define a `notarize.macos` entry for the
`specscore` build. It MUST sign with `MACOS_SIGN_P12` and
`MACOS_SIGN_PASSWORD`, then submit and wait for notarization using
`NOTARIZE_ISSUER_ID`, `NOTARIZE_KEY_ID`, and `NOTARIZE_KEY`. The entry MUST be
enabled only when `MACOS_SIGN_P12` is set so local and snapshot builds require
no Apple credential.

### Release credential forwarding

#### REQ: release-secret-forwarding

The release-workflow caller MUST forward these five repository secrets to the
shared release workflow with unchanged names: `MACOS_SIGN_P12`,
`MACOS_SIGN_PASSWORD`, `NOTARIZE_ISSUER_ID`, `NOTARIZE_KEY_ID`, and
`NOTARIZE_KEY`. The workflow MUST NOT log, transform, or persist their values.

### Fail-closed release gate

#### REQ: fail-closed-macos-release-gate

The shared release-workflow call MUST set `require_notarized_macos: true`.
That makes an absent, invalid, improperly signed, or unnotarized published
Darwin artifact fail the release rather than leave a quarantined Homebrew cask
that macOS blocks. This setting may be merged only after an operator confirms
all five named repository secrets exist; that confirmation is operational
evidence, not a value an agent may access or infer.

### Retain cask packaging

#### REQ: retain-homebrew-cask-packaging

GoReleaser MUST continue to publish the SpecScore CLI Homebrew cask. Deferring
the cask-install smoke check MUST NOT remove `homebrew_casks` configuration or
change the cask release channel.

### Block cask use until notarization and Gatekeeper enforcement are proven

#### REQ: block-homebrew-cask-until-verified

**Cask distribution status:** Blocked.

Until the release contract enforces Developer ID signing, macOS notarization,
and Gatekeeper validation fail-closed, SpecScore MUST NOT recommend, install,
upgrade, or collect runtime evidence through the Homebrew cask. Its shared
release-workflow caller MUST explicitly set
`artifact_smoke_test_homebrew_cask: false` and leave raw published-artifact
validation enabled. macOS installation guidance MUST use the temporary current
source-built channel and MUST NOT teach quarantine removal or any Gatekeeper
bypass. Automated or agent evidence MUST pin an exact released tag or merged
commit SHA, verify the resulting build identity, and never use a moving source
branch.

The cask may be restored only by an explicit, reviewed change of this cask
distribution status from **Blocked** to **Verified**, made after the required
signing, notarization, and Gatekeeper evidence is fail-closed in the release
contract.

## Acceptance Criteria

### AC: release-contract-is-explicit

**Requirements:** cli/release-distribution#req:macos-signed-and-notarized-artifact, cli/release-distribution#req:release-secret-forwarding, cli/release-distribution#req:fail-closed-macos-release-gate

**Given** the five named Apple repository secrets have been configured,
**When** the SpecScore CLI release workflow publishes a Darwin artifact,
**Then** GoReleaser signs and notarizes it, and the shared release workflow
fails if the published artifact does not pass the notarization gate.

### AC: cask-packaging-with-blocked-installation

**Requirements:** cli/release-distribution#req:retain-homebrew-cask-packaging, cli/release-distribution#req:block-homebrew-cask-until-verified

**Given** the SpecScore CLI release path does not yet have operator-verified
Gatekeeper validation end-to-end,
**When** it publishes a release,
**Then** GoReleaser retains the dormant Homebrew cask configuration, the raw
release artifact smoke remains enabled, no cask install/upgrade/runtime
evidence runs, and macOS guidance names only the temporary source-build
channel with immutable automated evidence.

## Open Questions

None. The required secret names and fail-closed enforcement contract are
fixed; the remaining operator action is to provision the five values in
repository secrets without exposing them to agents, then separately confirm
Gatekeeper validation before flipping cask distribution status from
**Blocked** to **Verified**.

---
*This document follows the https://specscore.md/feature-specification*
