---
format: https://specscore.md/scenario-specification
---

# File Assertion: missing-kind

**Status:** pending
**Verifies:** cli/rehearse/file-assertions#ac:missing-kind (REQ: file-assert-block-syntax)

Scenario source: [../README.md](../README.md) → `### AC: missing-kind`.

Given a scenario where a file should not exist, when the runner evaluates `### Assert: file `nonexistent.txt` missing`, then the assertion passes and the scenario succeeds.

```bash
# Ensure no file is created
true
```

### Assert: file `nonexistent.txt` missing

The file should not exist after the scenario steps execute.

---
*This document follows the https://specscore.md/scenario-specification*
