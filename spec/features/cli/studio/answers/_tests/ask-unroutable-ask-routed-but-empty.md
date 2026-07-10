---
format: https://specscore.md/scenario-specification
---

# Rehearse: ask-routed-but-empty

**Status:** pass-capable
**Verifies:** cli/studio/answers#ac:ask-routed-but-empty (REQ: ask-unroutable)

Scenario source: [../README.md](../README.md) → `### AC: ask-routed-but-empty`.

Given an indexed store with no `fronts` fact for `unknown.example`, when I run `specscore studio ask "who fronts unknown.example"`, then the command exits 3 with a message distinguishing routed-but-no-data from unroutable, and emits no citation-free answer.

Seam: the fixture store carries a `fronts` fact for a different domain, so the
`who-fronts` template routes but finds no fact for `unknown.example` — the
routed-but-empty (exit 3) path, distinct from unroutable (exit 1).

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:ask-routed-but-empty
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"

mkdir -p ops
cat > ops/domains.json <<'JSON'
{"domains":{"acme.app":{"cloudflare":{"workers":{"acme":"/*"}}}}}
JSON
cat > studio.yaml <<'YAML'
name: demo
repos:
  - ops
YAML
"$SPECSCORE_ABS" studio index >/dev/null

set +e
out="$("$SPECSCORE_ABS" studio ask "who fronts unknown.example" 2>&1)"
rc=$?
set -e
[ "$rc" -eq 3 ] || { echo "FAIL: exit code $rc, want 3 (routed-but-empty); out: $out"; exit 1; }

# Distinguishes routed-but-no-data from unroutable: names the routed template and
# the parameter it found no facts for, and does NOT print a template list.
case "$out" in
  *"routed to who-fronts"*|*"found no facts"*) ;;
  *) echo "FAIL: message does not distinguish routed-but-empty: $out"; exit 1 ;;
esac
case "$out" in
  *"no template matched"*) echo "FAIL: routed-but-empty must not claim unroutable: $out"; exit 1 ;;
  *) ;;
esac

echo "PASS: ask-routed-but-empty"
```

---
*This document follows the https://specscore.md/scenario-specification*
