---
format: https://specscore.md/scenario-specification
---

# Rehearse: probe-preserves-index-facts

**Status:** pass-capable
**Verifies:** cli/studio/probe#ac:probe-preserves-index-facts (REQ: probe-merge)

Scenario source: [../README.md](../README.md) → `### AC: probe-preserves-index-facts`.

Given an indexed store with declared and derived facts from `studio index`, when I run `specscore studio probe` and then query facts by an index adapter, then the command exits 0 and the pre-existing `declared` and `derived` facts are still present alongside the new `verified-behavior` facts.

Seam note: the domain kind is exercised against a localhost fixture
(`127.0.0.1:<port>`) so a real verified-behavior fact is produced; the ci kind
runs with no `gh` on PATH (skipped with a warning), keeping the run hermetic.
The store is seeded with a declared `serves-status` fact (registries adapter)
and a derived `imports` fact (codegraph adapter) so both index evidence classes
are proven to survive the merge.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:probe-preserves-index-facts
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

# Given an indexed store with a declared fact (domains.json) and a derived fact
# (codegraph imports edge).
mkdir -p repo-ops
cat > repo-ops/domains.json <<JSON
{"domains":{"${domain}":{"status":{"/":{"http_status":200}}}}}
JSON
mkdir -p repo-graph/codegraph/nodes repo-graph/codegraph/edges
cat > repo-graph/codegraph/nodes/nodes.ingr <<'INGR'
# INGR.io | nodes: $ID, kind, name, qualified_name, file_path, language
"package:a"
"package"
"a"
"a"
null
"go"
"package:b"
"package"
"b"
"b"
null
"go"
# 2 records
INGR
cat > repo-graph/codegraph/edges/edges.ingr <<'INGR'
# INGR.io | edges: $ID, source, target, kind, metadata, line:int, col:int, provenance
"package:a|package:b|imports|0|0"
"package:a"
"package:b"
"imports"
null
0
0
""
# 1 records
INGR
cat > studio.yaml <<'YAML'
name: demo
repos:
  - repo-ops
  - repo-graph
YAML
"$SPECSCORE_ABS" studio index >/dev/null

# Snapshot the index-written declared and derived facts.
before_declared="$("$SPECSCORE_ABS" studio facts --class declared --count)"
before_derived="$("$SPECSCORE_ABS" studio facts --class derived --count)"
[ "$before_declared" -ge 1 ] || { echo "FAIL: expected >=1 declared fact before probe, got $before_declared"; exit 1; }
[ "$before_derived" -ge 1 ] || { echo "FAIL: expected >=1 derived fact before probe, got $before_derived"; exit 1; }

# When I run `specscore studio probe` (default --kind all; ci skipped, no gh).
set +e
probe_out="$(PATH="$workdir" "$SPECSCORE_ABS" studio probe 2>&1)"
probe_exit=$?
set -e
[ "$probe_exit" -eq 0 ] || { echo "FAIL: probe exited $probe_exit: $probe_out"; exit 1; }

# Then the declared and derived facts are still present, and a new
# verified-behavior fact exists alongside them.
after_declared="$("$SPECSCORE_ABS" studio facts --class declared --count)"
after_derived="$("$SPECSCORE_ABS" studio facts --class derived --count)"
after_verified="$("$SPECSCORE_ABS" studio facts --class verified-behavior --count)"
[ "$after_declared" -eq "$before_declared" ] \
  || { echo "FAIL: declared count changed $before_declared -> $after_declared"; exit 1; }
[ "$after_derived" -eq "$before_derived" ] \
  || { echo "FAIL: derived count changed $before_derived -> $after_derived"; exit 1; }
[ "$after_verified" -ge 1 ] \
  || { echo "FAIL: expected >=1 verified-behavior fact after probe, got $after_verified"; exit 1; }

echo "PASS: probe-preserves-index-facts"
```

---
*This document follows the https://specscore.md/scenario-specification*
