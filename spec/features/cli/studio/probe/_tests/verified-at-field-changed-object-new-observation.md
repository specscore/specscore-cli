---
format: https://specscore.md/scenario-specification
---

# Rehearse: changed-object-new-observation

**Status:** pass-capable
**Verifies:** cli/studio/probe#ac:changed-object-new-observation (REQ: verified-at-field)

Scenario source: [../README.md](../README.md) → `### AC: changed-object-new-observation`.

Given a store whose prior verified fact was `serves-status` `200` at T1, when the domain probe finds a connection error at T2 and I query `specscore studio facts --subject <domain> --class verified-behavior --format json`, then a `serves-status` `down` fact exists whose `observed_at` and `verified_at` both equal T2.

Seam note: the plan pins the seam to a real localhost fixture; T1/T2 are wall-
clock progression. The fixture serves 200 at T1; it is then stopped so the T2
probe gets a genuine both-schemes connection refusal → a `down` fact. Because
the object changed (`200` → `down`), the merge inserts a fresh observation
rather than refreshing the old fact: the `down` fact carries `observed_at ==
verified_at == T2` and the original `200` fact coexists. Both halves of the AC's
Then (the `down` fact's stamps equal T2; it is a new observation, not a refresh)
are proven exactly.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:changed-object-new-observation
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
cleanup() { [ -n "${srv_pid:-}" ] && kill "$srv_pid" 2>/dev/null; rm -rf "$workdir"; return 0; }
trap cleanup EXIT
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

# Given: probe at T1 while the fixture serves 200.
"$SPECSCORE_ABS" studio probe --kind domain >/dev/null
t1_json="$("$SPECSCORE_ABS" studio facts --subject "$domain" --class verified-behavior --format json)"

# Stop the fixture so the T2 probe gets a connection error, and advance the clock.
kill "$srv_pid" 2>/dev/null || true
wait "$srv_pid" 2>/dev/null || true
srv_pid=""   # already reaped; keep the EXIT trap from re-killing a dead pid
sleep 2

# When: probe again at T2 (server down → both schemes refuse → `down`).
"$SPECSCORE_ABS" studio probe --kind domain >/dev/null
t2_json="$("$SPECSCORE_ABS" studio facts --subject "$domain" --class verified-behavior --format json)"

# Then a `down` fact exists whose observed_at == verified_at == T2, and the
# prior 200 fact coexists (a new observation, not a re-verification).
T1_JSON="$t1_json" T2_JSON="$t2_json" python3 - <<'PY'
import json, os

t1 = json.loads(os.environ["T1_JSON"])
t2 = json.loads(os.environ["T2_JSON"])
assert len(t1) == 1 and t1[0]["object"] == "200", f"FAIL: expected one 200 fact at T1, got {t1}"
t1_verified = t1[0]["verified_at"]

down = [f for f in t2 if f.get("object") == "down"]
assert len(down) == 1, f"FAIL: expected exactly one `down` fact after the T2 probe, got {t2}"
d = down[0]
assert d["observed_at"] == d["verified_at"], (
    f"FAIL: `down` is a fresh observation — observed_at must equal verified_at "
    f"({d['observed_at']} != {d['verified_at']})"
)
assert d["verified_at"] > t1_verified, (
    f"FAIL: the `down` observation's stamp (T2) must be later than T1 "
    f"({d['verified_at']} !> {t1_verified})"
)
assert any(f.get("object") == "200" for f in t2), (
    f"FAIL: the prior 200 fact must coexist (a changed object appends, not overwrites); got {t2}"
)
PY

echo "PASS: changed-object-new-observation"
```

---
*This document follows the https://specscore.md/scenario-specification*
