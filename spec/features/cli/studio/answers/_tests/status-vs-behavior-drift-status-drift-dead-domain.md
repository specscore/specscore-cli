---
format: https://specscore.md/scenario-specification
---

# Rehearse: status-drift-dead-domain

**Status:** pending
**Verifies:** cli/studio/answers#ac:status-drift-dead-domain (REQ: status-vs-behavior-drift)

Scenario source: [../README.md](../README.md) → `### AC: status-drift-dead-domain`.

Given an indexed store with `(dead.example, serves-status, 200)` with evidence_class `declared` (pointer `domains.json`) and `(dead.example, serves-status, down)` with evidence_class `verified-behavior` (the probe fact), when I run `specscore studio contradictions --format json`, then the JSON contains a `status-drift` item whose two sides are the declared `200` fact and the verified `down` fact, each side carrying its evidence_class, evidence_pointer, and observed_at.

Uses a fixture store seeded offline (no network).

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:status-drift-dead-domain
# Requires: specscore on PATH (override with $SPECSCORE).
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: status-drift-dead-domain not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
