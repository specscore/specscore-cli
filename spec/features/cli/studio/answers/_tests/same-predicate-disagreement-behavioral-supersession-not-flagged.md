---
format: https://specscore.md/scenario-specification
---

# Rehearse: behavioral-supersession-not-flagged

**Status:** pending
**Verifies:** cli/studio/answers#ac:behavioral-supersession-not-flagged (REQ: same-predicate-disagreement)

Scenario source: [../README.md](../README.md) → `### AC: behavioral-supersession-not-flagged`.

Given an indexed store with `(d, serves-status, 200)` and `(d, serves-status, down)`, both evidence_class `verified-behavior`, when I run `specscore studio contradictions --format json`, then no contradiction item of any detector references those two facts (differing objects across two behavioral observations are supersession — flagged by neither `naming-conflict` nor `status-drift`).

Uses a fixture store seeded offline (no network).

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:behavioral-supersession-not-flagged
# Requires: specscore on PATH (override with $SPECSCORE).
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: behavioral-supersession-not-flagged not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
