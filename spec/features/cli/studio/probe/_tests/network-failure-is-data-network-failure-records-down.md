---
format: https://specscore.md/scenario-specification
---

# Rehearse: network-failure-records-down

**Status:** pending
**Verifies:** cli/studio/probe#ac:network-failure-records-down (REQ: network-failure-is-data)

Scenario source: [../README.md](../README.md) → `### AC: network-failure-records-down`.

Given an indexed store with a `serves-status` fact for domain `dead.example`, and a domain probe stubbed to return a connection error for `https://dead.example/`, when I run `specscore studio probe --kind domain` and then `specscore studio facts --subject dead.example --predicate serves-status --class verified-behavior --format json`, then the command exits 0 and the JSON contains a fact with object `down` and evidence_class `verified-behavior`.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:network-failure-records-down
# Requires: specscore on PATH (override with $SPECSCORE). Uses stubbed HTTP/exec seams.
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: network-failure-records-down not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
