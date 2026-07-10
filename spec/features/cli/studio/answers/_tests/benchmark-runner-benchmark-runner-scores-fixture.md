---
format: https://specscore.md/scenario-specification
---

# Rehearse: benchmark-runner-scores-fixture

**Status:** pending
**Verifies:** cli/studio/answers#ac:benchmark-runner-scores-fixture (REQ: benchmark-runner)

Scenario source: [../README.md](../README.md) → `### AC: benchmark-runner-scores-fixture`.

Given the committed `benchmark/fixture/` indexed and probed with stubbed seams into a fixture store, when I run `benchmark/run.sh --db <fixture-db>`, then the runner prints an `answered-with-citations / 50` line, every `expected-unanswerable` instance is reported as correctly-declined, and the runner exits non-zero if any `expected-unanswerable` instance was answered.

Uses the committed hermetic fixture workspace and stubbed seams (no network).

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:benchmark-runner-scores-fixture
# Requires: specscore on PATH (override with $SPECSCORE).
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: benchmark-runner-scores-fixture not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
