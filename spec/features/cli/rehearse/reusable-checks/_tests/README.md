---
format: https://specscore.md/scenarios-index-specification
---

# Rehearse Scenarios — rehearse reusable checks

Acceptance scenarios for [`cli/rehearse/reusable-checks`](../README.md). The keystone reuse property is proven end-to-end: `checks/session-hardened.check.md` is written once and reused (via `**Use:**`) by two different sign-in flows (email and OAuth) — no copy-paste. Parsing and evaluation are also covered exhaustively by the `scenario` and `runner` Go unit tests.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/scenarios-index-specification*
