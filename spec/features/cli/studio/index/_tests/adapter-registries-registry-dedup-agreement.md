---
format: https://specscore.md/scenario-specification
---

# Rehearse: registry-dedup-agreement

**Status:** pending
**Verifies:** cli/studio/index#ac:registry-dedup-agreement (REQ: adapter-registries)

Scenario source: [../README.md](../README.md) → `### AC: registry-dedup-agreement`.

Given a fixture repo whose `ecosystem-map.yaml` and `ecosystem.yaml` both declare the same product fronting the same domain, when I run `specscore studio index` and then `specscore studio facts --predicate fronts --count`, then exactly one `fronts` fact is stored for that domain/product — the duplicate is collapsed, not doubled.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/index#ac:registry-dedup-agreement
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given a fixture repo whose two ecosystem files agree on the same product.
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"
mkdir -p repo-ops
cat > repo-ops/ecosystem-map.yaml <<'YAML'
products:
  - id: dupius
    brand: "Dupius"
    domains: [dup.app]
    vertical: hospitality
YAML
cat > repo-ops/ecosystem.yaml <<'YAML'
products:
  - id: dupius
    brand: Dupius
    domains:
      - name: dup.app
    vertical: hospitality
YAML
cat > studio.yaml <<'YAML'
name: demo
repos:
  - repo-ops
YAML

# When I run `specscore studio index`
"$SPECSCORE" studio index >/dev/null

# And then `specscore studio facts --predicate fronts --count`, scoped to the domain.
count="$("$SPECSCORE" studio facts --predicate fronts --subject dup.app --count)"

# Then exactly one fronts fact survives — the duplicate is collapsed.
if [ "$count" != "1" ]; then
  echo "FAIL: expected exactly 1 fronts fact for dup.app, got: $count"
  "$SPECSCORE" studio facts --predicate fronts --subject dup.app
  exit 1
fi

echo "PASS: registry-dedup-agreement"
```

---
*This document follows the https://specscore.md/scenario-specification*
