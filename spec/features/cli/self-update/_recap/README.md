# Recap Reports — cli/self-update

Per-run recap reports produced by `specstudio:recap`. Each report is named `<sha>.md` where `<sha>` is the abbreviated git SHA of `HEAD` at run time. Each row records which verify report the recap was compared against (`Verify revision`) so reviewers can trace the drift gate end-to-end.

## Contents

| Report | Run revision | Verify revision | Drift summary |
|---|---|---|---|
| [e766e7e.md](e766e7e.md) | e766e7e | 35ef702 | 14 no-drift, 0 spec-tighter, 1 code-tighter, 0 contradiction, 0 unmapped, 0 errored |

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/index-specification*
