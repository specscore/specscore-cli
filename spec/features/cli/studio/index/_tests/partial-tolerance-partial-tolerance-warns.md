---
format: https://specscore.md/scenario-specification
---

# Rehearse: partial-tolerance-warns

**Status:** pending
**Verifies:** cli/studio/index#ac:partial-tolerance-warns (REQ: partial-tolerance)

Scenario source: [../README.md](../README.md) → `### AC: partial-tolerance-warns`.

Given a workspace listing one healthy fixture repo and one path that does not exist, when I run `specscore studio index`, then the command exits 0, the summary lists a warning for the missing path, and facts from the healthy repo are queryable.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/index#ac:partial-tolerance-warns
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given a workspace listing one healthy fixture repo and one path that does
# not exist
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

# When I run `specscore studio index`
set +e
out="$("$SPECSCORE" studio index 2>&1)"
exit_code=$?
set -e

# Then the command exits 0
[ "$exit_code" -eq 0 ] || { echo "FAIL: exit code $exit_code, want 0; output: $out"; exit 1; }

# And the summary lists a warning for the missing path
# (accept the logical or physical cwd — /var vs /private/var on macOS)
warning_line="$(echo "$out" | grep "repo directory does not exist")" \
  || { echo "FAIL: summary has no missing-repo warning: $out"; exit 1; }
case "$warning_line" in
  *"$missing_logical"* | *"$missing_physical"*) ;;
  *) echo "FAIL: warning does not name the missing path $missing_logical: $warning_line"; exit 1 ;;
esac
echo "$out" | grep -q "Warnings: 1" \
  || { echo "FAIL: summary does not count the warning: $out"; exit 1; }

# And facts from the healthy repo are queryable
count="$("$SPECSCORE" studio facts --predicate has-status --count)"
[ "$count" -eq 1 ] || { echo "FAIL: healthy-repo fact count $count, want 1"; exit 1; }

echo "PASS: partial-tolerance-warns"
```

---
*This document follows the https://specscore.md/scenario-specification*
