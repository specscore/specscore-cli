---
format: https://specscore.md/scenario-specification
---

# Rehearse: ci-state-fact

**Status:** pass-capable
**Verifies:** cli/studio/probe#ac:ci-state-fact (REQ: ci-state)

Scenario source: [../README.md](../README.md) → `### AC: ci-state-fact`.

Given an indexed workspace repo `widget` whose `origin` remote is `https://github.com/acme/widget.git`, and `git`/`gh` PATH shims where `git remote get-url origin` returns that URL and `gh api` returns a latest default-branch run with conclusion `success`, when I run `specscore studio probe --kind ci` and then `specscore studio facts --predicate ci-status --format json`, then the command exits 0 and the JSON contains a fact with subject `widget` (the store's repo slug), object `success`, evidence_class `verified-behavior`, adapter id `probe-ci`, and an evidence_pointer naming the queried `gh api` path.

Seam note: the plan pins the git/gh seams to PATH-shim fake executables (no
cross-process Go-var stubbing). The fake `git` prints the GitHub remote URL; the
fake `gh` answers both `gh api repos/acme/widget --jq .default_branch` (→ `main`)
and `gh api repos/acme/widget/actions/runs?branch=main&per_page=1` (→ a run with
conclusion `success`). `$SPECSCORE` is resolved to an absolute path before the
shim dir is prepended to PATH, so only `git`/`gh` are shimmed. Every structural
assertion of the AC is proven exactly.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:ci-state-fact
# Requires: specscore on PATH (override with $SPECSCORE), python3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"

# Given an indexed workspace repo `widget` (a spec feature makes the store
# non-empty so probe does not treat it as a missing store).
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

# PATH shims: a fake `git` returning the GitHub origin remote, and a fake `gh`
# answering the default-branch and latest-run queries.
mkdir -p shims
cat > shims/git <<'SH'
#!/usr/bin/env bash
if [ "$1" = "remote" ] && [ "$2" = "get-url" ] && [ "$3" = "origin" ]; then
  echo "https://github.com/acme/widget.git"; exit 0
fi
exit 1
SH
cat > shims/gh <<'SH'
#!/usr/bin/env bash
if [ "$1" = "api" ]; then
  case "$2" in
    repos/acme/widget)                    echo "main"; exit 0 ;;
    repos/acme/widget/actions/runs*)      echo '{"workflow_runs":[{"conclusion":"success"}]}'; exit 0 ;;
  esac
fi
exit 1
SH
chmod +x shims/git shims/gh

# When I run `specscore studio probe --kind ci` with the shims on PATH.
set +e
probe_out="$(PATH="$workdir/shims:$PATH" "$SPECSCORE_ABS" studio probe --kind ci 2>&1)"
probe_exit=$?
set -e
[ "$probe_exit" -eq 0 ] || { echo "FAIL: probe exited $probe_exit: $probe_out"; exit 1; }

# And then `specscore studio facts --predicate ci-status --format json`.
json="$("$SPECSCORE_ABS" studio facts --predicate ci-status --format json)"

# Then the ci-status fact is fully formed.
FACTS_JSON="$json" python3 - <<'PY'
import json, os

facts = json.loads(os.environ["FACTS_JSON"])
match = [
    f for f in facts
    if f.get("subject") == "widget"
    and f.get("predicate") == "ci-status"
    and f.get("object") == "success"
    and f.get("evidence_class") == "verified-behavior"
    and f.get("adapter", {}).get("id") == "probe-ci"
    and "repos/acme/widget/actions/runs" in f.get("evidence_pointer", "")
]
assert match, f"FAIL: no probe-ci ci-status=success fact for `widget` naming the gh api path; got {facts}"
PY

echo "PASS: ci-state-fact"
```

---
*This document follows the https://specscore.md/scenario-specification*
