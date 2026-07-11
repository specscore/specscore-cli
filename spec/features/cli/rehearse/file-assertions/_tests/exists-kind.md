---
format: https://specscore.md/scenario-specification
---

# File Assertion: exists-kind

**Status:** pending
**Verifies:** cli/rehearse/file-assertions#ac:exists-kind (REQ: file-assert-block-syntax)

Scenario source: [../README.md](../README.md) → `### AC: exists-kind`.

Given a scenario that creates a file at `.specscore/test.txt`, when the runner evaluates `### Assert: file `.specscore/test.txt` exists`, then the assertion passes and the scenario succeeds.

```bash
mkdir -p .specscore
touch .specscore/test.txt
```

### Assert: file `.specscore/test.txt` exists

The file should exist after the scenario steps execute.

---
*This document follows the https://specscore.md/scenario-specification*
