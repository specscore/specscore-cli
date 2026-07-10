---
format: https://specscore.md/scenario-specification
---

# Rehearse: declared-and-verified-coexist

**Status:** pending
**Verifies:** cli/studio/probe#ac:declared-and-verified-coexist (REQ: domain-liveness)

Scenario source: [../README.md](../README.md) → `### AC: declared-and-verified-coexist`.

Given the store after a domain probe, when I run `specscore studio facts --predicate serves-status --subject example.app --format json`, then the JSON contains both a fact with evidence_class `declared` (pointer `domains.json`) and a fact with evidence_class `verified-behavior` (pointer the probed URL) for the same subject and predicate.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:declared-and-verified-coexist
# Requires: specscore on PATH (override with $SPECSCORE). Uses stubbed HTTP/exec seams.
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: declared-and-verified-coexist not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
