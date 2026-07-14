---
format: https://specscore.md/scenario-specification
---

# Rehearse: ingr-export-counts

**Status:** pending
**Verifies:** cli/studio/index#ac:ingr-export-counts (REQ: ingr-export)

Scenario source: [../README.md](../README.md) → `### AC: ingr-export-counts`.

Given an indexed two-repo workspace, when I compare each repo's INGR record count under `<workspace-dir>/.specscore-studio/ingr/` with that repo's fact count in the index summary, then the counts are equal for every repo.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/index#ac:ingr-export-counts
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given an indexed two-repo workspace (one spec-tree repo, one repo with a
# codegraph snapshot plus a go.mod, so the per-repo fact counts differ)
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"
mkdir -p repo-spec/spec/features/x
cat > repo-spec/specscore.yaml <<'YAML'
project:
  title: Spec Fixture Repo
YAML
cat > repo-spec/spec/features/x/README.md <<'MD'
# Feature: X

**Status:** Approved
MD
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
cat > repo-graph/go.mod <<'MOD'
module example.com/graph
MOD
cat > studio.yaml <<'YAML'
name: demo
repos:
  - repo-spec
  - repo-graph
YAML
out="$("$SPECSCORE" studio index --workspace studio.yaml)"

# When I compare each repo's INGR record count under
# <workspace-dir>/.specscore-studio/ingr/ with that repo's fact count in the
# index summary, then the counts are equal for every repo
for slug in repo-spec repo-graph; do
  summary_count="$(printf '%s\n' "$out" \
    | sed -n "s|^  .*/$slug: \([0-9][0-9]*\) facts.*|\1|p")"
  [ -n "$summary_count" ] \
    || { echo "FAIL: summary has no per-repo fact count for $slug: $out"; exit 1; }

  # Local-only repository IDs include an absolute-path hash and are used as
  # nested export paths (`local/<basename>-<hash>/facts.ingr`). Resolve the one
  # recordset for this fixture basename without hard-coding its temp-dir hash.
  recordset="$(find .specscore-studio/ingr/local -type f \
    -path "*/$slug-*/facts.ingr" -print)"
  recordset_count="$(printf '%s\n' "$recordset" | sed '/^$/d' | wc -l | tr -d ' ')"
  [ "$recordset_count" -eq 1 ] \
    || { echo "FAIL: expected one INGR recordset for $slug, got $recordset_count: $recordset"; exit 1; }
  record_count="$(tail -n 1 "$recordset" \
    | sed -n 's|^# \([0-9][0-9]*\) records$|\1|p')"
  [ -n "$record_count" ] \
    || { echo "FAIL: $recordset has no '# N records' trailer"; exit 1; }

  [ "$record_count" -eq "$summary_count" ] \
    || { echo "FAIL: $slug INGR record count $record_count != summary fact count $summary_count"; exit 1; }
done

# Guard against a vacuous pass: the fixture repos contribute facts.
repo_spec_count="$(printf '%s\n' "$out" | sed -n "s|^  .*/repo-spec: \([0-9][0-9]*\) facts.*|\1|p")"
repo_graph_count="$(printf '%s\n' "$out" | sed -n "s|^  .*/repo-graph: \([0-9][0-9]*\) facts.*|\1|p")"
[ "$repo_spec_count" -eq 1 ] \
  || { echo "FAIL: repo-spec fact count $repo_spec_count, want 1"; exit 1; }
[ "$repo_graph_count" -eq 2 ] \
  || { echo "FAIL: repo-graph fact count $repo_graph_count, want 2 (imports + publishes)"; exit 1; }

echo "PASS: ingr-export-counts"
```

---
*This document follows the https://specscore.md/scenario-specification*
