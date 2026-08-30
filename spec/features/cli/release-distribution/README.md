---
format: https://specscore.md/feature-specification
status: Implementing
---

# Feature: CLI Release Distribution

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/release-distribution?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/release-distribution?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/release-distribution?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/release-distribution?op=request-change) |

**Status:** Implementing
**Source Ideas:** —

## Summary

Publishes release archives, signs and notarizes the Darwin artifact, and now
recommends the Homebrew cask as a supported macOS installation and upgrade
channel, on the strength of operator-verified signing, notarization, and
Gatekeeper validation running fail-closed end to end.

## Problem

Homebrew casks quarantine downloaded macOS binaries. An ad-hoc-signed or
unsigned binary fails Gatekeeper assessment for an interactive user even when
it ran fine in a build job. Without fail-closed Developer ID signing, Apple
notarization, and Gatekeeper validation, neither an automated smoke nor a
user-facing cask recommendation can establish that an interactive user can
safely run the installed binary. The release needs raw archive validation
plus verified evidence that a Gatekeeper-cleared Developer ID artifact reaches
the cask before a release may claim the cask is installable.

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
evidence, not a value an agent may access or infer. **That confirmation has
occurred**: the operator confirmed all five named secrets exist, with fresh
update timestamps, in every org secret store that needs them, and the setting
is merged.

### Retain cask packaging

#### REQ: retain-homebrew-cask-packaging

GoReleaser MUST continue to publish the SpecScore CLI Homebrew cask. Verifying
the cask-install smoke check MUST NOT remove `homebrew_casks` configuration or
change the cask release channel.

### Cask use verified against fail-closed notarization and Gatekeeper enforcement

#### REQ: block-homebrew-cask-until-verified

**Cask distribution status:** Verified.

The release contract now enforces Developer ID signing, macOS notarization,
and Gatekeeper validation fail-closed (`require_notarized_macos: true`, above),
so SpecScore MAY recommend, install, upgrade, and collect runtime evidence
through the Homebrew cask. This was verified on a real published artifact —
`ingitdb/ingitdb-cli` `v0.65.11` (built with `toolchain go1.27.0`) satisfied its
Designated Requirement (`certificate 1[...6.2.6]`, not the broken
`certificate root[...6.2.6]` produced by a p12 lacking the Apple Root CA),
`spctl` accepted it as Notarized Developer ID, it executed successfully, and
that release's own `Smoke test Homebrew cask install (darwin/arm64)` job
passed — plus `require_notarized_macos: true` making the gate fail-closed
against a future regression. Its shared release-workflow caller MUST
explicitly set `artifact_smoke_test_homebrew_cask: true` and leave raw
published-artifact validation enabled. macOS installation guidance MUST
recommend the Homebrew cask as a supported channel and MUST NOT teach
quarantine removal or any Gatekeeper bypass. Automated or agent evidence MUST
pin an exact released tag or merged commit SHA, verify the resulting build
identity, and never use a moving source branch.

The cask distribution status may be reverted from **Verified** back to
**Blocked** only by an explicit, reviewed change, made if signing,
notarization, or Gatekeeper evidence in the release contract is found to have
regressed.

## Acceptance Criteria

### AC: release-contract-is-explicit

**Requirements:** cli/release-distribution#req:macos-signed-and-notarized-artifact, cli/release-distribution#req:release-secret-forwarding, cli/release-distribution#req:fail-closed-macos-release-gate

**Given** the five named Apple repository secrets have been configured,
**When** the SpecScore CLI release workflow publishes a Darwin artifact,
**Then** GoReleaser signs and notarizes it, and the shared release workflow
fails if the published artifact does not pass the notarization gate.

### AC: cask-packaging-with-blocked-installation

**Requirements:** cli/release-distribution#req:retain-homebrew-cask-packaging, cli/release-distribution#req:block-homebrew-cask-until-verified

**Given** the SpecScore CLI release path now has operator-verified Gatekeeper
validation end-to-end,
**When** it publishes a release,
**Then** GoReleaser retains the Homebrew cask configuration, the raw release
artifact smoke remains enabled, cask install/upgrade/runtime evidence runs via
`artifact_smoke_test_homebrew_cask: true`, and macOS guidance recommends the
Homebrew cask with immutable automated evidence.

## Open Questions

None. The required secret names and fail-closed enforcement contract are
fixed; the operator has provisioned the five values in repository secrets and
confirmed Gatekeeper validation, and cask distribution status is flipped from
**Blocked** to **Verified**.

---
*This document follows the https://specscore.md/feature-specification*
