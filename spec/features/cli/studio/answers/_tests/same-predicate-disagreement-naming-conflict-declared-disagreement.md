---
format: https://specscore.md/scenario-specification
---

# Rehearse: naming-conflict-declared-disagreement

**Status:** pass-capable
**Verifies:** cli/studio/answers#ac:naming-conflict-declared-disagreement (REQ: same-predicate-disagreement)

Scenario source: [../README.md](../README.md) → `### AC: naming-conflict-declared-disagreement`.

Given an indexed store with `(d.example, fronts, ext-foo)` and `(d.example, fronts, foo-contract)`, both evidence_class `declared` with different evidence pointers (`fronts` is in the single-valued set), when I run `specscore studio contradictions --format json`, then the JSON contains a `naming-conflict` item whose two sides are those two facts, and the item is absent when the two facts share the same object.

Seam: the two disagreeing `declared` facts are realized as the registries
adapter's `fronts` facts for one domain, declared in two ecosystem maps
(distinct filenames → distinct pointers) that front the domain with different
products — `fronts` is single-valued (a domain fronts one product), so the
disagreement is genuine. A second run over agreeing maps (same product) proves
the item is absent.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:naming-conflict-declared-disagreement
# Requires: specscore on PATH (override with $SPECSCORE), python3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }

build_store() {
  # $1 = the product id the legacy map says d.example fronts.
  local legacy_product="$1" dir
  dir="$(mktemp -d)"
  mkdir -p "$dir/ops" "$dir/ops-legacy"
  cat > "$dir/ops/ecosystem.yaml" <<'YAML'
products:
  - id: ext-foo
    domains:
      - d.example
YAML
  cat > "$dir/ops-legacy/ecosystem-legacy.yaml" <<YAML
products:
  - id: ${legacy_product}
    domains:
      - d.example
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

# Disagreeing objects (ext-foo vs foo-contract fronting d.example) → item present.
d1="$(build_store foo-contract)"
trap 'rm -rf "$d1"' EXIT
json="$( cd "$d1" && "$SPECSCORE_ABS" studio contradictions --format json )"
JSON_OUT="$json" python3 - <<'PY'
import json, os
items = json.loads(os.environ["JSON_OUT"])
nc = []
for it in items:
    if it.get("detector") != "naming-conflict":
        continue
    if it["a"]["subject"] != "d.example" or it["a"]["predicate"] != "fronts":
        continue
    objs = {it["a"]["object"], it["b"]["object"]}
    if objs == {"ext-foo", "foo-contract"} \
       and it["a"]["evidence_class"] == "declared" and it["b"]["evidence_class"] == "declared" \
       and it["a"]["evidence_pointer"] != it["b"]["evidence_pointer"]:
        nc.append(it)
assert nc, f"FAIL: no naming-conflict item over the two declared fronts objects; got {items}"
PY

# Agreeing objects (both maps front d.example with ext-foo) → item absent.
d2="$(build_store ext-foo)"
trap 'rm -rf "$d1" "$d2"' EXIT
json2="$( cd "$d2" && "$SPECSCORE_ABS" studio contradictions --format json )"
JSON_OUT="$json2" python3 - <<'PY'
import json, os
items = json.loads(os.environ["JSON_OUT"])
nc = [it for it in items if it.get("detector") == "naming-conflict"
      and "d.example" in (it["a"]["subject"] + it["b"]["subject"])]
assert not nc, f"FAIL: naming-conflict must be absent when objects agree; got {nc}"
PY

echo "PASS: naming-conflict-declared-disagreement"
```

---
*This document follows the https://specscore.md/scenario-specification*
