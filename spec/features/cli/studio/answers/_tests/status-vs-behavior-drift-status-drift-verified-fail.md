---
format: https://specscore.md/scenario-specification
---

# Rehearse: status-drift-verified-fail

**Status:** pending
**Verifies:** cli/studio/answers#ac:status-drift-verified-fail (REQ: status-vs-behavior-drift)

Scenario source: [../README.md](../README.md) → `### AC: status-drift-verified-fail`.

Given an indexed store with a `declared` `(<repo>#feat/x, has-status, Stable)` fact and a `verified-behavior` `(<repo>#feat/x#ac:y, has-verification-status, fail)` fact, when I run `specscore studio contradictions --format json`, then the command exits 0 and the JSON contains an item with detector `status-drift` whose two sides are the `has-status` fact and the `has-verification-status` fact, each side carrying its subject, object, evidence_class, evidence_pointer, and observed_at.

Uses a fixture store seeded offline (no network).

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:status-drift-verified-fail
# Requires: specscore on PATH (override with $SPECSCORE).
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: status-drift-verified-fail not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
