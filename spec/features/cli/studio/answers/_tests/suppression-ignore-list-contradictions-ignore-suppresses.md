---
format: https://specscore.md/scenario-specification
---

# Rehearse: contradictions-ignore-suppresses

**Status:** pending
**Verifies:** cli/studio/answers#ac:contradictions-ignore-suppresses (REQ: suppression-ignore-list)

Scenario source: [../README.md](../README.md) → `### AC: contradictions-ignore-suppresses`.

Given a store with one detectable contradiction and a `<workspace-dir>/.specscore-studio/contradictions-ignore.txt` whose single line is that contradiction's canonical `<side-a>  <side-b>` identity, when I run `specscore studio contradictions --format json`, then the item is absent from the JSON and no `contradicts` fact is written for it, and `specscore studio contradictions --show-ignored` lists that identity as suppressed.

Uses a fixture store and a workspace-local ignore file (no network).

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:contradictions-ignore-suppresses
# Requires: specscore on PATH (override with $SPECSCORE).
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: contradictions-ignore-suppresses not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
