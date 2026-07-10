---
format: https://specscore.md/scenario-specification
---

# Rehearse: contradictions-ignore-suppresses

**Status:** pass-capable
**Verifies:** cli/studio/answers#ac:contradictions-ignore-suppresses (REQ: suppression-ignore-list)

Scenario source: [../README.md](../README.md) → `### AC: contradictions-ignore-suppresses`.

Given a store with one detectable contradiction and a `<workspace-dir>/.specscore-studio/contradictions-ignore.txt` whose single line is that contradiction's canonical `<side-a>  <side-b>` identity, when I run `specscore studio contradictions --format json`, then the item is absent from the JSON and no `contradicts` fact is written for it, and `specscore studio contradictions --show-ignored` lists that identity as suppressed.

Seam: the one contradiction is a naming-conflict; its canonical identity (the
two smaller-ref-first `<subject>|<predicate>|<object>` refs joined by two spaces)
is computed from a first `--no-write` run and written verbatim into the ignore
file, so the identity is exactly the form the suppression reads.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:contradictions-ignore-suppresses
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
      - gameboard-contract
YAML
cat > studio.yaml <<'YAML'
name: demo
repos:
  - ops
  - ops-legacy
YAML
"$SPECSCORE_ABS" studio index >/dev/null

# Compute the canonical identity from a no-write detection run.
detected="$("$SPECSCORE_ABS" studio contradictions --no-write --format json)"
identity="$(DETECTED="$detected" python3 - <<'PY'
import json, os
items = json.loads(os.environ["DETECTED"])
nc = [it for it in items if it["detector"] == "naming-conflict"]
assert len(nc) == 1, f"expected exactly one naming-conflict, got {items}"
it = nc[0]
def ref(s): return f'{s["subject"]}|{s["predicate"]}|{s["object"]}'
a, b = sorted([ref(it["a"]), ref(it["b"])])
print(a + "  " + b)
PY
)"

# Write the ignore file with a reason comment on the line.
ignore="$workdir/.specscore-studio/contradictions-ignore.txt"
printf '%s  # accepted: legacy alias kept intentionally\n' "$identity" > "$ignore"

# Suppressed: the item is absent and no contradicts fact is written.
json="$("$SPECSCORE_ABS" studio contradictions --format json)"
facts="$("$SPECSCORE_ABS" studio facts --predicate contradicts --format json)"
JSON_OUT="$json" FACTS="$facts" python3 - <<'PY'
import json, os
items = json.loads(os.environ["JSON_OUT"])
nc = [it for it in items if it["detector"] == "naming-conflict"]
assert not nc, f"FAIL: suppressed contradiction still present; got {items}"
contra = json.loads(os.environ["FACTS"]) or []
assert contra == [], f"FAIL: a contradicts fact was written for a suppressed item; got {contra}"
PY

# --show-ignored lists the suppressed identity.
shown="$("$SPECSCORE_ABS" studio contradictions --show-ignored --format json)"
IDENTITY="$identity" SHOWN="$shown" python3 - <<'PY'
import json, os
items = json.loads(os.environ["SHOWN"])
ident = os.environ["IDENTITY"]
ign = [it for it in items if it.get("ignored") and it.get("identity") == ident]
assert ign, f"FAIL: --show-ignored did not list the suppressed identity {ident!r}; got {items}"
assert "legacy alias" in ign[0].get("ignore_reason", ""), \
    f"FAIL: --show-ignored did not carry the reason comment; got {ign[0]}"
PY

echo "PASS: contradictions-ignore-suppresses"
```

---
*This document follows the https://specscore.md/scenario-specification*
