---
format: https://specscore.md/scenario-specification
---

# Rehearse: Filter Matching Exact

**Status:** pending
**Verifies:** cli/rehearse/run-filter#ac:filter-matching-exact

Scenario source: [../README.md](../README.md) → `### AC: filter-matching-exact`.

Scenario: only the exact AC reference matches, and rows are labelled
Given two fixture scenarios verifying `demo#ac:one` and `demo#ac:two`
When I run `specscore rehearse run scn --filter demo#ac:one`
Then the first is labelled `[filter-match]` and the second `[filter-skip]`.

```bash
SPECSCORE="${SPECSCORE:-specscore}"
mkdir -p scn
printf '**Verifies:** demo#ac:one\n' > scn/a.md
printf '**Verifies:** demo#ac:two\n' > scn/b.md
"$SPECSCORE" rehearse run scn --filter demo#ac:one > out.txt 2>&1
grep "a.md" out.txt | grep -q "filter-match" || { echo "a.md not matched"; cat out.txt; exit 1; }
grep "b.md" out.txt | grep -q "filter-skip"  || { echo "b.md not skipped"; cat out.txt; exit 1; }
echo "exact filter matched a.md and skipped b.md"
```

---
*This document follows the https://specscore.md/scenario-specification*
