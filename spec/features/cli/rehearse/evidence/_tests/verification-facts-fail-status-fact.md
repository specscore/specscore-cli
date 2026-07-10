---
format: https://specscore.md/scenario-specification
---

# Rehearse: fail-status-fact

**Status:** pending
**Verifies:** cli/rehearse/evidence#ac:fail-status-fact (REQ: verification-facts)

Scenario source: [../README.md](../README.md) → `### AC: fail-status-fact`.

Given a fixture repo whose report lists a scenario with status `fail` verifying `x#ac:y`, when I run `specscore studio index` and then `specscore studio facts --predicate has-verification-status --format json`, then the JSON contains a fact with subject `<repo-slug>#x#ac:y` and object `fail`.

```bash
#!/usr/bin/env bash
# Rehearse: cli/rehearse/evidence#ac:fail-status-fact
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given a fixture repo whose report lists a scenario with status fail
# verifying x#ac:y
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"
mkdir -p repo-a/.specscore/rehearse
cat > repo-a/.specscore/rehearse/latest.json <<'JSON'
{
  "runner_version": "0.4.0",
  "git_sha": "",
  "git_dirty": false,
  "started_at": "2026-01-02T03:04:05Z",
  "scenarios": [
    {
      "file": "spec/features/x/_tests/fail.md",
      "status": "fail",
      "verifies": ["x#ac:y"],
      "duration_ms": 17,
      "bag": {},
      "steps": []
    }
  ]
}
JSON
cat > studio.yaml <<'YAML'
name: demo
repos:
  - repo-a
YAML

# When I run `specscore studio index`
"$SPECSCORE" studio index >/dev/null

# And then `specscore studio facts --predicate has-verification-status --format json`
json="$("$SPECSCORE" studio facts --predicate has-verification-status --format json)"

require() { # require <needle> <label>
  case "$json" in
    *"$1"*) ;;
    *) echo "FAIL: JSON lacks $2 ($1): $json"; exit 1 ;;
  esac
}

# Then the JSON contains a fact with subject repo-a#x#ac:y and object fail
require '"subject": "repo-a#x#ac:y"' "subject repo-a#x#ac:y"
require '"object": "fail"' "object fail"

echo "PASS: fail-status-fact"
```

---
*This document follows the https://specscore.md/scenario-specification*
