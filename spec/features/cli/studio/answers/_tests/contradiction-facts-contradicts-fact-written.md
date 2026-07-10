---
format: https://specscore.md/scenario-specification
---

# Rehearse: contradicts-fact-written

**Status:** pending
**Verifies:** cli/studio/answers#ac:contradicts-fact-written (REQ: contradiction-facts)

Scenario source: [../README.md](../README.md) → `### AC: contradicts-fact-written`.

Given an indexed store containing exactly one detectable contradiction, when I run `specscore studio contradictions` and then `specscore studio facts --predicate contradicts --format json`, then the facts JSON contains a `contradicts` fact whose subject and object are the two sides' fact refs, evidence_class `derived`, adapter id `contradictions`, and evidence_pointer naming the detector, and re-running `contradictions` does not create a duplicate (same subject/predicate/object).

Uses a fixture store seeded offline (no network).

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:contradicts-fact-written
# Requires: specscore on PATH (override with $SPECSCORE).
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: contradicts-fact-written not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
