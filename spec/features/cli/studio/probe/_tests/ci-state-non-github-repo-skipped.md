---
format: https://specscore.md/scenario-specification
---

# Rehearse: non-github-repo-skipped

**Status:** pass-capable
**Verifies:** cli/studio/probe#ac:non-github-repo-skipped (REQ: ci-state)

Scenario source: [../README.md](../README.md) → `### AC: non-github-repo-skipped`.

Given an indexed workspace repo with a `git` PATH shim where `git remote get-url origin` fails (no remote configured), when I run `specscore studio probe --kind ci`, then the command exits 0, the run summary contains a per-repo notice that the repo was skipped for having no GitHub remote, and no `ci-status` fact is written for it.

Seam note: the plan pins the git/gh seams to PATH shims. The fake `git` exits
non-zero for `remote get-url origin` (modelling a repo with no origin remote),
and a fake `gh` is present so the ci kind is not skipped upfront for gh-absence
— the skip we assert is the per-repo "no GitHub remote" notice, not the kind-
level gh-absent skip. The JSON run summary's `warnings` array is asserted to
carry the notice, and no `ci-status` fact is written.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:non-github-repo-skipped
# Requires: specscore on PATH (override with $SPECSCORE), python3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"

# Given an indexed workspace repo (a spec feature makes the store non-empty).
mkdir -p widget/spec/features/x
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

# PATH shims: `git remote get-url origin` fails (no origin remote); a `gh` is
# present so the ci kind is not skipped for gh-absence.
mkdir -p shims
cat > shims/git <<'SH'
#!/usr/bin/env bash
# No origin remote configured — exit non-zero for any git invocation.
exit 2
SH
cat > shims/gh <<'SH'
#!/usr/bin/env bash
# Present so LookPath(gh) succeeds; it should never be reached for this repo.
exit 0
SH
chmod +x shims/git shims/gh

# When I run `specscore studio probe --kind ci` (json summary to assert warnings).
set +e
summary="$(PATH="$workdir/shims:$PATH" "$SPECSCORE_ABS" studio probe --kind ci --format json 2>&1)"
probe_exit=$?
set -e
[ "$probe_exit" -eq 0 ] || { echo "FAIL: probe exited $probe_exit: $summary"; exit 1; }

# Then the run summary carries a per-repo "no GitHub remote" skip notice.
SUMMARY_JSON="$summary" python3 - <<'PY'
import json, os

s = json.loads(os.environ["SUMMARY_JSON"])
warnings = s.get("warnings", [])
skip = [w for w in warnings if "widget" in w and "skipped" in w and "remote" in w]
assert skip, f"FAIL: no per-repo skip notice for the non-GitHub repo; warnings: {warnings}"
assert s.get("facts_written", 0) == 0, f"FAIL: expected no facts written; got {s}"
PY

# And no ci-status fact is written for it.
count="$("$SPECSCORE_ABS" studio facts --predicate ci-status --count)"
[ "$count" -eq 0 ] || { echo "FAIL: expected 0 ci-status facts, got $count"; exit 1; }

echo "PASS: non-github-repo-skipped"
```

---
*This document follows the https://specscore.md/scenario-specification*
