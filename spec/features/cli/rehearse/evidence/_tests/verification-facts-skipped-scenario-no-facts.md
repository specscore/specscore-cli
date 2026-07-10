---
format: https://specscore.md/scenario-specification
---

# Rehearse: skipped-scenario-no-facts

**Status:** pending
**Verifies:** cli/rehearse/evidence#ac:skipped-scenario-no-facts (REQ: verification-facts)

Scenario source: [../README.md](../README.md) → `### AC: skipped-scenario-no-facts`.

Given a fixture repo whose report lists one scenario with status `skipped` and one with status `no-steps`, both with non-empty `verifies`, when I run `specscore studio index` and then `specscore studio facts --class verified-behavior --count`, then the count is 0.

```bash
#!/usr/bin/env bash
# Rehearse: cli/rehearse/evidence#ac:skipped-scenario-no-facts
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given a fixture repo whose report lists one scenario with status skipped
# and one with status no-steps, both with non-empty verifies
# (also include a specscore fact so the store is non-empty and --count works)
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"
mkdir -p repo-a/.specscore/rehearse repo-a/spec/features/x
cat > repo-a/specscore.yaml <<'YAML'
project:
  title: Fixture Repo
YAML
cat > repo-a/spec/features/x/README.md <<'MD'
# Feature: X

**Status:** Approved
MD
cat > repo-a/.specscore/rehearse/latest.json <<'JSON'
{
  "runner_version": "0.4.0",
  "git_sha": "",
  "git_dirty": false,
  "started_at": "2026-01-02T03:04:05Z",
  "scenarios": [
    {
      "file": "spec/features/x/_tests/skipped.md",
      "status": "skipped",
      "verifies": ["x#ac:y"],
      "duration_ms": 0,
      "bag": {},
      "steps": []
    },
    {
      "file": "spec/features/x/_tests/nosteps.md",
      "status": "no-steps",
      "verifies": ["x#ac:y"],
      "duration_ms": 0,
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

# And then `specscore studio facts --class verified-behavior --count`
count="$("$SPECSCORE" studio facts --class verified-behavior --count)"

# Then the count is 0
[ "$count" -eq 0 ] || { echo "FAIL: verified-behavior fact count $count, want 0"; exit 1; }

echo "PASS: skipped-scenario-no-facts"
```

---
*This document follows the https://specscore.md/scenario-specification*
