---
format: https://specscore.md/scenario-specification
---

# Rehearse: probe-without-index-errors

**Status:** pending
**Verifies:** cli/studio/probe#ac:probe-without-index-errors (REQ: probe-verb)

Scenario source: [../README.md](../README.md) → `### AC: probe-without-index-errors`.

Given a workspace directory where `studio index` has never run, when I run `specscore studio probe`, then the command exits 2 with a message naming the expected store path and suggesting `specscore studio index`.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:probe-without-index-errors
# Requires: specscore on PATH (override with $SPECSCORE). Uses stubbed HTTP/exec seams.
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: probe-without-index-errors not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
