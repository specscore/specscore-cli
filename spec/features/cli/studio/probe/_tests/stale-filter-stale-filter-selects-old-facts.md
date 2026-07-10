---
format: https://specscore.md/scenario-specification
---

# Rehearse: stale-filter-selects-old-facts

**Status:** pending
**Verifies:** cli/studio/probe#ac:stale-filter-selects-old-facts (REQ: stale-filter)

Scenario source: [../README.md](../README.md) → `### AC: stale-filter-selects-old-facts`.

Given a store containing one verified-behavior fact with `verified_at` 48 hours ago and one with `verified_at` 1 hour ago, when I run `specscore studio facts --class verified-behavior --stale 24h --count`, then the count is 1 (only the 48-hour-old fact).

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:stale-filter-selects-old-facts
# Requires: specscore on PATH (override with $SPECSCORE). Uses stubbed HTTP/exec seams.
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: stale-filter-selects-old-facts not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
