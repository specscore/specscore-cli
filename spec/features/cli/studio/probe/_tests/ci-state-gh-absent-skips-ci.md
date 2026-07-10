---
format: https://specscore.md/scenario-specification
---

# Rehearse: gh-absent-skips-ci

**Status:** pass-capable
**Verifies:** cli/studio/probe#ac:gh-absent-skips-ci (REQ: ci-state)

Scenario source: [../README.md](../README.md) → `### AC: gh-absent-skips-ci`.

Given an indexed store and no `gh` binary on `PATH`, when I run `specscore studio probe --kind ci`, then the command exits 0 and the run summary reports one warning that the `ci` kind was skipped because `gh` was not found, and no `ci-status` facts are written.

Seam note: "no `gh` on PATH" is realized honestly by invoking the probe with a
PATH pointing at an empty shim directory (the CI image ships `gh`, so we must
narrow PATH rather than assume its absence). `$SPECSCORE` is resolved to an
absolute path first, so it still runs; the probe's upfront `gh` LookPath check
fails and the ci kind is skipped with exactly one warning. No `git` is needed —
the kind is skipped before any repo is examined.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:gh-absent-skips-ci
# Requires: specscore on PATH (override with $SPECSCORE), python3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"

# Given an indexed store (a spec feature makes it non-empty).
mkdir -p widget/spec/features/x emptybin
cat > widget/specscore.yaml <<'YAML'
project:
  title: Widget
YAML
cat > widget/spec/features/x/README.md <<'MD'
# Feature: X

**Status:** Approved
MD
cat > studio.yaml <<'YAML'
name: demo
repos:
  - widget
YAML
"$SPECSCORE_ABS" studio index >/dev/null

# When I run `specscore studio probe --kind ci` with an empty PATH — no `gh`.
set +e
summary="$(PATH="$workdir/emptybin" "$SPECSCORE_ABS" studio probe --kind ci --format json 2>&1)"
probe_exit=$?
set -e
[ "$probe_exit" -eq 0 ] || { echo "FAIL: probe exited $probe_exit: $summary"; exit 1; }

# Then exactly one warning names the ci kind skipped for a missing gh.
SUMMARY_JSON="$summary" python3 - <<'PY'
import json, os

s = json.loads(os.environ["SUMMARY_JSON"])
warnings = s.get("warnings", [])
gh_skips = [w for w in warnings if "ci" in w and "gh" in w and "skip" in w.lower()]
assert len(gh_skips) == 1, f"FAIL: expected exactly one gh-absent ci skip warning; warnings: {warnings}"
assert s.get("facts_written", 0) == 0, f"FAIL: expected no facts written; got {s}"
PY

# And no ci-status facts are written.
count="$("$SPECSCORE_ABS" studio facts --predicate ci-status --count)"
[ "$count" -eq 0 ] || { echo "FAIL: expected 0 ci-status facts, got $count"; exit 1; }

echo "PASS: gh-absent-skips-ci"
```

---
*This document follows the https://specscore.md/scenario-specification*
