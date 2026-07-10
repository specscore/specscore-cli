---
format: https://specscore.md/scenario-specification
---

# Rehearse: stale-filter-malformed-duration

**Status:** pass-capable
**Verifies:** cli/studio/probe#ac:stale-filter-malformed-duration (REQ: stale-filter)

Scenario source: [../README.md](../README.md) → `### AC: stale-filter-malformed-duration`.

Given any indexed store, when I run `specscore studio facts --stale notaduration`, then the command exits 2 with a message naming the invalid duration.

No seams are needed: the `--stale` duration is validated before the store is
queried.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:stale-filter-malformed-duration
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"

# Given any indexed store.
mkdir -p repo-a/spec/features/x
cat > repo-a/specscore.yaml <<'YAML'
project:
  title: A
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
"$SPECSCORE_ABS" studio index >/dev/null

# When I run `specscore studio facts --stale notaduration`.
set +e
stderr="$("$SPECSCORE_ABS" studio facts --stale notaduration 2>&1 >/dev/null)"
exit_code=$?
set -e

# Then the command exits 2.
[ "$exit_code" -eq 2 ] || { echo "FAIL: exit code $exit_code, want 2; stderr: $stderr"; exit 1; }

# With a message naming the invalid duration.
case "$stderr" in
  *"notaduration"*) ;;
  *) echo "FAIL: message does not name the invalid duration: $stderr"; exit 1 ;;
esac

echo "PASS: stale-filter-malformed-duration"
```

---
*This document follows the https://specscore.md/scenario-specification*
