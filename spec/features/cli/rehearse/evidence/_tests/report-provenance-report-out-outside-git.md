---
format: https://specscore.md/scenario-specification
---

# Rehearse: report-out-outside-git

**Status:** pending
**Verifies:** cli/rehearse/evidence#ac:report-out-outside-git (REQ: report-provenance)

Scenario source: [../README.md](../README.md) → `### AC: report-out-outside-git`.

Given a passing scenario file in a directory that is not inside any git work tree, when I run `specscore rehearse run <file> --report-out out/report.json`, then the command exits 0 and the report's `git_sha` is empty and `git_dirty` is false.

```bash
#!/usr/bin/env bash
# Rehearse: cli/rehearse/evidence#ac:report-out-outside-git
# Requires: specscore on PATH (override with $SPECSCORE), python3, git.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given a passing scenario file in a directory that is not inside any git
# work tree (use /tmp which is outside any project git repo)
workdir="$(mktemp -d /tmp/rehearse-outside-git-XXXXXX)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"

# Verify the directory is not inside a git work tree
if git rev-parse --is-inside-work-tree 2>/dev/null; then
  echo "FAIL: setup error — $workdir is inside a git work tree"; exit 1
fi

fence='```'
cat > passing.md <<MD
---
format: https://specscore.md/scenario-specification
---

# Rehearse: passing fixture

**Status:** pending
**Verifies:** demo/fixture#ac:outside-git

${fence}bash
echo "outside git ok"
${fence}
MD

# When I run `specscore rehearse run <file> --report-out out/report.json`
"$SPECSCORE" rehearse run passing.md --report-out out/report.json >/dev/null
exit_code=$?

# Then the command exits 0
[ "$exit_code" -eq 0 ] || { echo "FAIL: exit code $exit_code, want 0"; exit 1; }

# And the report's git_sha is empty and git_dirty is false
python3 - out/report.json <<'PY'
import json, sys

with open(sys.argv[1]) as f:
    env = json.load(f)

sha = env.get("git_sha", "NOT_PRESENT")
assert sha == "", f"FAIL: git_sha is {sha!r}, want empty string"
dirty = env.get("git_dirty", "NOT_PRESENT")
assert dirty is False, f"FAIL: git_dirty is {dirty!r}, want false"
PY

echo "PASS: report-out-outside-git"
```

---
*This document follows the https://specscore.md/scenario-specification*
