---
format: https://specscore.md/scenario-specification
---

# Rehearse: status-drift-dead-domain

**Status:** pass-capable
**Verifies:** cli/studio/answers#ac:status-drift-dead-domain (REQ: status-vs-behavior-drift)

Scenario source: [../README.md](../README.md) → `### AC: status-drift-dead-domain`.

Given an indexed store with `(dead.example, serves-status, 200)` with evidence_class `declared` (pointer `domains.json`) and `(dead.example, serves-status, down)` with evidence_class `verified-behavior` (the probe fact), when I run `specscore studio contradictions --format json`, then the JSON contains a `status-drift` item whose two sides are the declared `200` fact and the verified `down` fact, each side carrying its evidence_class, evidence_pointer, and observed_at.

Seam: the declared `200` fact comes from `studio index` over a fixture
`domains.json`; the `down` verified-behavior fact — which `studio probe` would
mint from a live check — is seeded with `sqlite3` (the probe corpus's
stubbed-seam pattern) so the scenario stays hermetic (no network).

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:status-drift-dead-domain
# Requires: specscore on PATH (override with $SPECSCORE), python3, sqlite3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }
command -v sqlite3 >/dev/null 2>&1 || { echo "FAIL: sqlite3 not on PATH"; exit 1; }

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"

mkdir -p ops
cat > ops/domains.json <<'JSON'
{"domains":{"dead.example":{"status":{"/":{"http_status":200}}}}}
JSON
cat > studio.yaml <<'YAML'
name: demo
repos:
  - ops
YAML
"$SPECSCORE_ABS" studio index >/dev/null

db="$workdir/.specscore-studio/facts.db"
sqlite3 "$db" "INSERT INTO facts
  (subject, predicate, object, evidence_class, evidence_pointer,
   adapter_id, adapter_version, observed_at, verified_at, ecosystem)
  VALUES ('dead.example','serves-status','down','verified-behavior',
          'https://dead.example/','probe-domain','0.1.0',
          '2026-07-03T08:00:00Z','2026-07-03T08:00:00Z','demo');"

json="$("$SPECSCORE_ABS" studio contradictions --format json)"

JSON_OUT="$json" python3 - <<'PY'
import json, os
items = json.loads(os.environ["JSON_OUT"])
match = []
for it in items:
    if it.get("detector") != "status-drift":
        continue
    sides = [it["a"], it["b"]]
    declared200 = [s for s in sides if s["evidence_class"] == "declared"
                   and s["object"] == "200" and s["evidence_pointer"] == "domains.json"]
    verifieddown = [s for s in sides if s["evidence_class"] == "verified-behavior"
                    and s["object"] == "down"]
    if declared200 and verifieddown:
        match.append(it)
        for s in sides:
            for f in ("evidence_class", "evidence_pointer", "observed_at"):
                assert s.get(f), f"FAIL: side missing {f}: {s}"
assert match, f"FAIL: no status-drift item pairing declared 200 with verified down; got {items}"
PY

echo "PASS: status-drift-dead-domain"
```

---
*This document follows the https://specscore.md/scenario-specification*
