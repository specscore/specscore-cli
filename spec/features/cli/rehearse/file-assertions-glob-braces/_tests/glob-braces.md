---
format: https://specscore.md/scenario-specification
---

# Rehearse: Glob Brace Expansion

**Status:** pending
**Verifies:** cli/rehearse/file-assertions-glob-braces#ac:brace-matches-alternatives

Scenario source: [../README.md](../README.md) → `### AC: brace-matches-alternatives`.

Scenario: brace expands to multiple extensions
Given object and archive files `a.o` and `b.a` in the working directory
When the runner evaluates `### Assert: file *.{o,a} exists`
Then the assertion passes because doublestar expands `{o,a}` and matches both.

```bash
printf 'obj\n' > a.o
printf 'ar\n' > b.a
printf 'text\n' > c.txt
```

### Assert: file `*.{o,a}` exists

---
*This document follows the https://specscore.md/scenario-specification*
