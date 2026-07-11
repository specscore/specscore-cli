---
format: https://specscore.md/scenario-specification
---

# File Assertion: contains-fails-mismatch

**Status:** pending
**Expect:** fail
**Verifies:** cli/rehearse/file-assertions#ac:contains-fails-mismatch (REQ: assertion-failure-output)

Scenario source: [../README.md](../README.md) → `### AC: contains-fails-mismatch`.

This is a **negative** scenario: the assertion below is meant to fail, so it
declares `**Expect:** fail` — the runner inverts the outcome and reports `pass`
when the assertion correctly fails (Feature: cli/rehearse/expected-fail).

Given a file with content "goodbye", when the runner evaluates `### Assert: file `output.txt` contains` with "hello" in the code block, then the assertion fails, the scenario exits 1, and the output shows expected vs. actual.

```bash
echo "goodbye" > output.txt
```

### Assert: file `output.txt` contains

```
hello
```

---
*This document follows the https://specscore.md/scenario-specification*
