---
format: https://specscore.md/scenario-specification
---

# Rehearse: non-github-repo-skipped

**Status:** pending
**Verifies:** cli/studio/probe#ac:non-github-repo-skipped (REQ: ci-state)

Scenario source: [../README.md](../README.md) → `### AC: non-github-repo-skipped`.

Given an indexed workspace repo with the exec seam stubbed so `git remote get-url origin` fails (no remote configured), when I run `specscore studio probe --kind ci`, then the command exits 0, the run summary contains a per-repo notice that the repo was skipped for having no GitHub remote, and no `ci-status` fact is written for it.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:non-github-repo-skipped
# Requires: specscore on PATH (override with $SPECSCORE). Uses stubbed HTTP/exec seams.
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: non-github-repo-skipped not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
