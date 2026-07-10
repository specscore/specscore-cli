---
format: https://specscore.md/scenario-specification
---

# Rehearse: gh-absent-skips-ci

**Status:** pending
**Verifies:** cli/studio/probe#ac:gh-absent-skips-ci (REQ: ci-state)

Scenario source: [../README.md](../README.md) → `### AC: gh-absent-skips-ci`.

Given an indexed store and no `gh` binary on `PATH`, when I run `specscore studio probe --kind ci`, then the command exits 0 and the run summary reports one warning that the `ci` kind was skipped because `gh` was not found, and no `ci-status` facts are written.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:gh-absent-skips-ci
# Requires: specscore on PATH (override with $SPECSCORE). Uses stubbed HTTP/exec seams.
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: gh-absent-skips-ci not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
