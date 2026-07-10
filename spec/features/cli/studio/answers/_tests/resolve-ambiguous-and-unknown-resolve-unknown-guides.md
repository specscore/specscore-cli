---
format: https://specscore.md/scenario-specification
---

# Rehearse: resolve-unknown-guides

**Status:** pass-capable
**Verifies:** cli/studio/answers#ac:resolve-unknown-guides (REQ: resolve-ambiguous-and-unknown)

Scenario source: [../README.md](../README.md) → `### AC: resolve-unknown-guides`.

Given any indexed store not containing the name `nonexistent`, when I run `specscore studio resolve nonexistent`, then the command exits 3 with a message naming what was searched (aliases and entity ids) and suggesting a `studio facts` exploration.

Seam: any non-empty fixture store suffices; the name `nonexistent` matches no
alias or entity id, so resolution is unknown (NotFound).

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:resolve-unknown-guides
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"

mkdir -p ops
cat > ops/ecosystem.yaml <<'YAML'
products:
  - id: sizeus
    brand: SizeChart
YAML
cat > studio.yaml <<'YAML'
name: demo
repos:
  - ops
YAML
"$SPECSCORE_ABS" studio index >/dev/null

set +e
stderr="$("$SPECSCORE_ABS" studio resolve nonexistent 2>&1 >/dev/null)"
rc=$?
set -e
[ "$rc" -eq 3 ] || { echo "FAIL: exit code $rc, want 3 (NotFound); stderr: $stderr"; exit 1; }

# Names what was searched (aliases + entity ids).
case "$stderr" in
  *alias*) ;;
  *) echo "FAIL: message does not name aliases as searched: $stderr"; exit 1 ;;
esac
# Suggests a studio facts exploration.
case "$stderr" in
  *"studio facts"*) ;;
  *) echo "FAIL: message does not suggest a studio facts exploration: $stderr"; exit 1 ;;
esac

echo "PASS: resolve-unknown-guides"
```

---
*This document follows the https://specscore.md/scenario-specification*
