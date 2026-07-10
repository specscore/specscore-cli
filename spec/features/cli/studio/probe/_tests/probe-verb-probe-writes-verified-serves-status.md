---
format: https://specscore.md/scenario-specification
---

# Rehearse: probe-writes-verified-serves-status

**Status:** pending
**Verifies:** cli/studio/probe#ac:probe-writes-verified-serves-status (REQ: domain-liveness)

Scenario source: [../README.md](../README.md) → `### AC: probe-writes-verified-serves-status`.

Given an indexed store containing a `declared` `serves-status` fact for domain `example.app`, and a domain probe stubbed to return HTTP 200 for `https://example.app/`, when I run `specscore studio probe --kind domain` and then `specscore studio facts --predicate serves-status --class verified-behavior --format json`, then the command exits 0 and the JSON contains a fact with subject `example.app`, predicate `serves-status`, object `200`, evidence_class `verified-behavior`, adapter id `probe-domain`, an evidence_pointer of `https://example.app/`, and a non-empty `observed_at`.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:probe-writes-verified-serves-status
# Requires: specscore on PATH (override with $SPECSCORE). Uses stubbed HTTP/exec seams.
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: probe-writes-verified-serves-status not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
