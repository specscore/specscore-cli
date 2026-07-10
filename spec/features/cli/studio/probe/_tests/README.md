---
format: https://specscore.md/scenarios-index-specification
---

# Rehearse Scenarios — studio probe

Fourteen executable scenarios covering all acceptance criteria of [`cli/studio/probe`](../README.md).

Run with:

```
specscore rehearse run spec/features/cli/studio/probe/_tests
```

All fourteen scenarios are **pass-capable** and executable. Rather than
cross-process Go-var stubbing (which is impossible), the seams are reached the
way the plan pins them: the domain kind is probed against a localhost fixture
HTTP server (domains are `127.0.0.1:<port>` targets seeded into `domains.json`),
and the ci kind's `git`/`gh` are PATH-shim fake executables. Time-dependent
scenarios (`reprobe-refreshes-verified-at`, `changed-object-new-observation`)
use real wall-clock progression (two probe runs separated by a `sleep`), and the
freshness scenarios (`stale-filter-selects-old-facts`, `age-column-rendered`)
seed dated facts directly into the rebuild-only store via `sqlite3`. The corpus
stays hermetic — the only network is loopback, never the public internet.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/scenarios-index-specification*
