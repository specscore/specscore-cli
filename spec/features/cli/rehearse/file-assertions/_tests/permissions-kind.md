---
format: https://specscore.md/scenario-specification
---

# File Assertion: permissions-kind

**Status:** pending
**Verifies:** cli/rehearse/file-assertions#ac:permissions-kind (REQ: assertion-kinds-defined)

Scenario source: [../README.md](../README.md) → `### AC: permissions-kind`.

Given a file with permissions 0644, when the runner evaluates `### Assert: file `spec.md` permissions` with "0644" in the code block, then the assertion passes.

```bash
touch spec.md
chmod 644 spec.md
```

### Assert: file `spec.md` permissions

```
0644
```

---
*This document follows the https://specscore.md/scenario-specification*
