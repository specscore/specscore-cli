---
format: https://specscore.md/scenario-specification
---

# Rehearse: malformed-report-warns

**Status:** pending
**Verifies:** cli/rehearse/evidence#ac:malformed-report-warns (REQ: adapter-rehearse)

Scenario source: [../README.md](../README.md) → `### AC: malformed-report-warns`.

Given a fixture repo whose `.specscore/rehearse/latest.json` is not valid JSON and whose `go.mod` requires a module, when I run `specscore studio index`, then the command exits 0, the summary lists a warning naming the report file for adapter `rehearse`, and the manifests adapter's facts are queryable.

```bash
#!/usr/bin/env bash
# Rehearse: cli/rehearse/evidence#ac:malformed-report-warns
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given a fixture repo whose .specscore/rehearse/latest.json is not valid
# JSON and whose go.mod requires a module
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"
mkdir -p repo-a/.specscore/rehearse
cat > repo-a/.specscore/rehearse/latest.json <<'TXT'
this is not valid json {{{
TXT
cat > repo-a/go.mod <<'GOMOD'
module example.com/fixture

go 1.22

require example.com/dep v1.0.0
GOMOD
cat > studio.yaml <<'YAML'
name: demo
repos:
  - repo-a
YAML

# When I run `specscore studio index`
set +e
out="$("$SPECSCORE" studio index 2>&1)"
exit_code=$?
set -e

# Then the command exits 0
[ "$exit_code" -eq 0 ] || { echo "FAIL: exit code $exit_code, want 0; output: $out"; exit 1; }

# And the summary lists a warning naming the report file for adapter rehearse
case "$out" in
  *"rehearse"*) ;;
  *) echo "FAIL: output has no rehearse warning: $out"; exit 1 ;;
esac
case "$out" in
  *".specscore/rehearse/latest.json"*) ;;
  *) echo "FAIL: warning does not name the report file: $out"; exit 1 ;;
esac
echo "$out" | grep -q "Warnings: 1" \
  || { echo "FAIL: summary does not count the warning: $out"; exit 1; }

# And the manifests adapter's facts are queryable
count="$("$SPECSCORE" studio facts --predicate consumes --count)"
[ "$count" -ge 1 ] || { echo "FAIL: manifests fact count $count, want >= 1"; exit 1; }

echo "PASS: malformed-report-warns"
```

---
*This document follows the https://specscore.md/scenario-specification*
