---
format: https://specscore.md/scenario-specification
---

# Rehearse: dtql-counts-facts

**Status:** pending
**Verifies:** cli/rehearse/run#ac:dtql-counts-facts (REQ: dtql-block)

Scenario source: [../README.md](../README.md) → `### AC: dtql-counts-facts`.

Given a SQLite fact store produced by `specscore studio index` on a fixture workspace and a scenario whose dtql block selects facts with `-- assert-rows:` equal to the store's known fact count, when I run `specscore rehearse run <that-file>`, then the command exits 0 and the scenario is `pass`.

```bash
#!/usr/bin/env bash
# Rehearse: cli/rehearse/run#ac:dtql-counts-facts
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given a fixture workspace (one repo with a committed codegraph/ snapshot)
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

# And a SQLite fact store produced by `specscore studio index` on it
"$SPECSCORE" studio index >/dev/null
store="${workdir}/.specscore-studio/facts.db"
[ -f "$store" ] || { echo "FAIL: studio index produced no fact store at $store"; exit 1; }

# And the store's known fact count
count="$("$SPECSCORE" studio facts --count)"
[ "$count" -gt 0 ] || { echo "FAIL: fact store is empty"; exit 1; }

# And a scenario whose dtql block selects facts with `-- assert-rows:` equal
# to that count (the dtql block runs in a scenario-scoped temp dir, so db=
# carries the store's absolute path; the inner fence is assembled via $fence
# so this file stays parseable).
fence='```'
cat > scenario.md <<MD
# Rehearse: dtql fixture

**Status:** pending
**Verifies:** demo/fixture#ac:dtql-count

${fence}dtql db=${store}
from:
  name: facts
-- assert-rows: ${count}
${fence}
MD

# When I run `specscore rehearse run scenario.md`
set +e
out="$("$SPECSCORE" rehearse run scenario.md 2>&1)"
exit_code=$?
set -e

# Then the command exits 0
[ "$exit_code" -eq 0 ] || { echo "FAIL: exit code $exit_code, want 0; output: $out"; exit 1; }

# And the scenario is `pass`
printf '%s\n' "$out" | grep -qE '^pass[[:space:]]+.*scenario\.md' \
  || { echo "FAIL: report does not mark the scenario pass: $out"; exit 1; }
printf '%s\n' "$out" | grep -q '1 pass' \
  || { echo "FAIL: totals line does not count the pass: $out"; exit 1; }

echo "PASS: dtql-counts-facts"
```

---
*This document follows the https://specscore.md/scenario-specification*
