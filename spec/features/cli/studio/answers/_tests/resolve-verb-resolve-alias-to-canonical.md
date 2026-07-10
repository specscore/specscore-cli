---
format: https://specscore.md/scenario-specification
---

# Rehearse: resolve-alias-to-canonical

**Status:** pass-capable
**Verifies:** cli/studio/answers#ac:resolve-alias-to-canonical (REQ: resolve-verb)

Scenario source: [../README.md](../README.md) → `### AC: resolve-alias-to-canonical`.

Given an indexed store containing the `declared` fact `(sizeus, aliased-as, SizeChart)`, when I run `specscore studio resolve SizeChart`, then the command exits 0 and prints `sizeus`, and `specscore studio resolve sizechart` (different case) resolves identically.

Seam: the `aliased-as` fact comes from `studio index` over a fixture
`ecosystem.yaml` declaring product `sizeus` with brand `SizeChart` — the
benchmark's canonical resolution anchor.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:resolve-alias-to-canonical
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"

mkdir -p ops
cat > ops/ecosystem.yaml <<'YAML'
products:
  - id: sizeus
    brand: SizeChart
YAML
cat > studio.yaml <<'YAML'
name: demo
repos:
  - ops
YAML
"$SPECSCORE_ABS" studio index >/dev/null

out="$("$SPECSCORE_ABS" studio resolve SizeChart)"
[ "$out" = "sizeus" ] || { echo "FAIL: resolve SizeChart printed '$out', want 'sizeus'"; exit 1; }

out_lc="$("$SPECSCORE_ABS" studio resolve sizechart)"
[ "$out_lc" = "sizeus" ] || { echo "FAIL: resolve sizechart printed '$out_lc', want 'sizeus' (case-insensitive)"; exit 1; }

echo "PASS: resolve-alias-to-canonical"
```

---
*This document follows the https://specscore.md/scenario-specification*
