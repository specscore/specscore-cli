---
format: https://specscore.md/scenario-specification
---

# Rehearse: Frontmatter Verifies Metadata

**Status:** pending
**Verifies:** cli/rehearse/new#ac:frontmatter-verifies-metadata

Scenario source: [../README.md](../README.md) → `### AC: frontmatter-verifies-metadata`.

Scenario: frontmatter and Verifies line are correct
Given the command from AC: resolve-ac-reference
When I check the scaffold's metadata
Then `**Status:** pending`, the `**Verifies:** demo#ac:sample-case` line, and the
scenario-specification frontmatter are present.

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
format: https://specscore.md/scenario-specification
```

### Assert: file `spec/features/demo/_tests/sample-case.md` contains

```
**Status:** pending
```

### Assert: file `spec/features/demo/_tests/sample-case.md` contains

```
**Verifies:** demo#ac:sample-case
```

---
*This document follows the https://specscore.md/scenario-specification*
