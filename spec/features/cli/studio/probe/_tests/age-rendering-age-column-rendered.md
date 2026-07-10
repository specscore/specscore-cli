---
format: https://specscore.md/scenario-specification
---

# Rehearse: age-column-rendered

**Status:** pending
**Verifies:** cli/studio/probe#ac:age-column-rendered (REQ: age-rendering)

Scenario source: [../README.md](../README.md) → `### AC: age-column-rendered`.

Given a store containing a verified-behavior fact whose `verified_at` is 3 hours ago, when I run `specscore studio facts --class verified-behavior`, then the table includes a `VERIFIED` column and the row shows a human age (e.g. `3h`) for that fact.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:age-column-rendered
# Requires: specscore on PATH (override with $SPECSCORE). Uses stubbed HTTP/exec seams.
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: age-column-rendered not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
