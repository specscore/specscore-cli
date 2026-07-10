---
format: https://specscore.md/scenario-specification
---

# Rehearse: report-out-writes-envelope

**Status:** pending
**Verifies:** cli/rehearse/evidence#ac:report-out-writes-envelope (REQ: report-out)

Scenario source: [../README.md](../README.md) → `### AC: report-out-writes-envelope`.

Given a scenario file with a passing bash block inside a git work tree with a clean HEAD, when I run `specscore rehearse run <file> --report-out out/report.json`, then the command exits 0 and `out/report.json` is valid JSON with a non-empty `runner_version`, a non-empty `git_sha`, `git_dirty` false, a non-empty RFC 3339 `started_at`, and a `scenarios` array whose first element has `file`, `status` `pass`, `verifies`, `duration_ms`, and `steps`.

```bash
#!/usr/bin/env bash
# Rehearse: cli/rehearse/evidence#ac:report-out-writes-envelope
# Requires: specscore on PATH (override with $SPECSCORE), python3, git.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given a scenario file with a passing bash block inside a git work tree
# with a clean HEAD
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"
git init -q
git config user.email "test@rehearse.test"
git config user.name "Rehearse Test"
fence='```'
cat > passing.md <<MD
---
format: https://specscore.md/scenario-specification
---

# Rehearse: passing fixture

**Status:** pending
**Verifies:** demo/fixture#ac:shape

${fence}bash
echo "envelope ok"
${fence}
MD
git add passing.md
git commit -q -m "init"

# When I run `specscore rehearse run <file> --report-out out/report.json`
"$SPECSCORE" rehearse run passing.md --report-out out/report.json >/dev/null
exit_code=$?

# Then the command exits 0
[ "$exit_code" -eq 0 ] || { echo "FAIL: exit code $exit_code, want 0"; exit 1; }

# And out/report.json is valid JSON with the required envelope fields
python3 - out/report.json <<'PY'
import json, sys, re

with open(sys.argv[1]) as f:
    env = json.load(f)

# non-empty runner_version
assert env.get("runner_version"), f"FAIL: runner_version is empty or missing: {env}"
# non-empty git_sha
assert env.get("git_sha"), f"FAIL: git_sha is empty or missing: {env}"
# git_dirty false
assert env.get("git_dirty") is False, f"FAIL: git_dirty is not false: {env}"
# non-empty RFC 3339 started_at
sa = env.get("started_at", "")
assert sa, f"FAIL: started_at is empty or missing: {env}"
assert re.match(r'^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}', sa), \
    f"FAIL: started_at {sa!r} is not RFC 3339"
# scenarios array
assert isinstance(env.get("scenarios"), list) and env["scenarios"], \
    f"FAIL: scenarios is not a non-empty array: {env}"
first = env["scenarios"][0]
for key in ("file", "status", "verifies", "duration_ms", "steps"):
    assert key in first, f"FAIL: first scenario lacks {key!r}: {first}"
assert first["status"] == "pass", f"FAIL: scenario status is {first['status']!r}, want pass"
PY

echo "PASS: report-out-writes-envelope"
```

---
*This document follows the https://specscore.md/scenario-specification*
