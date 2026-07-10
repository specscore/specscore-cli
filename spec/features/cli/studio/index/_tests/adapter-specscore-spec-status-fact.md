---
format: https://specscore.md/scenario-specification
---

# Rehearse: spec-status-fact

**Status:** pending
**Verifies:** cli/studio/index#ac:spec-status-fact (REQ: adapter-specscore)

Scenario source: [../README.md](../README.md) → `### AC: spec-status-fact`.

Given a fixture repo whose `spec/features/x/README.md` has `**Status:** Approved`, indexed in ecosystem `demo`, when I run `specscore studio index` and then `specscore studio facts --predicate has-status --format json`, then the JSON contains a fact with subject ending `#x`, object `Approved`, evidence_class `declared`, an evidence_pointer to that README path, adapter id `specscore` with a non-empty version, a non-empty `observed_at`, and ecosystem `demo`.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/index#ac:spec-status-fact
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given a fixture repo whose spec/features/x/README.md has **Status:** Approved,
# indexed in ecosystem demo
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"
mkdir -p repo-a/spec/features/x
cat > repo-a/specscore.yaml <<'YAML'
project:
  title: Fixture Repo
YAML
cat > repo-a/spec/features/x/README.md <<'MD'
# Feature: X

**Status:** Approved
MD
cat > studio.yaml <<'YAML'
name: demo
repos:
  - repo-a
YAML

# When I run `specscore studio index`
"$SPECSCORE" studio index >/dev/null

# And then `specscore studio facts --predicate has-status --format json`
json="$("$SPECSCORE" studio facts --predicate has-status --format json)"

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

# Then the JSON contains a fact with subject ending #x
require '"subject": "repo-a#x"' "subject ending #x"
# object Approved
require '"object": "Approved"' "object Approved"
# evidence_class declared
require '"evidence_class": "declared"' "evidence_class declared"
# an evidence_pointer to that README path
require '"evidence_pointer": "spec/features/x/README.md"' "evidence_pointer to the README"
# adapter id specscore with a non-empty version
require '"id": "specscore"' "adapter id specscore"
require '"version": "' "an adapter version field"
forbid '"version": ""' "an empty adapter version"
# a non-empty observed_at
require '"observed_at": "' "an observed_at field"
forbid '"observed_at": ""' "an empty observed_at"
# and ecosystem demo
require '"ecosystem": "demo"' "ecosystem demo"

echo "PASS: spec-status-fact"
```

---
*This document follows the https://specscore.md/scenario-specification*
