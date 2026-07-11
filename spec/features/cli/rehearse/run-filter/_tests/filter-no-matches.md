---
format: https://specscore.md/scenario-specification
---

# Rehearse: Filter No Matches

**Status:** pending
**Verifies:** cli/rehearse/run-filter#ac:filter-no-matches

Scenario source: [../README.md](../README.md) → `### AC: filter-no-matches`.

Scenario: a filter that matches nothing exits 0 with a notice
Given a fixture scenario verifying `demo#ac:one`
When I run `specscore rehearse run scn --filter cli/nonexistent#ac:nope`
Then the command exits 0 and prints "No scenarios matched filter(s): ...".

```bash
SPECSCORE="${SPECSCORE:-specscore}"
mkdir -p scn
printf '**Verifies:** demo#ac:one\n' > scn/a.md
"$SPECSCORE" rehearse run scn --filter cli/nonexistent#ac:nope > out.txt 2>&1
grep -q "No scenarios matched filter" out.txt || { echo "missing no-match notice"; cat out.txt; exit 1; }
echo "no-match filter exited 0 with a notice"
```

---
*This document follows the https://specscore.md/scenario-specification*
