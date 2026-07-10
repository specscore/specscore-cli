---
format: https://specscore.md/scenario-specification
---

# Rehearse: naming-conflict-declared-disagreement

**Status:** pending
**Verifies:** cli/studio/answers#ac:naming-conflict-declared-disagreement (REQ: same-predicate-disagreement)

Scenario source: [../README.md](../README.md) → `### AC: naming-conflict-declared-disagreement`.

Given an indexed store with `(subj, provides, ext-foo)` and `(subj, provides, foo-contract)`, both evidence_class `declared` with different evidence pointers, when I run `specscore studio contradictions --format json`, then the JSON contains a `naming-conflict` item whose two sides are those two facts, and the item is absent when the two facts share the same object.

Uses a fixture store seeded offline (no network).

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:naming-conflict-declared-disagreement
# Requires: specscore on PATH (override with $SPECSCORE).
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: naming-conflict-declared-disagreement not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
