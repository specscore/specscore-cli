---
format: https://specscore.md/feature-specification
status: Implementing
---

# Feature: CLI Release Distribution

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/release-distribution?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/release-distribution?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/release-distribution?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/release-distribution?op=request-change) |

**Status:** Implementing
**Source Ideas:** —

## Summary

Publishes SpecScore CLI release archives and a Homebrew cask while keeping the
cask-install smoke check deferred until macOS notarization is enforced.

## Problem

Homebrew casks quarantine downloaded macOS binaries. Before the release path
enforces Developer ID signing and Apple notarization, an automated cask install
cannot establish that an interactive user can run the installed binary. The
release still needs its raw archive validation, and the cask must remain
packaged for its distribution channel, but the cask-install smoke job must not
run prematurely.

## Behavior

### Retain cask packaging

#### REQ: retain-homebrew-cask-packaging

GoReleaser MUST continue to publish the SpecScore CLI Homebrew cask. Deferring
the cask-install smoke check MUST NOT remove `homebrew_casks` configuration or
change the cask release channel.

### Defer cask install smoke until notarization enforcement

#### REQ: defer-cask-install-smoke

While the shared release workflow does not enforce macOS notarization with
`require_notarized_macos: true`, its SpecScore CLI caller MUST explicitly set
`artifact_smoke_test_homebrew_cask: false`. The caller MUST leave raw published
artifact validation enabled. Enabling the cask-install smoke check is deferred
until the same release contract enforces notarization fail-closed.

## Acceptance Criteria

### AC: cask-packaging-with-deferred-install-smoke

**Requirements:** cli/release-distribution#req:retain-homebrew-cask-packaging, cli/release-distribution#req:defer-cask-install-smoke

**Given** the SpecScore CLI release path does not yet enforce macOS
notarization fail-closed,
**When** it publishes a release,
**Then** GoReleaser retains the Homebrew cask configuration, the raw release
artifact smoke remains enabled, and the reusable release caller skips the
Homebrew-cask installation smoke layer.

## Open Questions

None. The cask-install smoke layer is re-enabled only with an explicit
fail-closed notarization enforcement change.

---
*This document follows the https://specscore.md/feature-specification*
