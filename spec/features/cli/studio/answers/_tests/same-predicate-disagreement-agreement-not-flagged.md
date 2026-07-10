---
format: https://specscore.md/scenario-specification
---

# Rehearse: agreement-not-flagged

**Status:** pass-capable
**Verifies:** cli/studio/answers#ac:agreement-not-flagged (REQ: same-predicate-disagreement)

Scenario source: [../README.md](../README.md) → `### AC: agreement-not-flagged`.

Given an indexed store where two `declared` facts share subject, predicate, and object but come from different evidence pointers, and no other disagreement exists, when I run `specscore studio contradictions --format json`, then the command exits 0 and the JSON is an empty array.

Seam: two ecosystem maps (distinct filenames → distinct pointers) both declare
`(gameboard-ext, implemented-by, ext-gameboard)` — same object, different
pointers: agreement, never a contradiction. No probe/rehearse facts exist, so no
status-drift can fire either; the array must be empty.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:agreement-not-flagged
# Requires: specscore on PATH (override with $SPECSCORE), python3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"

mkdir -p ops ops-legacy
cat > ops/ecosystem.yaml <<'YAML'
products:
  - id: gameboard-ext
    repos:
      - ext-gameboard
YAML
cat > ops-legacy/ecosystem-legacy.yaml <<'YAML'
products:
  - id: gameboard-ext
    repos:
      - ext-gameboard
YAML
cat > studio.yaml <<'YAML'
name: demo
repos:
  - ops
  - ops-legacy
YAML
"$SPECSCORE_ABS" studio index >/dev/null

set +e
json="$("$SPECSCORE_ABS" studio contradictions --format json)"
rc=$?
set -e
[ "$rc" -eq 0 ] || { echo "FAIL: exit code $rc, want 0"; exit 1; }

JSON_OUT="$json" python3 - <<'PY'
import json, os
items = json.loads(os.environ["JSON_OUT"])
assert items == [], f"FAIL: expected empty array (agreement is not a contradiction); got {items}"
PY

echo "PASS: agreement-not-flagged"
```

---
*This document follows the https://specscore.md/scenario-specification*
