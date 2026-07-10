---
format: https://specscore.md/scenario-specification
---

# Rehearse: contradictions-without-index-errors

**Status:** pending
**Verifies:** cli/studio/answers#ac:contradictions-without-index-errors (REQ: contradictions-verb)

Scenario source: [../README.md](../README.md) → `### AC: contradictions-without-index-errors`.

Given a workspace directory where `studio index` has never run, when I run `specscore studio contradictions`, then the command exits 2 with a message naming the expected store path and suggesting `specscore studio index`.

No seams are needed: the missing-store guard fires before any store read.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:contradictions-without-index-errors
# Requires: specscore on PATH (override with $SPECSCORE).
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: contradictions-without-index-errors not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
