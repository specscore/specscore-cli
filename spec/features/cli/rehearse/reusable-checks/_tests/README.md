---
format: https://specscore.md/scenarios-index-specification
---

# Rehearse Scenarios — rehearse reusable checks

Acceptance scenarios for [`cli/rehearse/reusable-checks`](../README.md). The keystone reuse property is proven end-to-end across a **three-method login demo**: email (form POST), Google (OAuth callback), and Facebook (OAuth callback). All three flows produce their own fake HTTP response in their own way, then invoke the **same** two shared checks via `**Use:**` — `_checks/session-hardened.check.md` and `_checks/redirected-to-dashboard.check.md` — with byte-identical directive lines. No copy-paste: each check is authored once and reused by every flow. Parsing and evaluation are also covered exhaustively by the `scenario` and `runner` Go unit tests.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/scenarios-index-specification*
