---
format: https://specscore.md/scenario-specification
---

# Rehearse: corpus-green

**Status:** pending
**Verifies:** cli/rehearse/run#ac:corpus-green (REQ: studio-corpus-green)

Scenario source: [../README.md](../README.md) → `### AC: corpus-green`.

Given the specscore-cli repo checkout, when I run `specscore rehearse run spec/features/cli/studio/index/_tests --format json`, then the command exits 0 and the JSON lists 13 scenarios all with status `pass` and non-empty `verifies` arrays.

```bash
#!/usr/bin/env bash
# Rehearse: cli/rehearse/run#ac:corpus-green
# Requires: specscore on PATH (override with $SPECSCORE), python3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
# The nested invocation runs from the repo root while this step runs in a
# scenario-scoped temp dir, so resolve the binary to an absolute path first.
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }

# Given the specscore-cli repo checkout. Steps run in a scenario-scoped temp
# dir, so locate the checkout: $SPECSCORE_CLI_REPO wins when set; otherwise
# fall back to the working directory of the parent process — the runner (or
# shell) that launched this step from the repo root.
repo="${SPECSCORE_CLI_REPO:-}"
if [ -z "$repo" ]; then
  if [ -e "/proc/$PPID/cwd" ]; then
    repo="$(readlink "/proc/$PPID/cwd")"
  else
    repo="$(lsof -a -p "$PPID" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p' | head -n 1)"
  fi
fi
corpus="spec/features/cli/studio/index/_tests"
[ -n "$repo" ] && [ -d "$repo/$corpus" ] \
  || { echo "FAIL: cannot locate the specscore-cli checkout (looked in '$repo'); set \$SPECSCORE_CLI_REPO"; exit 1; }

# When I run `specscore rehearse run spec/features/cli/studio/index/_tests --format json`
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
set +e
(cd "$repo" && "$SPECSCORE_ABS" rehearse run "$corpus" --format json) \
  > "$workdir/report.json" 2> "$workdir/stderr.log"
exit_code=$?
set -e

# Then the command exits 0
[ "$exit_code" -eq 0 ] \
  || { echo "FAIL: exit code $exit_code, want 0; stderr: $(cat "$workdir/stderr.log"); report: $(cat "$workdir/report.json")"; exit 1; }

# And the JSON lists every studio-index scenario, all with status `pass` and
# non-empty `verifies` arrays (bump the count below when index scenarios change)
python3 - "$workdir/report.json" <<'PY'
import json, sys

with open(sys.argv[1]) as f:
    reports = json.load(f)

assert isinstance(reports, list), "FAIL: report is not a JSON array"
assert len(reports) == 13, f"FAIL: {len(reports)} scenarios, want 13"
for r in reports:
    assert r["status"] == "pass", f"FAIL: {r['file']} is {r['status']!r}, want pass: {r.get('detail', '')}"
    verifies = r["verifies"]
    assert isinstance(verifies, list) and verifies, f"FAIL: {r['file']} has an empty verifies array"
PY

echo "PASS: corpus-green"
```

---
*This document follows the https://specscore.md/scenario-specification*
