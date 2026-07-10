---
format: https://specscore.md/scenario-specification
---

# Rehearse: graphql-jsonpath

**Status:** pending
**Verifies:** cli/rehearse/run#ac:graphql-jsonpath (REQ: graphql-block)

Scenario source: [../README.md](../README.md) → `### AC: graphql-jsonpath`.

Given a local `httptest`-style GraphQL stub server returning `{"data":{"ok":true}}` and `hurl` on PATH, and a scenario whose graphql block posts a query with `-- assert-jsonpath: $.data.ok == true`, when I run `specscore rehearse run <that-file>`, then the command exits 0 and the scenario is `pass`.

```bash
#!/usr/bin/env bash
# Rehearse: cli/rehearse/run#ac:graphql-jsonpath
# Requires: specscore on PATH (override with $SPECSCORE), hurl on PATH
# (the graphql block compiles onto the hurl delegation), python3 (the stub).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
command -v hurl >/dev/null 2>&1 \
  || { echo "FAIL: this scenario's Given requires hurl on PATH"; exit 1; }

workdir="$(mktemp -d)"
server_pid=""
trap '[ -n "$server_pid" ] && kill "$server_pid" 2>/dev/null; rm -rf "$workdir"' EXIT
cd "$workdir"

# Given a local GraphQL stub server returning {"data":{"ok":true}}
port=$(( (RANDOM % 20000) + 20000 ))
python3 - "$port" >/dev/null 2>&1 <<'PY' &
import http.server, sys

class Stub(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        body = b'{"data":{"ok":true}}'
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *args):
        pass

http.server.HTTPServer(('127.0.0.1', int(sys.argv[1])), Stub).serve_forever()
PY
server_pid=$!
for _ in $(seq 1 100); do
  curl -fs -X POST "http://127.0.0.1:${port}/graphql" >/dev/null 2>&1 && break
  sleep 0.1
done
curl -fs -X POST "http://127.0.0.1:${port}/graphql" >/dev/null 2>&1 \
  || { echo "FAIL: the GraphQL stub never came up"; exit 1; }

# And a scenario whose graphql block posts a query with
# `-- assert-jsonpath: $.data.ok == true` (the inner fence is assembled via
# $fence so this file stays parseable).
fence='```'
cat > graphql-ok.md <<MD
# Rehearse: graphql-ok fixture

**Status:** pending
**Verifies:** demo/fixture#ac:graphql-ok

${fence}graphql url=http://127.0.0.1:${port}/graphql
query { ok }
-- assert-jsonpath: \$.data.ok == true
${fence}
MD

# When I run `specscore rehearse run graphql-ok.md`
set +e
out="$("$SPECSCORE" rehearse run graphql-ok.md 2>&1)"
exit_code=$?
set -e

# Then the command exits 0
[ "$exit_code" -eq 0 ] || { echo "FAIL: exit code $exit_code, want 0; output: $out"; exit 1; }

# And the scenario is `pass`
printf '%s\n' "$out" | grep -qE '^pass[[:space:]]+.*graphql-ok\.md' \
  || { echo "FAIL: report does not mark the scenario pass: $out"; exit 1; }
printf '%s\n' "$out" | grep -q '1 pass' \
  || { echo "FAIL: totals line does not count the pass: $out"; exit 1; }

echo "PASS: graphql-jsonpath"
```

---
*This document follows the https://specscore.md/scenario-specification*
