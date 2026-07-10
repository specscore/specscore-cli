---
format: https://specscore.md/scenario-specification
---

# Rehearse: workspace-missing-error

**Status:** pending
**Verifies:** cli/studio/index#ac:workspace-missing-error (REQ: workspace-errors)

Scenario source: [../README.md](../README.md) → `### AC: workspace-missing-error`.

Given a directory containing no `studio.yaml`, when I run `specscore studio index`, then the command exits 2 and prints a one-line error naming the expected workspace path.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/index#ac:workspace-missing-error
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given a directory containing no studio.yaml
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"

# When I run `specscore studio index`
set +e
stderr="$("$SPECSCORE" studio index 2>&1 >/dev/null)"
exit_code=$?
set -e

# Then the command exits 2
[ "$exit_code" -eq 2 ] || { echo "FAIL: exit code $exit_code, want 2"; exit 1; }

# And prints a one-line error naming the expected workspace path
# (accept the logical or physical cwd — /var vs /private/var on macOS)
expected_logical="$(pwd)/studio.yaml"
expected_physical="$(pwd -P)/studio.yaml"
one_line="$(echo "$stderr" | grep "^workspace file not found:")" \
  || { echo "FAIL: stderr has no workspace-not-found error: $stderr"; exit 1; }
[ "$(echo "$one_line" | wc -l)" -eq 1 ] || { echo "FAIL: error is not one line"; exit 1; }
case "$one_line" in
  *"$expected_logical"* | *"$expected_physical"*) ;;
  *) echo "FAIL: error does not name expected workspace path $expected_logical: $one_line"; exit 1 ;;
esac

echo "PASS: workspace-missing-error"
```

---
*This document follows the https://specscore.md/scenario-specification*
