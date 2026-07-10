---
format: https://specscore.md/scenario-specification
---

# Rehearse: failing-scenario-fails-run

**Status:** pending
**Verifies:** cli/rehearse/run#ac:failing-scenario-fails-run (REQ: bash-block)

Scenario source: [../README.md](../README.md) → `### AC: failing-scenario-fails-run`.

Given a scenario file whose bash block exits non-zero, when I run `specscore rehearse run <that-file>`, then the command exits 1 and the report marks that scenario `fail`.

```bash
#!/usr/bin/env bash
# Rehearse: cli/rehearse/run#ac:failing-scenario-fails-run
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given a scenario file whose bash block exits non-zero
# (the inner fence is assembled via $fence so this file stays parseable).
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"
fence='```'
cat > broken.md <<MD
# Rehearse: broken fixture

**Status:** pending
**Verifies:** demo/fixture#ac:always-fails

${fence}bash
echo "about to fail"
exit 5
${fence}
MD

# When I run `specscore rehearse run broken.md`
set +e
out="$("$SPECSCORE" rehearse run broken.md 2>&1)"
exit_code=$?
set -e

# Then the command exits 1
[ "$exit_code" -eq 1 ] || { echo "FAIL: exit code $exit_code, want 1; output: $out"; exit 1; }

# And the report marks that scenario `fail`
printf '%s\n' "$out" | grep -qE '^fail[[:space:]]+.*broken\.md' \
  || { echo "FAIL: report does not mark the scenario fail: $out"; exit 1; }
printf '%s\n' "$out" | grep -q '1 fail' \
  || { echo "FAIL: totals line does not count the failure: $out"; exit 1; }

echo "PASS: failing-scenario-fails-run"
```

---
*This document follows the https://specscore.md/scenario-specification*
