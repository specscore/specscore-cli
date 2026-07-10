---
format: https://specscore.md/scenario-specification
---

# Rehearse: report-out-on-failure

**Status:** pending
**Verifies:** cli/rehearse/evidence#ac:report-out-on-failure (REQ: report-out)

Scenario source: [../README.md](../README.md) → `### AC: report-out-on-failure`.

Given a scenario file whose bash block exits non-zero, when I run `specscore rehearse run <file> --report-out out/report.json`, then the command exits 1 and `out/report.json` exists with that scenario's status `fail`.

```bash
#!/usr/bin/env bash
# Rehearse: cli/rehearse/evidence#ac:report-out-on-failure
# Requires: specscore on PATH (override with $SPECSCORE), python3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given a scenario file whose bash block exits non-zero
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"
fence='```'
cat > failing.md <<MD
---
format: https://specscore.md/scenario-specification
---

# Rehearse: failing fixture

**Status:** pending
**Verifies:** demo/fixture#ac:fails

${fence}bash
exit 1
${fence}
MD

# When I run `specscore rehearse run <file> --report-out out/report.json`
set +e
"$SPECSCORE" rehearse run failing.md --report-out out/report.json >/dev/null 2>&1
exit_code=$?
set -e

# Then the command exits 1
[ "$exit_code" -eq 1 ] || { echo "FAIL: exit code $exit_code, want 1"; exit 1; }

# And out/report.json exists with that scenario's status fail
[ -f out/report.json ] || { echo "FAIL: out/report.json does not exist"; exit 1; }

python3 - out/report.json <<'PY'
import json, sys

with open(sys.argv[1]) as f:
    env = json.load(f)

assert isinstance(env.get("scenarios"), list) and env["scenarios"], \
    f"FAIL: scenarios is not a non-empty array: {env}"
first = env["scenarios"][0]
assert first.get("status") == "fail", \
    f"FAIL: scenario status is {first.get('status')!r}, want fail"
PY

echo "PASS: report-out-on-failure"
```

---
*This document follows the https://specscore.md/scenario-specification*
