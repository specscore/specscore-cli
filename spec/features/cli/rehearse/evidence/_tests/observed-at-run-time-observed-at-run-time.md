---
format: https://specscore.md/scenario-specification
---

# Rehearse: observed-at-run-time

**Status:** pending
**Verifies:** cli/rehearse/evidence#ac:observed-at-run-time (REQ: observed-at-run-time)

Scenario source: [../README.md](../README.md) → `### AC: observed-at-run-time`.

Given a fixture repo whose report's `started_at` is `2026-01-02T03:04:05Z`, when I run `specscore studio index` and then `specscore studio facts --class verified-behavior --format json`, then every rehearse-adapter fact's `observed_at` is `2026-01-02T03:04:05Z` while facts from other adapters carry the index run's timestamp.

```bash
#!/usr/bin/env bash
# Rehearse: cli/rehearse/evidence#ac:observed-at-run-time
# Requires: specscore on PATH (override with $SPECSCORE), python3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given a fixture repo whose report's started_at is 2026-01-02T03:04:05Z
# (also include a spec status fact so other-adapter observed_at is queryable)
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
  "git_sha": "abc123",
  "git_dirty": false,
  "started_at": "2026-01-02T03:04:05Z",
  "scenarios": [
    {
      "file": "spec/features/x/_tests/s.md",
      "status": "pass",
      "verifies": ["x#ac:y"],
      "duration_ms": 42,
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

# And then `specscore studio facts --class verified-behavior --format json`
vb_json="$("$SPECSCORE" studio facts --class verified-behavior --format json)"
# Also query other-adapter facts to check their observed_at differs
other_json="$("$SPECSCORE" studio facts --predicate has-status --format json)"

python3 - <<PY
import json

vb_facts = json.loads("""$vb_json""")
other_facts = json.loads("""$other_json""")

expected_ts = "2026-01-02T03:04:05Z"

# Every rehearse-adapter fact's observed_at must equal the report's started_at
for f in vb_facts:
    assert f.get("observed_at") == expected_ts, \
        f"FAIL: rehearse fact observed_at is {f.get('observed_at')!r}, want {expected_ts!r}: {f}"

# Facts from other adapters must carry a different (index-run) timestamp
assert other_facts, "FAIL: no other-adapter facts found"
for f in other_facts:
    assert f.get("observed_at") != expected_ts, \
        f"FAIL: other-adapter fact has same timestamp {expected_ts!r} as rehearse: {f}"
PY

echo "PASS: observed-at-run-time"
```

---
*This document follows the https://specscore.md/scenario-specification*
