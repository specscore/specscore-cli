---
format: https://specscore.md/scenario-specification
---

# Rehearse: resolve-ambiguous-lists-candidates

**Status:** pass-capable
**Verifies:** cli/studio/answers#ac:resolve-ambiguous-lists-candidates (REQ: resolve-ambiguous-and-unknown)

Scenario source: [../README.md](../README.md) → `### AC: resolve-ambiguous-lists-candidates`.

Given an indexed store where the name `shared` is an `aliased-as` brand for two different product ids, when I run `specscore studio resolve shared`, then the command exits 5 and lists both candidate canonical ids, each with a citation.

Seam: two products in a fixture `ecosystem.yaml` share the brand `shared`, so
the store holds `(prod-a, aliased-as, shared)` and `(prod-b, aliased-as, shared)`
— the reused-brand ambiguity the AmbiguousSlug rule requires.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:resolve-ambiguous-lists-candidates
# Requires: specscore on PATH (override with $SPECSCORE), python3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"

mkdir -p ops
cat > ops/ecosystem.yaml <<'YAML'
products:
  - id: prod-a
    brand: shared
  - id: prod-b
    brand: shared
YAML
cat > studio.yaml <<'YAML'
name: demo
repos:
  - ops
YAML
"$SPECSCORE_ABS" studio index >/dev/null

set +e
json="$("$SPECSCORE_ABS" studio resolve shared --format json)"
rc=$?
set -e
[ "$rc" -eq 5 ] || { echo "FAIL: exit code $rc, want 5 (AmbiguousSlug)"; exit 1; }

JSON_OUT="$json" python3 - <<'PY'
import json, os
resp = json.loads(os.environ["JSON_OUT"])
cands = resp.get("candidates", [])
ids = {c["id"] for c in cands}
assert ids == {"prod-a", "prod-b"}, f"FAIL: candidates {ids}, want prod-a, prod-b"
for c in cands:
    cite = c.get("citation", {})
    assert cite.get("predicate") == "aliased-as" and cite.get("evidence_pointer"), \
        f"FAIL: candidate lacks an aliased-as citation: {c}"
PY

echo "PASS: resolve-ambiguous-lists-candidates"
```

---
*This document follows the https://specscore.md/scenario-specification*
