---
format: https://specscore.md/scenario-specification
---

# Rehearse: manifest-consumes-fact

**Status:** pending
**Verifies:** cli/studio/index#ac:manifest-consumes-fact (REQ: adapter-manifests)

Scenario source: [../README.md](../README.md) → `### AC: manifest-consumes-fact`.

Given a fixture repo whose `go.mod` requires `example.com/m v1.2.3`, when I run `specscore studio index` and then `specscore studio facts --predicate consumes --format json`, then the JSON contains a fact whose object is `example.com/m@v1.2.3` with evidence_pointer `go.mod`.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/index#ac:manifest-consumes-fact
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given a fixture repo whose go.mod requires example.com/m v1.2.3
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"
mkdir -p repo-a
cat > repo-a/go.mod <<'GOMOD'
module example.com/fixture

go 1.22

require example.com/m v1.2.3
GOMOD
cat > studio.yaml <<'YAML'
name: demo
repos:
  - repo-a
YAML

# When I run `specscore studio index`
"$SPECSCORE" studio index >/dev/null

# And then `specscore studio facts --predicate consumes --format json`
json="$("$SPECSCORE" studio facts --predicate consumes --format json)"

require() { # require <needle> <label>
  case "$json" in
    *"$1"*) ;;
    *) echo "FAIL: JSON lacks $2 ($1): $json"; exit 1 ;;
  esac
}

# Then the JSON contains a fact whose object is example.com/m@v1.2.3
require '"object": "example.com/m@v1.2.3"' "object example.com/m@v1.2.3"
# with evidence_pointer go.mod
require '"evidence_pointer": "go.mod"' "evidence_pointer go.mod"

echo "PASS: manifest-consumes-fact"
```

---
*This document follows the https://specscore.md/scenario-specification*
