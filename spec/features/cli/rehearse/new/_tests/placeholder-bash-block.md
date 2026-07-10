---
format: https://specscore.md/scenario-specification
---

# Rehearse: Placeholder Bash Block

**Status:** pending
**Verifies:** cli/rehearse/new#ac:placeholder-bash-block

Scenario source: [../README.md](../README.md) → `### AC: placeholder-bash-block`.

Scenario: bash block placeholder guides implementation
Given the command from AC: resolve-ac-reference
When I read the file's last 10 lines
Then there is a `\`\`\`bash` block with `# TODO: Implement steps...` and `echo "TODO: implement scenario steps"`

### Step: [TODO — implement the scenario steps]

A placeholder bash block with guidance:

```bash
# TODO: Implement steps to verify the scenario's Given/When/Then.
# Use $SPECSCORE to invoke the CLI under test (e.g., $SPECSCORE rehearse new ...).
# Assert exit codes and output as needed.
echo "TODO: implement scenario steps"
```
