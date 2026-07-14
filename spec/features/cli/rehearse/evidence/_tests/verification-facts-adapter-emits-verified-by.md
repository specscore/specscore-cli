---
format: https://specscore.md/scenario-specification
---

# Rehearse: adapter-emits-verified-by

**Status:** pending
**Verifies:** cli/rehearse/evidence#ac:adapter-emits-verified-by (REQ: verification-facts)

Scenario source: [../README.md](../README.md) → `### AC: adapter-emits-verified-by`.

Given a fixture repo whose `.specscore/rehearse/latest.json` reports scenario `spec/features/x/_tests/s.md` with status `pass` verifying `x#ac:y`, indexed in ecosystem `demo`, when I run `specscore studio index` and then `specscore studio facts --class verified-behavior --format json`, then the JSON contains a fact with subject `<repo-slug>#x#ac:y`, predicate `verified-by`, object `spec/features/x/_tests/s.md`, evidence_class `verified-behavior`, evidence_pointer `.specscore/rehearse/latest.json`, and adapter id `rehearse` with a non-empty version.

```bash
#!/usr/bin/env bash
# Rehearse: cli/rehearse/evidence#ac:adapter-emits-verified-by
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given a fixture repo whose .specscore/rehearse/latest.json reports
# scenario spec/features/x/_tests/s.md with status pass verifying x#ac:y,
# indexed in ecosystem demo
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"
mkdir -p repo-a/.specscore/rehearse
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
json="$("$SPECSCORE" studio facts --class verified-behavior --format json)"

require() { # require <needle> <label>
  case "$json" in
    *"$1"*) ;;
    *) echo "FAIL: JSON lacks $2 ($1): $json"; exit 1 ;;
  esac
}
forbid() { # forbid <needle> <label>
  case "$json" in
    *"$1"*) echo "FAIL: JSON has $2 ($1): $json"; exit 1 ;;
    *) ;;
  esac
}

# Then the JSON contains a local-only repository ID whose entity suffix is
# x#ac:y. The hash derives from the fixture's temporary absolute path.
require '"subject": "local/repo-a-' "local-only repository ID"
require '#x#ac:y"' "entity suffix x#ac:y"
# predicate verified-by
require '"predicate": "verified-by"' "predicate verified-by"
# object spec/features/x/_tests/s.md
require '"object": "spec/features/x/_tests/s.md"' "object scenario file path"
# evidence_class verified-behavior
require '"evidence_class": "verified-behavior"' "evidence_class verified-behavior"
# evidence_pointer .specscore/rehearse/latest.json
require '"evidence_pointer": ".specscore/rehearse/latest.json"' "evidence_pointer"
# adapter id rehearse with a non-empty version
require '"id": "rehearse"' "adapter id rehearse"
require '"version": "' "an adapter version field"
forbid '"version": ""' "an empty adapter version"

echo "PASS: adapter-emits-verified-by"
```

---
*This document follows the https://specscore.md/scenario-specification*
