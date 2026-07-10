---
format: https://specscore.md/scenario-specification
---

# Rehearse: probe-without-index-errors

**Status:** pass-capable
**Verifies:** cli/studio/probe#ac:probe-without-index-errors (REQ: probe-verb)

Scenario source: [../README.md](../README.md) → `### AC: probe-without-index-errors`.

Given a workspace directory where `studio index` has never run, when I run `specscore studio probe`, then the command exits 2 with a message naming the expected store path and suggesting `specscore studio index`.

No seams are needed: the missing-store guard fires before any network or exec
I/O.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:probe-without-index-errors
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given a workspace directory where `studio index` has never run.
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"
mkdir repo-a
cat > studio.yaml <<'YAML'
name: demo
repos:
  - repo-a
YAML

# When I run `specscore studio probe`.
set +e
stderr="$("$SPECSCORE" studio probe 2>&1 >/dev/null)"
exit_code=$?
set -e

# Then the command exits 2.
[ "$exit_code" -eq 2 ] || { echo "FAIL: exit code $exit_code, want 2; stderr: $stderr"; exit 1; }

# With a message naming the expected store path (accept logical or physical cwd).
expected_logical="$(pwd)/.specscore-studio/facts.db"
expected_physical="$(pwd -P)/.specscore-studio/facts.db"
case "$stderr" in
  *"$expected_logical"* | *"$expected_physical"*) ;;
  *) echo "FAIL: message does not name expected store path $expected_logical: $stderr"; exit 1 ;;
esac

# And suggesting `studio index`.
case "$stderr" in
  *"studio index"*) ;;
  *) echo "FAIL: message does not suggest studio index: $stderr"; exit 1 ;;
esac

echo "PASS: probe-without-index-errors"
```

---
*This document follows the https://specscore.md/scenario-specification*
