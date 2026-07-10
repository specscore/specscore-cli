---
format: https://specscore.md/scenario-specification
---

# Rehearse: ask-routes-with-citations

**Status:** pass-capable
**Verifies:** cli/studio/answers#ac:ask-routes-with-citations (REQ: ask-verb-and-router)

Scenario source: [../README.md](../README.md) → `### AC: ask-routes-with-citations`.

Given an indexed store with `(acme.app, fronts, acme)` declared, when I run `specscore studio ask "who fronts acme.app" --format json`, then the command exits 0, the JSON `template` is `who-fronts`, the `answer` names `acme`, and `citations` is a non-empty array whose entry names the `fronts` fact's evidence_pointer and adapter id.

Seam: the `fronts` fact comes from `studio index` over a fixture `domains.json`
whose `acme.app` entry routes to a Cloudflare worker `acme` — the registries
adapter's declared `(acme.app, fronts, acme)` fact.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:ask-routes-with-citations
# Requires: specscore on PATH (override with $SPECSCORE), python3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"

mkdir -p ops
cat > ops/domains.json <<'JSON'
{"domains":{"acme.app":{"cloudflare":{"workers":{"acme":"/*"}}}}}
JSON
cat > studio.yaml <<'YAML'
name: demo
repos:
  - ops
YAML
"$SPECSCORE_ABS" studio index >/dev/null

set +e
json="$("$SPECSCORE_ABS" studio ask "who fronts acme.app" --format json)"
rc=$?
set -e
[ "$rc" -eq 0 ] || { echo "FAIL: exit code $rc, want 0; out: $json"; exit 1; }

JSON_OUT="$json" python3 - <<'PY'
import json, os
resp = json.loads(os.environ["JSON_OUT"])
assert resp["template"] == "who-fronts", f"FAIL: template {resp['template']!r}, want who-fronts"
assert "acme" in resp["answer"], f"FAIL: answer does not name acme: {resp['answer']!r}"
cites = resp["citations"]
assert cites, f"FAIL: citations empty; {resp}"
c = cites[0]
assert c.get("evidence_pointer") and c.get("adapter"), \
    f"FAIL: citation lacks evidence_pointer / adapter: {c}"
PY

echo "PASS: ask-routes-with-citations"
```

---
*This document follows the https://specscore.md/scenario-specification*
