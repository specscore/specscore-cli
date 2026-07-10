---
format: https://specscore.md/scenario-specification
---

# Rehearse: multi-valued-not-flagged

**Status:** pass-capable
**Verifies:** cli/studio/answers#ac:multi-valued-not-flagged (REQ: same-predicate-disagreement)

Scenario source: [../README.md](../README.md) → `### AC: multi-valued-not-flagged`.

Given an indexed store with two `declared` `has-ac` facts sharing a subject but carrying different objects and different evidence pointers (a feature with two acceptance criteria), when I run `specscore studio contradictions --format json`, then the command exits 0 and no `naming-conflict` item references those two facts (`has-ac` is not in the single-valued predicate set — two ACs are two valid values, not a disagreement).

Seam: `studio index` over a tiny specscore repo whose one feature declares TWO
acceptance criteria — the specscore adapter emits one `has-ac` fact per AC with
line-suffixed pointers, exactly the multi-valued shape that flooded the Sneat
dogfood run (5,328 has-ac items) before the detector was scoped to the
single-valued predicate set.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:multi-valued-not-flagged
# Requires: specscore on PATH (override with $SPECSCORE), python3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"

mkdir -p app/spec/features/checkout
cat > app/specscore.yaml <<'YAML'
name: app
YAML
cat > app/spec/features/checkout/README.md <<'MD'
---
format: https://specscore.md/feature-specification
---
# Feature: checkout

**Status:** Draft

## Acceptance Criteria

### AC: pays

Scenario: pays
Given x
When y
Then z

### AC: refunds

Scenario: refunds
Given x
When y
Then z
MD
cat > studio.yaml <<'YAML'
name: demo
repos:
  - app
YAML
"$SPECSCORE_ABS" studio index >/dev/null

# The store holds two declared has-ac facts for the same subject with
# different objects and different (line-suffixed) pointers.
acs="$("$SPECSCORE_ABS" studio facts --predicate has-ac --format json)"
ACS="$acs" python3 - <<'PY'
import json, os
facts = json.loads(os.environ["ACS"]) or []
assert len(facts) == 2, f"FAIL: expected two has-ac facts in the Given store; got {facts}"
assert facts[0]["object"] != facts[1]["object"], f"FAIL: objects must differ: {facts}"
assert facts[0]["evidence_pointer"] != facts[1]["evidence_pointer"], \
    f"FAIL: pointers must differ: {facts}"
PY

set +e
json="$("$SPECSCORE_ABS" studio contradictions --format json)"
rc=$?
set -e
[ "$rc" -eq 0 ] || { echo "FAIL: exit code $rc, want 0"; exit 1; }

JSON_OUT="$json" python3 - <<'PY'
import json, os
items = json.loads(os.environ["JSON_OUT"])
nc = [it for it in items if it.get("detector") == "naming-conflict"
      and it["a"]["predicate"] == "has-ac"]
assert not nc, \
    f"FAIL: multi-valued has-ac must never be a naming-conflict; got {nc}"
assert items == [], f"FAIL: expected no contradictions at all; got {items}"
PY

echo "PASS: multi-valued-not-flagged"
```

---
*This document follows the https://specscore.md/scenario-specification*
