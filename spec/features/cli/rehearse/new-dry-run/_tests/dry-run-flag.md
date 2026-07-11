---
format: https://specscore.md/scenario-specification
---

# Rehearse: Dry Run Flag

**Status:** pending
**Verifies:** cli/rehearse/new-dry-run#ac:dry-run-flag

Scenario source: [../README.md](../README.md) → `### AC: dry-run-flag`.

Scenario: --dry-run prints scaffold to stdout
Given a feature `demo` with an AC `sample-case`
When I run `specscore rehearse new demo#ac:sample-case --dry-run`
Then the command exits 0, prints the scaffold markdown to stdout, and writes no file.

```bash
SPECSCORE="${SPECSCORE:-specscore}"
mkdir -p spec/features/demo
cat > spec/features/demo/README.md <<'MD'
# Feature: demo

### AC: sample-case

Given a seeded workspace

## Next
MD
"$SPECSCORE" rehearse new demo#ac:sample-case --dry-run > out.txt
grep -q "# Rehearse: Sample Case" out.txt || { echo "scaffold not printed to stdout"; cat out.txt; exit 1; }
grep -q "demo#ac:sample-case" out.txt || { echo "Verifies reference missing from stdout"; cat out.txt; exit 1; }
echo "scaffold printed to stdout"
```

### Assert: file `spec/features/demo/_tests/sample-case.md` missing

---
*This document follows the https://specscore.md/scenario-specification*
