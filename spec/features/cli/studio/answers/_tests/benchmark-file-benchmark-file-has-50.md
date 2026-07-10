---
format: https://specscore.md/scenario-specification
---

# Rehearse: benchmark-file-has-50

**Status:** pending
**Verifies:** cli/studio/answers#ac:benchmark-file-has-50 (REQ: benchmark-file)

Scenario source: [../README.md](../README.md) → `### AC: benchmark-file-has-50`.

Given the committed `benchmark/questions.jsonl`, when I count its lines and parse each as JSON with fields `id`, `question`, `template`, `parameter`, `expectation`, then there are exactly 50 instances, every `template` is one of the documented templates or empty (for `expected-unanswerable`), and the per-template counts match the `## Benchmark composition` table.

Pure file assertion over the committed benchmark (no network, no store).

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:benchmark-file-has-50
# Requires: specscore on PATH (override with $SPECSCORE).
# Status: pending — scaffolded stub; implement alongside the feature.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

echo "PENDING: benchmark-file-has-50 not yet implemented"
exit 1
```

---
*This document follows the https://specscore.md/scenario-specification*
