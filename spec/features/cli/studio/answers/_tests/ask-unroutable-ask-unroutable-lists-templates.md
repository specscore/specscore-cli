---
format: https://specscore.md/scenario-specification
---

# Rehearse: ask-unroutable-lists-templates

**Status:** pending
**Verifies:** cli/studio/answers#ac:ask-unroutable-lists-templates (REQ: ask-unroutable)

Scenario source: [../README.md](../README.md) → `### AC: ask-unroutable-lists-templates`.

Given any indexed store, when I run `specscore studio ask "why does contactus exist"`, then the command exits 1, prints a "no template matched" notice, and lists the routable templates (the same content as `specscore studio ask --list`).

Uses a fixture store seeded offline (no network).

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:ask-unroutable-lists-templates
# Requires: specscore on PATH (override with $SPECSCORE).
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: ask-unroutable-lists-templates not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
