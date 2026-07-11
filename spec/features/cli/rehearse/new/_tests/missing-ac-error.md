---
format: https://specscore.md/scenario-specification
---

# Rehearse: Missing Ac Error

**Status:** pending
**Verifies:** cli/rehearse/new#ac:missing-ac-error

Scenario source: [../README.md](../README.md) → `### AC: missing-ac-error`.

Scenario: invalid AC reference fails cleanly
Given a feature `demo` that exists but the AC slug `nonexistent` does not
When I run `specscore rehearse new demo#ac:nonexistent`
Then the command exits 2 and prints an error naming the missing AC.

The bash step asserts the exit code and error text itself, so the scenario
*passes* when the command correctly rejects the bad reference.

```bash
SPECSCORE="${SPECSCORE:-specscore}"
mkdir -p spec/features/demo
cat > spec/features/demo/README.md <<'MD'
# Feature: demo

### AC: real-one

Given a thing

## Next
MD
set +e
"$SPECSCORE" rehearse new demo#ac:nonexistent >out.txt 2>&1
rc=$?
set -e
if [ "$rc" -ne 2 ]; then
  echo "expected exit 2, got $rc"; cat out.txt; exit 1
fi
grep -q nonexistent out.txt || { echo "error did not name the missing AC"; cat out.txt; exit 1; }
echo "correctly rejected missing AC (exit 2)"
```

---
*This document follows the https://specscore.md/scenario-specification*
