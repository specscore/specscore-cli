---
format: https://specscore.md/scenario-specification
---

# Rehearse: naming-conflict-declared-disagreement

**Status:** pass-capable
**Verifies:** cli/studio/answers#ac:naming-conflict-declared-disagreement (REQ: same-predicate-disagreement)

Scenario source: [../README.md](../README.md) → `### AC: naming-conflict-declared-disagreement`.

Given an indexed store with `(subj, provides, ext-foo)` and `(subj, provides, foo-contract)`, both evidence_class `declared` with different evidence pointers, when I run `specscore studio contradictions --format json`, then the JSON contains a `naming-conflict` item whose two sides are those two facts, and the item is absent when the two facts share the same object.

Seam: the two disagreeing `declared` facts are realized as the registries
adapter's `implemented-by` facts for one product id, declared in two ecosystem
maps (distinct filenames → distinct pointers) that name different repos — the
`ext-<id>` vs `<id>-contract` class. A second run over agreeing maps (same
object) proves the item is absent.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:naming-conflict-declared-disagreement
# Requires: specscore on PATH (override with $SPECSCORE), python3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }

build_store() {
  # $1 = the object the legacy map asserts for gameboard-ext implemented-by.
  local legacy_repo="$1" dir
  dir="$(mktemp -d)"
  mkdir -p "$dir/ops" "$dir/ops-legacy"
  cat > "$dir/ops/ecosystem.yaml" <<'YAML'
products:
  - id: gameboard-ext
    repos:
      - ext-gameboard
YAML
  cat > "$dir/ops-legacy/ecosystem-legacy.yaml" <<YAML
products:
  - id: gameboard-ext
    repos:
      - ${legacy_repo}
YAML
  cat > "$dir/studio.yaml" <<'YAML'
name: demo
repos:
  - ops
  - ops-legacy
YAML
  ( cd "$dir" && "$SPECSCORE_ABS" studio index >/dev/null )
  echo "$dir"
}

# Disagreeing objects (ext-gameboard vs gameboard-contract) → item present.
d1="$(build_store gameboard-contract)"
trap 'rm -rf "$d1"' EXIT
json="$( cd "$d1" && "$SPECSCORE_ABS" studio contradictions --format json )"
JSON_OUT="$json" python3 - <<'PY'
import json, os
items = json.loads(os.environ["JSON_OUT"])
nc = []
for it in items:
    if it.get("detector") != "naming-conflict":
        continue
    objs = {it["a"]["object"], it["b"]["object"]}
    if objs == {"ext-gameboard", "gameboard-contract"} \
       and it["a"]["evidence_class"] == "declared" and it["b"]["evidence_class"] == "declared" \
       and it["a"]["evidence_pointer"] != it["b"]["evidence_pointer"]:
        nc.append(it)
assert nc, f"FAIL: no naming-conflict item over the two declared objects; got {items}"
PY

# Agreeing objects (both ext-gameboard) → item absent.
d2="$(build_store ext-gameboard)"
trap 'rm -rf "$d1" "$d2"' EXIT
json2="$( cd "$d2" && "$SPECSCORE_ABS" studio contradictions --format json )"
JSON_OUT="$json2" python3 - <<'PY'
import json, os
items = json.loads(os.environ["JSON_OUT"])
nc = [it for it in items if it.get("detector") == "naming-conflict"
      and "gameboard-ext" in (it["a"]["subject"] + it["b"]["subject"])]
assert not nc, f"FAIL: naming-conflict must be absent when objects agree; got {nc}"
PY

echo "PASS: naming-conflict-declared-disagreement"
```

---
*This document follows the https://specscore.md/scenario-specification*
