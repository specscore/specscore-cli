---
format: https://specscore.md/scenario-specification
---

# Rehearse: ask-routes-with-citations

**Status:** pending
**Verifies:** cli/studio/answers#ac:ask-routes-with-citations (REQ: ask-verb-and-router)

Scenario source: [../README.md](../README.md) → `### AC: ask-routes-with-citations`.

Given an indexed store with `(acme.app, fronts, acme)` declared, when I run `specscore studio ask "who fronts acme.app" --format json`, then the command exits 0, the JSON `template` is `who-fronts`, the `answer` names `acme`, and `citations` is a non-empty array whose entry names the `fronts` fact's evidence_pointer and adapter id.

Uses a fixture store seeded offline (no network).

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:ask-routes-with-citations
# Requires: specscore on PATH (override with $SPECSCORE).
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: ask-routes-with-citations not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
