---
format: https://specscore.md/scenario-specification
---

# Rehearse: json-report-shape

**Status:** pending
**Verifies:** cli/rehearse/run#ac:json-report-shape (REQ: run-report)

Scenario source: [../README.md](../README.md) → `### AC: json-report-shape`.

Given any single passing scenario, when I run `specscore rehearse run <file> --format json`, then the output is valid JSON whose first element has `file`, `status`, `verifies`, `duration_ms`, and a non-empty `steps` array with `kind` and `status`.

```bash
#!/usr/bin/env bash
# Rehearse: cli/rehearse/run#ac:json-report-shape
# Requires: specscore on PATH (override with $SPECSCORE), python3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given any single passing scenario (the inner fence is assembled via $fence
# so this file stays parseable).
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"
fence='```'
cat > passing.md <<MD
# Rehearse: passing fixture

**Status:** pending
**Verifies:** demo/fixture#ac:shape

${fence}bash
echo "shape ok"
${fence}
MD

# When I run `specscore rehearse run passing.md --format json`
set +e
"$SPECSCORE" rehearse run passing.md --format json > report.json 2>stderr.log
exit_code=$?
set -e
[ "$exit_code" -eq 0 ] || { echo "FAIL: exit code $exit_code, want 0; stderr: $(cat stderr.log)"; exit 1; }

# Then the output is valid JSON whose first element has `file`, `status`,
# `verifies`, `duration_ms`, and a non-empty `steps` array with `kind` and
# `status` (plus the REQ run-report `bag`).
python3 - report.json <<'PY'
import json, sys

with open(sys.argv[1]) as f:
    reports = json.load(f)

assert isinstance(reports, list) and reports, "FAIL: report is not a non-empty JSON array"
first = reports[0]
for key in ("file", "status", "verifies", "duration_ms", "bag", "steps"):
    assert key in first, f"FAIL: first element lacks {key!r}: {first}"
assert first["status"] == "pass", f"FAIL: status is {first['status']!r}, want pass"
assert isinstance(first["verifies"], list), "FAIL: verifies is not an array"
assert isinstance(first["duration_ms"], int), "FAIL: duration_ms is not an integer"
assert isinstance(first["bag"], dict), "FAIL: bag is not an object"
steps = first["steps"]
assert isinstance(steps, list) and steps, "FAIL: steps is not a non-empty array"
for key in ("kind", "status"):
    assert key in steps[0], f"FAIL: step entry lacks {key!r}: {steps[0]}"
PY

echo "PASS: json-report-shape"
```

---
*This document follows the https://specscore.md/scenario-specification*
