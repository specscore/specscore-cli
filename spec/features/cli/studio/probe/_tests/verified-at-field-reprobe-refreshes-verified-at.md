---
format: https://specscore.md/scenario-specification
---

# Rehearse: reprobe-refreshes-verified-at

**Status:** pending
**Verifies:** cli/studio/probe#ac:reprobe-refreshes-verified-at (REQ: verified-at-field)

Scenario source: [../README.md](../README.md) → `### AC: reprobe-refreshes-verified-at`.

Given a store already probed at time T1 with a `serves-status` `200` verified-behavior fact for `example.app` whose `observed_at` and `verified_at` both equal T1, when the domain probe is stubbed to again return 200 at a later time T2 and I run `specscore studio probe --kind domain` then `specscore studio facts --subject example.app --class verified-behavior --format json`, then the fact's `observed_at` is still T1 and its `verified_at` is T2.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:reprobe-refreshes-verified-at
# Requires: specscore on PATH (override with $SPECSCORE). Uses stubbed HTTP/exec seams.
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: reprobe-refreshes-verified-at not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
