---
format: https://specscore.md/scenario-specification
---

# Rehearse: changed-object-new-observation

**Status:** pending
**Verifies:** cli/studio/probe#ac:changed-object-new-observation (REQ: verified-at-field)

Scenario source: [../README.md](../README.md) → `### AC: changed-object-new-observation`.

Given the same store whose prior verified fact was `serves-status` `200` at T1, when the domain probe is stubbed to return a connection error at T2 and I run `specscore studio probe --kind domain` then `specscore studio facts --subject example.app --class verified-behavior --format json`, then a `serves-status` `down` fact exists whose `observed_at` and `verified_at` both equal T2.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:changed-object-new-observation
# Requires: specscore on PATH (override with $SPECSCORE). Uses stubbed HTTP/exec seams.
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: changed-object-new-observation not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
