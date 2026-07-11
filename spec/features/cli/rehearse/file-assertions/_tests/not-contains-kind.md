---
format: https://specscore.md/scenario-specification
---

# File Assertion: not-contains-kind

**Status:** pending
**Verifies:** cli/rehearse/file-assertions#ac:not-contains-kind (REQ: assertion-kinds-defined)

Scenario source: [../README.md](../README.md) → `### AC: not-contains-kind`.

Given a file with content "goodbye", when the runner evaluates `### Assert: file `output.txt` not-contains` with "hello" in the code block, then the assertion passes and the scenario succeeds.

```bash
echo "goodbye" > output.txt
```

### Assert: file `output.txt` not-contains

```
hello
```

---
*This document follows the https://specscore.md/scenario-specification*
