---
format: https://specscore.md/scenario-specification
---

# Glob Patterns in File Assertions

**Status:** pending
**Verifies:** cli/rehearse/file-assertions-glob#ac:glob-multiple-match

Scenario source: [../README.md](../README.md) → `### AC: glob-multiple-match`.

Given several `.json` files that each contain the substring "config", when the
runner evaluates `### Assert: file *.json contains` with "config", then the
assertion passes because every matched file satisfies the condition (set-based
AND semantics). A parallel `*.json exists` heading confirms the glob resolves
to at least one file.

```bash
printf 'config alpha\n' > alpha.json
printf 'config beta\n' > beta.json
printf 'config gamma\n' > gamma.json
```

### Assert: file `*.json` exists

### Assert: file `*.json` contains

```
config
```

---
*This document follows the https://specscore.md/scenario-specification*
