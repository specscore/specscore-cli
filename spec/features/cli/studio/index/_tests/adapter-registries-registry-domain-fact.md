---
format: https://specscore.md/scenario-specification
---

# Rehearse: registry-domain-fact

**Status:** pending
**Verifies:** cli/studio/index#ac:registry-domain-fact (REQ: adapter-registries)

Scenario source: [../README.md](../README.md) → `### AC: registry-domain-fact`.

Given a fixture repo with a `domains.json` mapping `example.app` to a live status, when I run `specscore studio index` and then `specscore studio facts --predicate fronts`, then the output contains a fact whose subject is domain `example.app`.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/index#ac:registry-domain-fact
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given a fixture repo with a domains.json mapping example.app to a live status
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"
mkdir -p repo-ops
cat > repo-ops/domains.json <<'JSON'
{
  "generated_at": "2026-07-07T06:16:58Z",
  "domains": {
    "example.app": {
      "status": {
        "/": {
          "http_status": 200,
          "title": "Example — live"
        }
      },
      "cloudflare": {
        "workers": {
          "example-worker": "example.app/*"
        }
      }
    }
  }
}
JSON
cat > studio.yaml <<'YAML'
name: demo
repos:
  - repo-ops
YAML

# When I run `specscore studio index`
"$SPECSCORE" studio index >/dev/null

# And then `specscore studio facts --predicate fronts`
out="$("$SPECSCORE" studio facts --predicate fronts)"

# Then the output contains a fact whose subject is domain example.app
case "$out" in
  *"example.app"*) ;;
  *) echo "FAIL: fronts output lacks a fact with subject example.app: $out"; exit 1 ;;
esac

# And the domain also carries its live serves-status
status_out="$("$SPECSCORE" studio facts --predicate serves-status --subject example.app)"
case "$status_out" in
  *"200"*) ;;
  *) echo "FAIL: serves-status output lacks 200 for example.app: $status_out"; exit 1 ;;
esac

echo "PASS: registry-domain-fact"
```

---
*This document follows the https://specscore.md/scenario-specification*
