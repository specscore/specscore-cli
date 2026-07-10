---
format: https://specscore.md/scenario-specification
---

# Rehearse: Missing Ac Error

**Status:** pending
**Verifies:** cli/rehearse/new#ac:missing-ac-error

Scenario source: [../README.md](../README.md) → `### AC: missing-ac-error`.

Scenario: invalid AC reference fails cleanly
Given a feature that exists but the AC slug `nonexistent` does not
When I run `specscore rehearse new cli/studio/index#ac:nonexistent`
Then the command exits 2 and prints an error naming the missing AC

### Step: [TODO — implement the scenario steps]

A placeholder bash block with guidance:

```bash
# TODO: Implement steps to verify the scenario's Given/When/Then.
# Use $SPECSCORE to invoke the CLI under test (e.g., $SPECSCORE rehearse new ...).
# Assert exit codes and output as needed.
echo "TODO: implement scenario steps"
```
