---
format: https://specscore.md/scenario-specification
---

# Rehearse: Resolve Ac Reference

**Status:** pending
**Verifies:** cli/rehearse/new#ac:resolve-ac-reference

Scenario source: [../README.md](../README.md) → `### AC: resolve-ac-reference`.

Scenario: valid AC reference resolves
Given a feature `cli/studio/index` with an AC `index-two-repos`
When I run `specscore rehearse new cli/studio/index#ac:index-two-repos`
Then the command exits 0 and creates a file at `spec/features/cli/studio/index/_tests/index-two-repos.md`

### Step: [TODO — implement the scenario steps]

A placeholder bash block with guidance:

```bash
# TODO: Implement steps to verify the scenario's Given/When/Then.
# Use $SPECSCORE to invoke the CLI under test (e.g., $SPECSCORE rehearse new ...).
# Assert exit codes and output as needed.
echo "TODO: implement scenario steps"
```
