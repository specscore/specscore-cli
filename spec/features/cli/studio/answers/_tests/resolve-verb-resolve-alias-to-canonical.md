---
format: https://specscore.md/scenario-specification
---

# Rehearse: resolve-alias-to-canonical

**Status:** pending
**Verifies:** cli/studio/answers#ac:resolve-alias-to-canonical (REQ: resolve-verb)

Scenario source: [../README.md](../README.md) → `### AC: resolve-alias-to-canonical`.

Given an indexed store containing the `declared` fact `(sizeus, aliased-as, SizeChart)`, when I run `specscore studio resolve SizeChart`, then the command exits 0 and prints `sizeus`, and `specscore studio resolve sizechart` (different case) resolves identically.

Uses a fixture store seeded offline (no network).

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:resolve-alias-to-canonical
# Requires: specscore on PATH (override with $SPECSCORE).
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: resolve-alias-to-canonical not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
