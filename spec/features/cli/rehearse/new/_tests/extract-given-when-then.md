---
format: https://specscore.md/scenario-specification
---

# Rehearse: Extract Given When Then

**Status:** pending
**Verifies:** cli/rehearse/new#ac:extract-given-when-then

Scenario source: [../README.md](../README.md) → `### AC: extract-given-when-then`.

Scenario: AC text is extracted into the scaffold
Given the AC whose `### AC: index-two-repos` body has `Scenario: index a two-repo workspace` followed by `Given a studio.yaml...`, `When I run specscore studio index`, `Then the command exits 0...`
When I run the command above
Then the file contains the verbatim Given/When/Then text under a `Scenario source:` heading

### Step: [TODO — implement the scenario steps]

A placeholder bash block with guidance:

```bash
# TODO: Implement steps to verify the scenario's Given/When/Then.
# Use $SPECSCORE to invoke the CLI under test (e.g., $SPECSCORE rehearse new ...).
# Assert exit codes and output as needed.
echo "TODO: implement scenario steps"
```
