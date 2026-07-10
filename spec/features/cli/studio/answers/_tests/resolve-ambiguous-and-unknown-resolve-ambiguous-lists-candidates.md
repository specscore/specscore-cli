---
format: https://specscore.md/scenario-specification
---

# Rehearse: resolve-ambiguous-lists-candidates

**Status:** pending
**Verifies:** cli/studio/answers#ac:resolve-ambiguous-lists-candidates (REQ: resolve-ambiguous-and-unknown)

Scenario source: [../README.md](../README.md) → `### AC: resolve-ambiguous-lists-candidates`.

Given an indexed store where the name `shared` is an `aliased-as` brand for two different product ids, when I run `specscore studio resolve shared`, then the command exits 5 and lists both candidate canonical ids, each with a citation.

Uses a fixture store seeded offline (no network).

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:resolve-ambiguous-lists-candidates
# Requires: specscore on PATH (override with $SPECSCORE).
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: resolve-ambiguous-lists-candidates not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
