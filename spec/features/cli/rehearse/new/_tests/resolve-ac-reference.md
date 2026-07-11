---
format: https://specscore.md/scenario-specification
---

# Rehearse: Resolve Ac Reference

**Status:** pending
**Verifies:** cli/rehearse/new#ac:resolve-ac-reference

Scenario source: [../README.md](../README.md) → `### AC: resolve-ac-reference`.

Scenario: valid AC reference resolves
Given a feature `demo` with an AC `sample-case`
When I run `specscore rehearse new demo#ac:sample-case`
Then the command exits 0 and creates a file at `spec/features/demo/_tests/sample-case.md`.

```bash
SPECSCORE="${SPECSCORE:-specscore}"
mkdir -p spec/features/demo
cat > spec/features/demo/README.md <<'MD'
# Feature: demo

### AC: sample-case

Scenario: sample case
Given a seeded workspace
When I run the tool
Then it exits 0

## Next
MD
"$SPECSCORE" rehearse new demo#ac:sample-case
```

### Assert: file `spec/features/demo/_tests/sample-case.md` exists

---
*This document follows the https://specscore.md/scenario-specification*
