---
format: https://specscore.md/scenario-specification
---

# Rehearse: behavioral-supersession-not-flagged

**Status:** pass-capable
**Verifies:** cli/studio/answers#ac:behavioral-supersession-not-flagged (REQ: same-predicate-disagreement)

Scenario source: [../README.md](../README.md) → `### AC: behavioral-supersession-not-flagged`.

Given an indexed store with `(d, serves-status, 200)` and `(d, serves-status, down)`, both evidence_class `verified-behavior`, when I run `specscore studio contradictions --format json`, then no contradiction item of any detector references those two facts (differing objects across two behavioral observations are supersession — flagged by neither `naming-conflict` nor `status-drift`).

Seam: both `verified-behavior` `serves-status` facts are seeded with `sqlite3`
(the probe corpus's stubbed-seam pattern) so the two-behavioral-observations
case exists without a live probe. There is no `declared` serves-status here, so
status-drift's declared-vs-verified branch has nothing to pair — the two
behavioral facts are supersession, not contradiction.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:behavioral-supersession-not-flagged
# Requires: specscore on PATH (override with $SPECSCORE), python3, sqlite3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }
command -v sqlite3 >/dev/null 2>&1 || { echo "FAIL: sqlite3 not on PATH"; exit 1; }

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"

# A repo with a manifest so the store is non-empty but carries no serves-status
# declaration (only the two seeded behavioral facts exist for the domain).
mkdir -p app
cat > app/go.mod <<'GOMOD'
module github.com/acme/app

go 1.22
GOMOD
cat > studio.yaml <<'YAML'
name: demo
repos:
  - app
YAML
"$SPECSCORE_ABS" studio index >/dev/null

db="$workdir/.specscore-studio/facts.db"
for obj in 200 down; do
  sqlite3 "$db" "INSERT INTO facts
    (subject, predicate, object, evidence_class, evidence_pointer,
     adapter_id, adapter_version, observed_at, verified_at, ecosystem)
    VALUES ('d.example','serves-status','$obj','verified-behavior',
            'https://d.example/','probe-domain','0.1.0',
            '2026-07-03T08:00:00Z','2026-07-03T08:00:00Z','demo');"
done

json="$("$SPECSCORE_ABS" studio contradictions --format json)"

JSON_OUT="$json" python3 - <<'PY'
import json, os
items = json.loads(os.environ["JSON_OUT"])
for it in items:
    for side in (it["a"], it["b"]):
        assert side["subject"] != "d.example", \
            f"FAIL: a contradiction references the superseded behavioral fact: {it}"
PY

echo "PASS: behavioral-supersession-not-flagged"
```

---
*This document follows the https://specscore.md/scenario-specification*
