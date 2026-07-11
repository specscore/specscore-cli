---
format: https://specscore.md/scenario-specification
---

# Rehearse: Dry Run Ignores Flags

**Status:** pending
**Verifies:** cli/rehearse/new-dry-run#ac:dry-run-ignores-flags

Scenario source: [../README.md](../README.md) → `### AC: dry-run-ignores-flags`.

Scenario: --dry-run ignores --force and --commit
Given a feature `demo` with an AC `sample-case`
When I run `specscore rehearse new demo#ac:sample-case --dry-run --force --commit`
Then the command exits 0, prints the scaffold to stdout, and neither writes the
file nor requires a git repository (the write/commit path is skipped entirely).

```bash
SPECSCORE="${SPECSCORE:-specscore}"
mkdir -p spec/features/demo
cat > spec/features/demo/README.md <<'MD'
# Feature: demo

### AC: sample-case

Given a seeded workspace

## Next
MD
# No git repo here: --dry-run must return before the --commit path runs.
"$SPECSCORE" rehearse new demo#ac:sample-case --dry-run --force --commit > out.txt
grep -q "# Rehearse: Sample Case" out.txt || { echo "scaffold not printed"; cat out.txt; exit 1; }
echo "dry-run ignored --force/--commit and printed the scaffold"
```

### Assert: file `spec/features/demo/_tests/sample-case.md` missing

---
*This document follows the https://specscore.md/scenario-specification*
