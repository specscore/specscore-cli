---
format: https://specscore.md/scenarios-index-specification
---

# Rehearse Scenarios — rehearse nested scenario suites

Acceptance scenarios for [`cli/rehearse/nested-suites`](../README.md). `sign-in.md` is a nested suite: one shared `## Given` (a login endpoint stub) with two `### When` branches (correct vs wrong password), each verified independently — success and failure behaviour in one readable file, no duplicated setup. The runner executes each branch as its own case. Splitting and per-branch execution are also covered exhaustively by the `scenario` and `runner` Go unit tests.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/scenarios-index-specification*
