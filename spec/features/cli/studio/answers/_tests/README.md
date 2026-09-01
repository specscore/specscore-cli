---
format: https://specscore.md/scenarios-index-specification
---

# Rehearse Scenarios — studio answers

Seventeen executable scenarios covering all acceptance criteria of [`cli/studio/answers`](../README.md).

Run with:

```
specscore rehearse run spec/features/cli/studio/answers/_tests
```

All scenarios are **pass-capable** — the feature is Implementing. Every scenario
is hermetic: detector, resolve, and ask scenarios run over a fixture store built
offline (`studio index` over tiny fixture repos, plus `sqlite3`-seeded
`verified-behavior` facts where a probed store is Given — the house pattern from
`cli/studio/probe/_tests`); the benchmark scenarios assert over the committed
`benchmark/questions.jsonl` and the committed `benchmark/testdata/fixture/` workspace.
No scenario touches the public network or requires a Sneat checkout — the
Sneat 40/50 run is the human-runnable exit gate documented in the feature's
`## Exit gate`, not a corpus scenario.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/scenarios-index-specification*
