---
format: https://specscore.md/scenario-specification
---

# Rehearse: strict-mode-fails

**Status:** pending
**Verifies:** cli/studio/index#ac:strict-mode-fails (REQ: partial-tolerance)

Scenario source: [../README.md](../README.md) → `### AC: strict-mode-fails`.

Given the same workspace with one missing repo path, when I run `specscore studio index --strict`, then the command exits 3 and the warning is printed.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/index#ac:strict-mode-fails
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given the same workspace with one missing repo path
# (one healthy fixture repo + one path that does not exist)
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"
mkdir -p repo-a/spec/features/x
cat > repo-a/specscore.yaml <<'YAML'
project:
  title: Fixture Repo
YAML
cat > repo-a/spec/features/x/README.md <<'MD'
# Feature: X

**Status:** Approved
MD
cat > studio.yaml <<'YAML'
name: demo
repos:
  - repo-a
  - no-such-repo
YAML
missing_logical="$(pwd)/no-such-repo"
missing_physical="$(pwd -P)/no-such-repo"

# When I run `specscore studio index --strict`
set +e
out="$("$SPECSCORE" studio index --strict 2>&1)"
exit_code=$?
set -e

# Then the command exits 3
[ "$exit_code" -eq 3 ] || { echo "FAIL: exit code $exit_code, want 3; output: $out"; exit 1; }

# And the warning is printed
# (accept the logical or physical cwd — /var vs /private/var on macOS)
warning_line="$(echo "$out" | grep "repo directory does not exist")" \
  || { echo "FAIL: missing-repo warning not printed: $out"; exit 1; }
case "$warning_line" in
  *"$missing_logical"* | *"$missing_physical"*) ;;
  *) echo "FAIL: warning does not name the missing path $missing_logical: $warning_line"; exit 1 ;;
esac

echo "PASS: strict-mode-fails"
```

---
*This document follows the https://specscore.md/scenario-specification*
