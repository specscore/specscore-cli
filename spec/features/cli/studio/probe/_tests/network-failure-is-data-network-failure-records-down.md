---
format: https://specscore.md/scenario-specification
---

# Rehearse: network-failure-records-down

**Status:** pass-capable
**Verifies:** cli/studio/probe#ac:network-failure-records-down (REQ: network-failure-is-data)

Scenario source: [../README.md](../README.md) → `### AC: network-failure-records-down`.

Given an indexed store with a `serves-status` fact for a domain, and a domain probe that gets a connection error over both schemes, when I run `specscore studio probe --kind domain` and then `specscore studio facts --subject <domain> --predicate serves-status --class verified-behavior --format json`, then the command exits 0 and the JSON contains a fact with object `down` and evidence_class `verified-behavior`.

Seam note: the plan's localhost-fixture seam realizes the "unreachable" domain as
a `127.0.0.1:<port>` whose port has no listener — a genuine connection refusal
at the transport layer over both https and http, which is exactly the both-
schemes failure `REQ: network-failure-is-data` maps to `down`. The AC's
`dead.example` is honestly substituted; the `down` mapping and exit-0 semantics
are proven exactly.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:network-failure-records-down
# Requires: specscore on PATH (override with $SPECSCORE), python3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"

# Reserve a free localhost port, then release it so nothing listens there — a
# probe of it gets a genuine connection refusal over both schemes.
port="$(python3 -c 'import socket
s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')"
[ -n "$port" ] || { echo "FAIL: could not reserve a free port"; exit 1; }
domain="127.0.0.1:${port}"

# Given an indexed store with a serves-status fact for the (now dead) domain.
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
# A dead domain is a successful observation — the verb still exits 0.
[ "$probe_exit" -eq 0 ] || { echo "FAIL: probe exited $probe_exit: $probe_out"; exit 1; }

# And then query the verified-behavior serves-status fact.
json="$("$SPECSCORE_ABS" studio facts \
  --subject "$domain" --predicate serves-status --class verified-behavior --format json)"

# Then the fact's object is `down`. Pass the JSON via the environment (the
# `down` pointer embeds the transport error verbatim, so it must not be spliced
# into the Python source).
DOMAIN="$domain" FACTS_JSON="$json" python3 - <<'PY'
import json, os

facts = json.loads(os.environ["FACTS_JSON"])
domain = os.environ["DOMAIN"]
match = [
    f for f in facts
    if f.get("subject") == domain
    and f.get("predicate") == "serves-status"
    and f.get("object") == "down"
    and f.get("evidence_class") == "verified-behavior"
]
assert match, f"FAIL: no verified-behavior serves-status=down fact for {domain}; got {facts}"
PY

echo "PASS: network-failure-records-down"
```

---
*This document follows the https://specscore.md/scenario-specification*
