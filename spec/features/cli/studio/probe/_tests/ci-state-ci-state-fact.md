---
format: https://specscore.md/scenario-specification
---

# Rehearse: ci-state-fact

**Status:** pending
**Verifies:** cli/studio/probe#ac:ci-state-fact (REQ: ci-state)

Scenario source: [../README.md](../README.md) → `### AC: ci-state-fact`.

Given an indexed workspace repo `widget` whose `origin` remote is `https://github.com/acme/widget.git`, with the exec seam stubbed so `git remote get-url origin` returns that URL and `gh api` returns a latest default-branch run with conclusion `success`, when I run `specscore studio probe --kind ci` and then `specscore studio facts --predicate ci-status --format json`, then the command exits 0 and the JSON contains a fact with subject `widget` (the store's repo slug), object `success`, evidence_class `verified-behavior`, adapter id `probe-ci`, and an evidence_pointer naming the queried `gh api` path.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:ci-state-fact
# Requires: specscore on PATH (override with $SPECSCORE). Uses stubbed HTTP/exec seams.
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: ci-state-fact not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
