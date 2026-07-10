---
format: https://specscore.md/scenario-specification
---

# Rehearse: stale-filter-malformed-duration

**Status:** pending
**Verifies:** cli/studio/probe#ac:stale-filter-malformed-duration (REQ: stale-filter)

Scenario source: [../README.md](../README.md) → `### AC: stale-filter-malformed-duration`.

Given any indexed store, when I run `specscore studio facts --stale notaduration`, then the command exits 2 with a message naming the invalid duration.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:stale-filter-malformed-duration
# Requires: specscore on PATH (override with $SPECSCORE). Uses stubbed HTTP/exec seams.
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: stale-filter-malformed-duration not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
