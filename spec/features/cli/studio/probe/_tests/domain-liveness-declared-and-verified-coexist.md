---
format: https://specscore.md/scenario-specification
---

# Rehearse: declared-and-verified-coexist

**Status:** pass-capable
**Verifies:** cli/studio/probe#ac:declared-and-verified-coexist (REQ: domain-liveness)

Scenario source: [../README.md](../README.md) → `### AC: declared-and-verified-coexist`.

Given the store after a domain probe, when I run `specscore studio facts --predicate serves-status --subject <domain> --format json`, then the JSON contains both a fact with evidence_class `declared` (pointer `domains.json`) and a fact with evidence_class `verified-behavior` (pointer the probed URL) for the same subject and predicate.

Seam note: the domain is realized as `127.0.0.1:<port>` against a localhost
fixture HTTP server (the plan's pinned seam). The declared fact's pointer is
`domains.json` and the verified fact's pointer is the URL that answered
(`http://127.0.0.1:<port>/` via the http fallback); the AC's coexistence
assertion is proven exactly — both facts survive for the same subject/predicate,
differing by evidence_class and pointer.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:declared-and-verified-coexist
# Requires: specscore on PATH (override with $SPECSCORE), python3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }

workdir="$(mktemp -d)"

python3 - "$workdir/port.txt" <<'PY' &
import http.server, socketserver, sys
class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200); self.end_headers(); self.wfile.write(b"ok")
    def log_message(self, *a): pass
with socketserver.TCPServer(("127.0.0.1", 0), H) as s:
    with open(sys.argv[1], "w") as f:
        f.write(str(s.server_address[1]))
    s.serve_forever()
PY
srv_pid=$!
trap 'kill "$srv_pid" 2>/dev/null; rm -rf "$workdir"' EXIT
for _ in $(seq 1 50); do [ -s "$workdir/port.txt" ] && break; sleep 0.1; done
port="$(cat "$workdir/port.txt")"
[ -n "$port" ] || { echo "FAIL: fixture server did not report a port"; exit 1; }
domain="127.0.0.1:${port}"

cd "$workdir"

# Given a store after a domain probe (declared fact from index + verified fact).
mkdir -p repo-ops
cat > repo-ops/domains.json <<JSON
{"domains":{"${domain}":{"status":{"/":{"http_status":200}}}}}
JSON
cat > studio.yaml <<'YAML'
name: demo
repos:
  - repo-ops
YAML
"$SPECSCORE_ABS" studio index >/dev/null
"$SPECSCORE_ABS" studio probe --kind domain >/dev/null

# When I query serves-status for the subject.
json="$("$SPECSCORE_ABS" studio facts \
  --predicate serves-status --subject "$domain" --format json)"

# Then both the declared and the verified-behavior fact coexist.
DOMAIN="$domain" FACTS_JSON="$json" python3 - <<'PY'
import json, os

facts = json.loads(os.environ["FACTS_JSON"])
domain = os.environ["DOMAIN"]
declared = [
    f for f in facts
    if f.get("subject") == domain
    and f.get("predicate") == "serves-status"
    and f.get("evidence_class") == "declared"
    and f.get("evidence_pointer") == "domains.json"
]
verified = [
    f for f in facts
    if f.get("subject") == domain
    and f.get("predicate") == "serves-status"
    and f.get("evidence_class") == "verified-behavior"
    and f.get("evidence_pointer", "").startswith("http")
]
assert declared, f"FAIL: no declared serves-status fact (pointer domains.json); got {facts}"
assert verified, f"FAIL: no verified-behavior serves-status fact (probed URL pointer); got {facts}"
PY

echo "PASS: declared-and-verified-coexist"
```

---
*This document follows the https://specscore.md/scenario-specification*
