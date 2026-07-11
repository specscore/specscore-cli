---
format: https://specscore.md/scenario-specification
---

# Rehearse: Placeholder Bash Block

**Status:** pending
**Verifies:** cli/rehearse/new#ac:placeholder-bash-block

Scenario source: [../README.md](../README.md) → `### AC: placeholder-bash-block`.

Scenario: bash block placeholder guides implementation
Given the command from AC: resolve-ac-reference
When I inspect the scaffold's step section
Then it contains a bash block with the `# TODO: Implement steps` guidance and the
`echo "TODO: implement scenario steps"` placeholder line.

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
# TODO: Implement steps
```

### Assert: file `spec/features/demo/_tests/sample-case.md` contains

```
echo "TODO: implement scenario steps"
```

---
*This document follows the https://specscore.md/scenario-specification*
