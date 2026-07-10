---
format: https://specscore.md/scenario-specification
---

# Rehearse: missing-store-error

**Status:** pending
**Verifies:** cli/studio/index#ac:missing-store-error (REQ: facts-query)

Scenario source: [../README.md](../README.md) → `### AC: missing-store-error`.

Given a workspace directory where `studio index` has never run, when I run `specscore studio facts --predicate imports`, then the command exits 2 with a message naming the expected store path and suggesting `studio index`.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/index#ac:missing-store-error
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given a workspace directory where `studio index` has never run
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"
mkdir repo-a
cat > studio.yaml <<'YAML'
name: demo
repos:
  - repo-a
YAML

# When I run `specscore studio facts --predicate imports`
set +e
stderr="$("$SPECSCORE" studio facts --predicate imports 2>&1 >/dev/null)"
exit_code=$?
set -e

# Then the command exits 2
[ "$exit_code" -eq 2 ] || { echo "FAIL: exit code $exit_code, want 2"; exit 1; }

# With a message naming the expected store path
# (accept the logical or physical cwd — /var vs /private/var on macOS)
expected_logical="$(pwd)/.specscore-studio/facts.db"
expected_physical="$(pwd -P)/.specscore-studio/facts.db"
case "$stderr" in
  *"$expected_logical"* | *"$expected_physical"*) ;;
  *) echo "FAIL: message does not name expected store path $expected_logical: $stderr"; exit 1 ;;
esac

# And suggesting `studio index`
case "$stderr" in
  *"studio index"*) ;;
  *) echo "FAIL: message does not suggest studio index: $stderr"; exit 1 ;;
esac

echo "PASS: missing-store-error"
```

---
*This document follows the https://specscore.md/scenario-specification*
