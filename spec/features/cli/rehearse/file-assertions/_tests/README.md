---
format: https://specscore.md/scenarios-index-specification
---

# Rehearse Scenarios — rehearse file assertions

One scenario per acceptance criterion of [`cli/rehearse/file-assertions`](../README.md), covering the `exists`, `missing`, `contains`, `not-contains`, and `permissions` kinds plus the failure-output path. Assertion evaluation is also proven by the `internal/rehearse/blocks/fileblock` and `internal/rehearse/runner` Go unit tests. The `contains-fails` scenario is a deliberate negative case (it asserts the runner fails), so this directory is verified via unit tests rather than the CI corpus runner, which has no expected-fail concept.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/scenarios-index-specification*
