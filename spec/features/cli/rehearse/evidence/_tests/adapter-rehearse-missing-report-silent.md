---
format: https://specscore.md/scenario-specification
---

# Rehearse: missing-report-silent

**Status:** pending
**Verifies:** cli/rehearse/evidence#ac:missing-report-silent (REQ: adapter-rehearse)

Scenario source: [../README.md](../README.md) → `### AC: missing-report-silent`.

Given a fixture repo with no `.specscore/rehearse/` directory, when I run `specscore studio index`, then the command exits 0 with no warning from adapter `rehearse` and zero `verified-behavior` facts.

```bash
#!/usr/bin/env bash
# Rehearse: cli/rehearse/evidence#ac:missing-report-silent
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given a fixture repo with no .specscore/rehearse/ directory
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"
mkdir -p repo-a/spec/features/x
cat > repo-a/specscore.yaml <<'YAML'
project:
  title: Fixture Repo
YAML
cat > repo-a/spec/features/x/README.md <<'MD'
# Feature: X

**Status:** Approved
MD
cat > studio.yaml <<'YAML'
name: demo
repos:
  - repo-a
YAML

# Confirm no .specscore/rehearse directory exists
[ ! -d repo-a/.specscore/rehearse ] \
  || { echo "FAIL: setup error — .specscore/rehearse exists"; exit 1; }

# When I run `specscore studio index`
set +e
out="$("$SPECSCORE" studio index 2>&1)"
exit_code=$?
set -e

# Then the command exits 0
[ "$exit_code" -eq 0 ] || { echo "FAIL: exit code $exit_code, want 0; output: $out"; exit 1; }

# And no warning from adapter rehearse
case "$out" in
  *"[rehearse]"*) echo "FAIL: output has unexpected rehearse warning: $out"; exit 1 ;;
  *) ;;
esac
echo "$out" | grep -q "Warnings: 0" \
  || { echo "FAIL: summary shows unexpected warnings: $out"; exit 1; }

# And zero verified-behavior facts
count="$("$SPECSCORE" studio facts --class verified-behavior --count)"
[ "$count" -eq 0 ] || { echo "FAIL: verified-behavior fact count $count, want 0"; exit 1; }

echo "PASS: missing-report-silent"
```

---
*This document follows the https://specscore.md/scenario-specification*
