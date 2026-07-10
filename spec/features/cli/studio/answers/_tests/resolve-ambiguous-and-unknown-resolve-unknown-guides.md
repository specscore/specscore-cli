---
format: https://specscore.md/scenario-specification
---

# Rehearse: resolve-unknown-guides

**Status:** pending
**Verifies:** cli/studio/answers#ac:resolve-unknown-guides (REQ: resolve-ambiguous-and-unknown)

Scenario source: [../README.md](../README.md) → `### AC: resolve-unknown-guides`.

Given any indexed store not containing the name `nonexistent`, when I run `specscore studio resolve nonexistent`, then the command exits 3 with a message naming what was searched (aliases and entity ids) and suggesting a `studio facts` exploration.

Uses a fixture store seeded offline (no network).

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:resolve-unknown-guides
# Requires: specscore on PATH (override with $SPECSCORE).
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: resolve-unknown-guides not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
