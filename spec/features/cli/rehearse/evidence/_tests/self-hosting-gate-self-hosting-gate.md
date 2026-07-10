---
format: https://specscore.md/scenario-specification
---

# Rehearse: self-hosting-gate

**Status:** pending
**Verifies:** cli/rehearse/evidence#ac:self-hosting-gate (REQ: self-hosting-gate)

Scenario source: [../README.md](../README.md) → `### AC: self-hosting-gate`.

Given this repo after `specscore rehearse run spec/features/cli/studio/index/_tests --report-out .specscore/rehearse/latest.json` and a `studio.yaml` listing this repo, when I run `specscore studio index` and then `specscore studio facts --class verified-behavior --format json`, then the command exits 0 and the JSON contains at least one fact whose subject contains `cli/studio/index#ac:`.

```bash
#!/usr/bin/env bash
# Rehearse: cli/rehearse/evidence#ac:self-hosting-gate
# Requires: specscore on PATH (override with $SPECSCORE), python3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }

# Locate the specscore-cli repo root: $REPO_ROOT wins when set (CI passes it);
# otherwise use $SPECSCORE_CLI_REPO for compatibility with the corpus-green
# scenario; finally fall back to the parent process cwd.
repo="${REPO_ROOT:-${SPECSCORE_CLI_REPO:-}}"
if [ -z "$repo" ]; then
  if [ -e "/proc/$PPID/cwd" ]; then
    repo="$(readlink "/proc/$PPID/cwd")"
  else
    repo="$(lsof -a -p "$PPID" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p' | head -n 1)"
  fi
fi
corpus="spec/features/cli/studio/index/_tests"
[ -n "$repo" ] && [ -d "$repo/$corpus" ] \
  || { echo "FAIL: cannot locate the specscore-cli checkout (looked in '$repo'); set \$REPO_ROOT"; exit 1; }

# Given this repo after rehearse run ... --report-out .specscore/rehearse/latest.json
# Run from the repo root so relative paths in the report are repo-relative.
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
report_path="$repo/.specscore/rehearse/latest.json"
set +e
(cd "$repo" && "$SPECSCORE_ABS" rehearse run "$corpus" --report-out "$report_path") \
  > "$workdir/run.log" 2>&1
run_exit=$?
set -e
[ "$run_exit" -eq 0 ] \
  || { echo "FAIL: rehearse run exited $run_exit; log: $(cat "$workdir/run.log")"; exit 1; }
[ -f "$report_path" ] \
  || { echo "FAIL: report not written to $report_path"; exit 1; }

# And a studio.yaml listing this repo
cat > "$workdir/studio.yaml" <<YAML
name: specscore-cli-self-hosting
repos:
  - $repo
YAML

# When I run `specscore studio index`
set +e
"$SPECSCORE_ABS" studio index --workspace "$workdir/studio.yaml" > "$workdir/index.log" 2>&1
index_exit=$?
set -e
[ "$index_exit" -eq 0 ] \
  || { echo "FAIL: studio index exited $index_exit; log: $(cat "$workdir/index.log")"; exit 1; }

# And then `specscore studio facts --class verified-behavior --format json`
db="$workdir/.specscore-studio/facts.db"
set +e
json="$("$SPECSCORE_ABS" studio facts --db "$db" \
  --class verified-behavior --format json 2>"$workdir/facts.err")"
facts_exit=$?
set -e

# Then the command exits 0
[ "$facts_exit" -eq 0 ] \
  || { echo "FAIL: studio facts exited $facts_exit; err: $(cat "$workdir/facts.err")"; exit 1; }

# And the JSON contains at least one fact whose subject contains cli/studio/index#ac:
python3 - <<PY
import json, sys

facts = json.loads("""$json""")
assert isinstance(facts, list) and facts, f"FAIL: verified-behavior facts list is empty"
matches = [f for f in facts if "cli/studio/index#ac:" in f.get("subject", "")]
assert matches, (
    f"FAIL: no fact with 'cli/studio/index#ac:' in subject; "
    f"subjects: {[f.get('subject') for f in facts]}"
)
PY

echo "PASS: self-hosting-gate"
```

---
*This document follows the https://specscore.md/scenario-specification*
