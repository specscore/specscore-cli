---
format: https://specscore.md/scenario-specification
---

# Rehearse: codegraph-import-fact

**Status:** pending
**Verifies:** cli/studio/index#ac:codegraph-import-fact (REQ: adapter-codegraph)

Scenario source: [../README.md](../README.md) → `### AC: codegraph-import-fact`.

Given a fixture repo with a committed `codegraph/` snapshot containing a package-import edge from `a` to `b`, when I run `specscore studio index` and then `specscore studio facts --predicate imports`, then the output contains one row with subject package `a`, object package `b`, evidence_class `derived`.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/index#ac:codegraph-import-fact
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given a fixture repo with a committed codegraph/ snapshot containing a
# package-import edge from a to b (CodeGrapher INGR recordsets)
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"
mkdir -p repo-a/codegraph/nodes repo-a/codegraph/edges
cat > repo-a/codegraph/nodes/nodes.ingr <<'INGR'
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
cat > repo-a/codegraph/edges/edges.ingr <<'INGR'
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
  - repo-a
YAML

# When I run `specscore studio index`
"$SPECSCORE" studio index >/dev/null

# And then `specscore studio facts --predicate imports`
out="$("$SPECSCORE" studio facts --predicate imports)"

# Then the output contains one row ...
count="$("$SPECSCORE" studio facts --predicate imports --count)"
if [ "$count" != "1" ]; then
  echo "FAIL: expected exactly 1 imports fact, got $count: $out"; exit 1
fi

# ... with subject package a, object package b, evidence_class derived
row="$(printf '%s\n' "$out" | grep -F 'imports')"
require() { # require <needle> <label>
  case "$row" in
    *"$1"*) ;;
    *) echo "FAIL: imports row lacks $2 ($1): $row"; exit 1 ;;
  esac
}
require '#a' "subject package a"
require '#b' "object package b"
require 'derived' "evidence_class derived"

echo "PASS: codegraph-import-fact"
```

---
*This document follows the https://specscore.md/scenario-specification*
