---
format: https://specscore.md/scenario-specification
---

# Rehearse: cadences-in-help

**Status:** pending
**Verifies:** cli/studio/probe#ac:cadences-in-help (REQ: freshness-cadences)

Scenario source: [../README.md](../README.md) → `### AC: cadences-in-help`.

Given the specscore CLI, when I run `specscore studio probe --help`, then the help text names a re-verification cadence for each of `verified-behavior`, `derived`, `declared`, `claimed`, and `attested`.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:cadences-in-help
# Requires: specscore on PATH (override with $SPECSCORE). Uses stubbed HTTP/exec seams.
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: cadences-in-help not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
