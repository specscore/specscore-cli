---
format: https://specscore.md/scenario-specification
---

# Rehearse: standalone-run

**Status:** pending
**Verifies:** cli/rehearse/run#ac:standalone-run (REQ: scenario-discovery)

Scenario source: [../README.md](../README.md) → `### AC: standalone-run`.

Given a scenario file with a passing bash block in a directory containing no `specscore.yaml`, when I run `specscore rehearse run <that-file>`, then the command exits 0 and reports the scenario `pass`.

```bash
#!/usr/bin/env bash
# Rehearse: cli/rehearse/run#ac:standalone-run
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given a scenario file with a passing bash block in a directory containing
# no specscore.yaml (the inner fence is assembled via $fence so this file
# stays parseable).
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"
[ ! -f specscore.yaml ] || { echo "FAIL: fixture dir unexpectedly has specscore.yaml"; exit 1; }
fence='```'
cat > standalone.md <<MD
# Rehearse: standalone fixture

**Status:** pending
**Verifies:** demo/fixture#ac:standalone

${fence}bash
echo "standalone ok"
${fence}
MD

# When I run `specscore rehearse run standalone.md`
set +e
out="$("$SPECSCORE" rehearse run standalone.md 2>&1)"
exit_code=$?
set -e

# Then the command exits 0
[ "$exit_code" -eq 0 ] || { echo "FAIL: exit code $exit_code, want 0; output: $out"; exit 1; }

# And reports the scenario `pass`
printf '%s\n' "$out" | grep -qE '^pass[[:space:]]+.*standalone\.md' \
  || { echo "FAIL: report does not mark the scenario pass: $out"; exit 1; }
printf '%s\n' "$out" | grep -q '1 pass' \
  || { echo "FAIL: totals line does not count the pass: $out"; exit 1; }

echo "PASS: standalone-run"
```

---
*This document follows the https://specscore.md/scenario-specification*
