---
format: https://specscore.md/scenario-specification
---

# File Assertion: contains-kind

**Status:** pending
**Verifies:** cli/rehearse/file-assertions#ac:contains-kind (REQ: assertion-kinds-defined)

Scenario source: [../README.md](../README.md) → `### AC: contains-kind`.

Given a scenario that writes "hello world" to a file, when the runner evaluates `### Assert: file `output.txt` contains` with "hello world" in the code block, then the assertion passes.

```bash
echo "hello world" > output.txt
```

### Assert: file `output.txt` contains

```
hello world
```

---
*This document follows the https://specscore.md/scenario-specification*
