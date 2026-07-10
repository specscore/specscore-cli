---
format: https://specscore.md/scenario-specification
---

# Rehearse: index-two-repos

**Status:** pending
**Verifies:** cli/studio/index#ac:index-two-repos (REQ: workspace-config)

Scenario source: [../README.md](../README.md) → `### AC: index-two-repos`.

Given a `studio.yaml` naming ecosystem `demo` and listing two fixture repos, one with a `spec/` tree and one with a `codegraph/` snapshot, when I run `specscore studio index --workspace studio.yaml`, then the command exits 0, prints a summary with both repos and per-adapter fact counts, and `<workspace-dir>/.specscore-studio/facts.db` exists.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/index#ac:index-two-repos
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given a studio.yaml naming ecosystem demo and listing two fixture repos,
# one with a spec/ tree ...
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
# ... and one with a codegraph/ snapshot (CodeGrapher INGR recordsets)
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
cat > studio.yaml <<'YAML'
name: demo
repos:
  - repo-spec
  - repo-graph
YAML

# When I run `specscore studio index --workspace studio.yaml`
set +e
out="$("$SPECSCORE" studio index --workspace studio.yaml 2>&1)"
exit_code=$?
set -e

# Then the command exits 0
[ "$exit_code" -eq 0 ] || { echo "FAIL: exit code $exit_code, want 0; output: $out"; exit 1; }

require() { # require <needle> <label>
  case "$out" in
    *"$1"*) ;;
    *) echo "FAIL: summary lacks $2 ($1): $out"; exit 1 ;;
  esac
}

# And prints a summary with both repos ...
require 'Ecosystem: demo' "the ecosystem name"
require 'Repos: 2' "the repo count"
require "repo-spec: 1 facts" "the spec repo's per-repo line"
require "repo-graph: 1 facts" "the graph repo's per-repo line"

# ... and per-adapter fact counts (all four adapters, zero counts included)
require 'Facts by adapter:' "the per-adapter section"
require 'specscore: 1' "the specscore adapter count"
require 'codegraph: 1' "the codegraph adapter count"
require 'manifests: 0' "the manifests adapter count"
require 'registries: 0' "the registries adapter count"

# And <workspace-dir>/.specscore-studio/facts.db exists
[ -f .specscore-studio/facts.db ] \
  || { echo "FAIL: .specscore-studio/facts.db does not exist"; exit 1; }

echo "PASS: index-two-repos"
```

---
*This document follows the https://specscore.md/scenario-specification*
