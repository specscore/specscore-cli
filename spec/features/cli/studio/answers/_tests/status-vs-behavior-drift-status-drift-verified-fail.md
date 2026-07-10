---
format: https://specscore.md/scenario-specification
---

# Rehearse: status-drift-verified-fail

**Status:** pass-capable
**Verifies:** cli/studio/answers#ac:status-drift-verified-fail (REQ: status-vs-behavior-drift)

Scenario source: [../README.md](../README.md) → `### AC: status-drift-verified-fail`.

Given an indexed store with a `declared` `(<repo>#feat/x, has-status, Stable)` fact and a `verified-behavior` `(<repo>#feat/x#ac:y, has-verification-status, fail)` fact, when I run `specscore studio contradictions --format json`, then the command exits 0 and the JSON contains an item with detector `status-drift` whose two sides are the `has-status` fact and the `has-verification-status` fact, each side carrying its subject, object, evidence_class, evidence_pointer, and observed_at.

Seam: the declared `has-status` comes from `studio index` over a tiny fixture
repo (a `specscore.yaml` + one Stable feature); the failing
`has-verification-status` is the rehearse adapter's fact from a seeded
`latest.json` report — the same store `index` + a rehearse run produces, offline.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:status-drift-verified-fail
# Requires: specscore on PATH (override with $SPECSCORE), python3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"

mkdir -p app/spec/features/checkout app/.specscore/rehearse
cat > app/specscore.yaml <<'YAML'
name: app
YAML
cat > app/spec/features/checkout/README.md <<'MD'
---
format: https://specscore.md/feature-specification
---
# Feature: checkout

**Status:** Stable

## Acceptance Criteria

### AC: pays

Scenario: pays
Given x
When y
Then z
MD
cat > app/.specscore/rehearse/latest.json <<'JSON'
{"started_at":"2026-07-01T00:00:00Z","scenarios":[{"file":"pays.md","status":"fail","verifies":["checkout#ac:pays"]}]}
JSON
cat > studio.yaml <<'YAML'
name: demo
repos:
  - app
YAML
"$SPECSCORE_ABS" studio index >/dev/null

json="$("$SPECSCORE_ABS" studio contradictions --format json)"

JSON_OUT="$json" python3 - <<'PY'
import json, os
items = json.loads(os.environ["JSON_OUT"])
drift = [it for it in items if it.get("detector") == "status-drift"]
assert drift, f"FAIL: no status-drift item; got {items}"
match = []
for it in drift:
    sides = [it["a"], it["b"]]
    has_status = [s for s in sides if s["predicate"] == "has-status" and s["object"] == "Stable"]
    has_verif = [s for s in sides if s["predicate"] == "has-verification-status" and s["object"] == "fail"]
    if has_status and has_verif:
        match.append(it)
        for s in sides:
            for f in ("subject", "object", "evidence_class", "evidence_pointer", "observed_at"):
                assert s.get(f), f"FAIL: side missing {f}: {s}"
assert match, f"FAIL: no status-drift item pairing Stable has-status with fail has-verification-status; got {drift}"
PY

echo "PASS: status-drift-verified-fail"
```

---
*This document follows the https://specscore.md/scenario-specification*
