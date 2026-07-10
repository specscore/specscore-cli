---
format: https://specscore.md/scenario-specification
---

# Rehearse: reprobe-refreshes-verified-at

**Status:** pass-capable
**Verifies:** cli/studio/probe#ac:reprobe-refreshes-verified-at (REQ: verified-at-field)

Scenario source: [../README.md](../README.md) → `### AC: reprobe-refreshes-verified-at`.

Given a store already probed at time T1 with a `serves-status` `200` verified-behavior fact whose `observed_at` and `verified_at` both equal T1, when the domain probe is re-run at a later time T2 and I query `specscore studio facts --subject <domain> --class verified-behavior --format json`, then the fact's `observed_at` is still T1 and its `verified_at` is T2.

Seam note: the plan pins the seam to a real localhost fixture; the CLI has no
clock flag, so T1/T2 are realized by wall-clock progression — two probe runs
separated by a `sleep`, each against the same 200 fixture. The re-probe finds an
unchanged `(subject, predicate, object, class, adapter)` and refreshes
`verified_at` while preserving `observed_at`, exactly the AC's Then. The
assertion is honest and complete: `observed_at` is unchanged across the two runs
and `verified_at` strictly advances (T2 > T1).

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:reprobe-refreshes-verified-at
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

# Given: probe at T1 → a 200 verified fact whose observed_at == verified_at == T1.
"$SPECSCORE_ABS" studio probe --kind domain >/dev/null
t1_json="$("$SPECSCORE_ABS" studio facts --subject "$domain" --class verified-behavior --format json)"

# Advance the wall clock so the re-probe's verified_at is strictly later.
sleep 2

# When: probe again at T2 (same 200 fixture).
"$SPECSCORE_ABS" studio probe --kind domain >/dev/null
t2_json="$("$SPECSCORE_ABS" studio facts --subject "$domain" --class verified-behavior --format json)"

# Then observed_at is unchanged (still T1) and verified_at advanced to T2.
T1_JSON="$t1_json" T2_JSON="$t2_json" python3 - <<'PY'
import json, os

t1 = json.loads(os.environ["T1_JSON"])
t2 = json.loads(os.environ["T2_JSON"])
assert len(t1) == 1, f"FAIL: expected exactly one verified fact after first probe, got {t1}"
assert len(t2) == 1, f"FAIL: re-probe must refresh (not append) — expected one fact, got {t2}"
o1, v1 = t1[0]["observed_at"], t1[0]["verified_at"]
o2, v2 = t2[0]["observed_at"], t2[0]["verified_at"]
assert o1 == v1, f"FAIL: first-write observed_at != verified_at ({o1} != {v1})"
assert o2 == o1, f"FAIL: observed_at must be preserved across re-probe ({o2} != {o1})"
assert v2 > v1, f"FAIL: verified_at must advance across re-probe ({v2} !> {v1})"
PY

echo "PASS: reprobe-refreshes-verified-at"
```

---
*This document follows the https://specscore.md/scenario-specification*
