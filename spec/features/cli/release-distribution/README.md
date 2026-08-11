---
format: https://specscore.md/feature-specification
status: Implementing
---

# Feature: CLI Release Distribution

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/release-distribution?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/release-distribution?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/release-distribution?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/release-distribution?op=request-change) |

**Status:** Implementing
**Source Ideas:** —

## Summary

Publishes release archives and retains dormant Homebrew-cask packaging while
blocking every cask install, upgrade, and runtime-evidence path until macOS
signing, notarization, and Gatekeeper validation are fail-closed.

## Problem

Homebrew casks quarantine downloaded macOS binaries. Without fail-closed
Developer ID signing, Apple notarization, and Gatekeeper validation, neither an
automated smoke nor a user-facing cask recommendation can establish that an
interactive user can safely run the installed binary. The release still needs
raw archive validation, and the cask remains dormant packaging metadata, but
no cask install, upgrade, or runtime evidence may run prematurely.

## Behavior

### Retain cask packaging

#### REQ: retain-homebrew-cask-packaging

GoReleaser MUST continue to publish the SpecScore CLI Homebrew cask. Deferring
the cask-install smoke check MUST NOT remove `homebrew_casks` configuration or
change the cask release channel.

### Block cask use until notarization and Gatekeeper enforcement

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

### AC: cask-packaging-with-blocked-installation

**Requirements:** cli/release-distribution#req:retain-homebrew-cask-packaging, cli/release-distribution#req:block-homebrew-cask-until-verified

**Given** the SpecScore CLI release path does not yet enforce fail-closed
signing, notarization, and Gatekeeper validation,
**When** it publishes a release,
**Then** GoReleaser retains the dormant Homebrew cask configuration, the raw
release artifact smoke remains enabled, no cask install/upgrade/runtime
evidence runs, and macOS guidance names only the temporary source-build
channel with immutable automated evidence.

## Open Questions

None. Cask use is restored only with the explicit **Blocked** → **Verified**
distribution-status change after fail-closed signing, notarization, and
Gatekeeper validation are present.

---
*This document follows the https://specscore.md/feature-specification*
