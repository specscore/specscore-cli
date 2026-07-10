---
format: https://specscore.md/scenario-specification
---

# Rehearse: agreement-not-flagged

**Status:** pending
**Verifies:** cli/studio/answers#ac:agreement-not-flagged (REQ: same-predicate-disagreement)

Scenario source: [../README.md](../README.md) → `### AC: agreement-not-flagged`.

Given an indexed store where two `declared` facts share subject, predicate, and object but come from different evidence pointers, and no other disagreement exists, when I run `specscore studio contradictions --format json`, then the command exits 0 and the JSON is an empty array.

Uses a fixture store seeded offline (no network).

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:agreement-not-flagged
# Requires: specscore on PATH (override with $SPECSCORE).
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: agreement-not-flagged not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
