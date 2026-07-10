---
format: https://specscore.md/scenario-specification
---

# Rehearse: Frontmatter Verifies Metadata

**Status:** pending
**Verifies:** cli/rehearse/new#ac:frontmatter-verifies-metadata

Scenario source: [../README.md](../README.md) → `### AC: frontmatter-verifies-metadata`.

Scenario: frontmatter and Verifies line are correct
Given the command from AC: resolve-ac-reference
When I check the file's first 10 lines
Then `**Status:** pending` and `**Verifies:** cli/studio/index#ac:index-two-repos` are present

### Step: [TODO — implement the scenario steps]

A placeholder bash block with guidance:

```bash
# TODO: Implement steps to verify the scenario's Given/When/Then.
# Use $SPECSCORE to invoke the CLI under test (e.g., $SPECSCORE rehearse new ...).
# Assert exit codes and output as needed.
echo "TODO: implement scenario steps"
```
