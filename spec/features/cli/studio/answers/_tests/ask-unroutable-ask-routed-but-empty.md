---
format: https://specscore.md/scenario-specification
---

# Rehearse: ask-routed-but-empty

**Status:** pending
**Verifies:** cli/studio/answers#ac:ask-routed-but-empty (REQ: ask-unroutable)

Scenario source: [../README.md](../README.md) → `### AC: ask-routed-but-empty`.

Given an indexed store with no `fronts` fact for `unknown.example`, when I run `specscore studio ask "who fronts unknown.example"`, then the command exits 3 with a message distinguishing routed-but-no-data from unroutable, and emits no citation-free answer.

Uses a fixture store seeded offline (no network).

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:ask-routed-but-empty
# Requires: specscore on PATH (override with $SPECSCORE).
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: ask-routed-but-empty not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
