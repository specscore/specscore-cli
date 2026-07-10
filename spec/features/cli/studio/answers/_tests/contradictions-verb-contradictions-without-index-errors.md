---
format: https://specscore.md/scenario-specification
---

# Rehearse: contradictions-without-index-errors

**Status:** pass-capable
**Verifies:** cli/studio/answers#ac:contradictions-without-index-errors (REQ: contradictions-verb)

Scenario source: [../README.md](../README.md) → `### AC: contradictions-without-index-errors`.

Given a workspace directory where `studio index` has never run, when I run `specscore studio contradictions`, then the command exits 2 with a message naming the expected store path and suggesting `specscore studio index`.

No seams are needed: the missing-store guard fires before any store read.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:contradictions-without-index-errors
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"
mkdir repo-a
cat > studio.yaml <<'YAML'
name: demo
repos:
  - repo-a
YAML

set +e
stderr="$("$SPECSCORE" studio contradictions 2>&1 >/dev/null)"
exit_code=$?
set -e

[ "$exit_code" -eq 2 ] || { echo "FAIL: exit code $exit_code, want 2; stderr: $stderr"; exit 1; }

expected_logical="$(pwd)/.specscore-studio/facts.db"
expected_physical="$(pwd -P)/.specscore-studio/facts.db"
case "$stderr" in
  *"$expected_logical"* | *"$expected_physical"*) ;;
  *) echo "FAIL: message does not name expected store path $expected_logical: $stderr"; exit 1 ;;
esac

case "$stderr" in
  *"studio index"*) ;;
  *) echo "FAIL: message does not suggest studio index: $stderr"; exit 1 ;;
esac

echo "PASS: contradictions-without-index-errors"
```

---
*This document follows the https://specscore.md/scenario-specification*
