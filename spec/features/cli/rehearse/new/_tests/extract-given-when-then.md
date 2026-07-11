---
format: https://specscore.md/scenario-specification
---

# Rehearse: Extract Given When Then

**Status:** pending
**Verifies:** cli/rehearse/new#ac:extract-given-when-then

Scenario source: [../README.md](../README.md) → `### AC: extract-given-when-then`.

Scenario: AC text is extracted into the scaffold
Given an AC whose body has `Given`/`When`/`Then` lines
When I run `specscore rehearse new demo#ac:sample-case`
Then the scaffold contains the verbatim Given/When/Then text.

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

### Assert: file `spec/features/demo/_tests/sample-case.md` contains

```
Given a seeded workspace
```

### Assert: file `spec/features/demo/_tests/sample-case.md` contains

```
Then it exits 0
```

---
*This document follows the https://specscore.md/scenario-specification*
