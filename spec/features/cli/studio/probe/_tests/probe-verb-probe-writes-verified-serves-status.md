---
format: https://specscore.md/scenario-specification
---

# Rehearse: probe-writes-verified-serves-status

**Status:** pass-capable
**Verifies:** cli/studio/probe#ac:probe-writes-verified-serves-status (REQ: domain-liveness)

Scenario source: [../README.md](../README.md) → `### AC: probe-writes-verified-serves-status`.

Given an indexed store containing a `declared` `serves-status` fact for a domain, and a domain probe that reaches a live HTTP fixture returning 200, when I run `specscore studio probe --kind domain` and then `specscore studio facts --predicate serves-status --class verified-behavior --format json`, then the command exits 0 and the JSON contains a fact with the domain subject, predicate `serves-status`, object `200`, evidence_class `verified-behavior`, adapter id `probe-domain`, an evidence_pointer of the probed URL, and a non-empty `observed_at`.

Seam note: the plan pins the domain seam to a localhost fixture HTTP server (no
cross-process Go-var stubbing). The AC's illustrative `example.app` /
`https://example.app/` are honestly realized here as `127.0.0.1:<port>` (the
domain subject the registries adapter derives verbatim from `domains.json`) and
the URL that actually answered. The fixture serves plaintext HTTP, so the
probe's https-first attempt fails at the transport layer and the http fallback
answers — the recorded evidence pointer is `http://127.0.0.1:<port>/`, the URL
that produced the response (`REQ: domain-liveness`'s scheme policy). Every
structural assertion of the AC (object `200`, class, adapter id, the pointer =
the requested URL, non-empty `observed_at`) is proven exactly.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:probe-writes-verified-serves-status
# Requires: specscore on PATH (override with $SPECSCORE), python3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }

workdir="$(mktemp -d)"

# A localhost fixture HTTP server that returns 200 for GET /.
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

# Given an indexed store with a declared serves-status fact for the domain.
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

# When I run `specscore studio probe --kind domain`
set +e
probe_out="$("$SPECSCORE_ABS" studio probe --kind domain 2>&1)"
probe_exit=$?
set -e
[ "$probe_exit" -eq 0 ] || { echo "FAIL: probe exited $probe_exit: $probe_out"; exit 1; }

# And then `specscore studio facts ... --class verified-behavior --format json`
json="$("$SPECSCORE_ABS" studio facts \
  --predicate serves-status --class verified-behavior --format json)"

# Then the JSON contains the fully-formed verified-behavior fact.
DOMAIN="$domain" FACTS_JSON="$json" python3 - <<'PY'
import json, os

facts = json.loads(os.environ["FACTS_JSON"])
domain = os.environ["DOMAIN"]
want_pointer = f"http://{domain}/"
match = [
    f for f in facts
    if f.get("subject") == domain
    and f.get("predicate") == "serves-status"
    and f.get("object") == "200"
    and f.get("evidence_class") == "verified-behavior"
    and f.get("adapter", {}).get("id") == "probe-domain"
    and f.get("evidence_pointer") == want_pointer
    and f.get("observed_at")
]
assert match, (
    f"FAIL: no verified-behavior serves-status 200 fact for {domain} "
    f"with pointer {want_pointer} and a non-empty observed_at; got: {facts}"
)
PY

echo "PASS: probe-writes-verified-serves-status"
```

---
*This document follows the https://specscore.md/scenario-specification*
