---
format: https://specscore.md/scenario-specification
---

# Rehearse: probe-preserves-index-facts

**Status:** pending
**Verifies:** cli/studio/probe#ac:probe-preserves-index-facts (REQ: probe-merge)

Scenario source: [../README.md](../README.md) → `### AC: probe-preserves-index-facts`.

Given an indexed store with declared and derived facts from `studio index`, when I run `specscore studio probe` and then query facts by any index adapter, then the command exits 0 and the pre-existing `declared` and `derived` facts are still present in the store alongside the new `verified-behavior` facts.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:probe-preserves-index-facts
# Requires: specscore on PATH (override with $SPECSCORE). Uses stubbed HTTP/exec seams.
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: probe-preserves-index-facts not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
