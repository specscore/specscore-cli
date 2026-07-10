---
format: https://specscore.md/scenarios-index-specification
---

# Rehearse Scenarios — rehearse evidence

Ten executable scenarios covering all acceptance criteria of [`cli/rehearse/evidence`](../README.md).

Run with:

```
specscore rehearse run spec/features/cli/rehearse/evidence/_tests
```

The `self-hosting-gate` scenario accepts `$REPO_ROOT` (or `$SPECSCORE_CLI_REPO`) to locate the repo checkout; the CI job passes `REPO_ROOT: ${{ github.workspace }}`. Without it the scenario falls back to the parent process cwd (works when the runner is invoked from the repo root).

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/scenarios-index-specification*
