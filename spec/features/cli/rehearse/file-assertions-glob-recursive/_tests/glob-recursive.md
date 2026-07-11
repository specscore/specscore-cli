---
format: https://specscore.md/scenario-specification
---

# Rehearse: Glob Recursive Matches All Depths

**Status:** pending
**Verifies:** cli/rehearse/file-assertions-glob-recursive#ac:glob-recursive-matches-all-depths

Scenario source: [../README.md](../README.md) → `### AC: glob-recursive-matches-all-depths`.

Scenario: `**` matches files at every depth
Given `.o` files at `build/x86/obj.o`, `build/arm/obj.o`, and `build/arm/deep/obj.o`
When the runner evaluates `### Assert: file build/**/*.o exists`
Then the assertion passes because doublestar matches all three across depths 1 and 2.

```bash
mkdir -p build/x86 build/arm/deep
printf 'obj\n' > build/x86/obj.o
printf 'obj\n' > build/arm/obj.o
printf 'obj\n' > build/arm/deep/obj.o
```

### Assert: file `build/**/*.o` exists

---
*This document follows the https://specscore.md/scenario-specification*
