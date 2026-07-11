---
format: https://specscore.md/scenario-specification
---

# File Assertion: contains-fails-mismatch

**Status:** pending
**Verifies:** cli/rehearse/file-assertions#ac:contains-fails-mismatch (REQ: assertion-failure-output)

Scenario source: [../README.md](../README.md) → `### AC: contains-fails-mismatch`.

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
