---
format: https://specscore.md/scenario-specification
---

# Rehearse: hurl-pass

**Status:** pending
**Verifies:** cli/rehearse/run#ac:hurl-pass (REQ: hurl-block)

Scenario source: [../README.md](../README.md) → `### AC: hurl-pass`.

Given `hurl` on PATH and a scenario whose bash step starts a local HTTP server and whose hurl block asserts `HTTP 200` from it, when I run `specscore rehearse run <that-file>`, then the command exits 0 and the scenario is `pass`.

```bash
#!/usr/bin/env bash
# Rehearse: cli/rehearse/run#ac:hurl-pass
# Requires: specscore on PATH (override with $SPECSCORE), hurl on PATH,
# python3 (the local HTTP server).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
command -v hurl >/dev/null 2>&1 \
  || { echo "FAIL: this scenario's Given requires hurl on PATH"; exit 1; }

# Given a scenario whose bash step starts a local HTTP server and whose hurl
# block asserts HTTP 200 from it (the inner fence is assembled via $fence so
# this file stays parseable). The port is randomized to avoid collisions;
# the outer trap reaps the server even if the run fails midway.
workdir="$(mktemp -d)"
pidfile="$workdir/server.pid"
trap '[ -f "$pidfile" ] && kill "$(cat "$pidfile")" 2>/dev/null; rm -rf "$workdir"' EXIT
cd "$workdir"
port=$(( (RANDOM % 20000) + 20000 ))
fence='```'
cat > hurl-pass.md <<MD
# Rehearse: hurl-pass fixture

**Status:** pending
**Verifies:** demo/fixture#ac:hurl-pass

${fence}bash
python3 -m http.server ${port} --bind 127.0.0.1 >/dev/null 2>&1 &
echo \$! > "${pidfile}"
for _ in \$(seq 1 100); do
  curl -fs "http://127.0.0.1:${port}/" >/dev/null 2>&1 && exit 0
  sleep 0.1
done
echo "server never came up"
exit 1
${fence}

${fence}hurl
GET http://127.0.0.1:${port}/
HTTP 200
${fence}

${fence}bash
kill "\$(cat "${pidfile}")"
rm -f "${pidfile}"
${fence}
MD

# When I run `specscore rehearse run hurl-pass.md`
set +e
out="$("$SPECSCORE" rehearse run hurl-pass.md 2>&1)"
exit_code=$?
set -e

# Then the command exits 0
[ "$exit_code" -eq 0 ] || { echo "FAIL: exit code $exit_code, want 0; output: $out"; exit 1; }

# And the scenario is `pass`
printf '%s\n' "$out" | grep -qE '^pass[[:space:]]+.*hurl-pass\.md' \
  || { echo "FAIL: report does not mark the scenario pass: $out"; exit 1; }
printf '%s\n' "$out" | grep -q '1 pass' \
  || { echo "FAIL: totals line does not count the pass: $out"; exit 1; }

echo "PASS: hurl-pass"
```

---
*This document follows the https://specscore.md/scenario-specification*
