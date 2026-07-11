---
format: https://specscore.md/scenario-specification
---

# File Assertion: runner-parses-file-blocks

**Status:** pending
**Verifies:** cli/rehearse/file-assertions#ac:runner-parses-file-blocks (REQ: runner-parses-file-blocks)

Scenario source: [../README.md](../README.md) → `### AC: runner-parses-file-blocks`.

Given a scenario with multiple `### Assert: file` blocks, when the runner parses the scenario, then all file blocks are recognized and queued for evaluation.

```bash
echo "test" > file1.txt
echo "data" > file2.txt
```

### Assert: file `file1.txt` exists

The first file should exist.

### Assert: file `file2.txt` exists

The second file should exist.

### Assert: file `file1.txt` contains

```
test
```

---
*This document follows the https://specscore.md/scenario-specification*
