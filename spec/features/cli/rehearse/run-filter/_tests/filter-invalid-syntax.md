---
format: https://specscore.md/scenario-specification
---

# Rehearse: Filter Invalid Syntax

**Status:** pending
**Verifies:** cli/rehearse/run-filter#ac:filter-invalid-syntax

Scenario source: [../README.md](../README.md) → `### AC: filter-invalid-syntax`.

Scenario: a malformed filter reference exits 2
Given a fixture scenario directory
When I run `specscore rehearse run scn --filter demo` (missing the `#ac:` part)
Then the command exits 2 with an error. The bash step asserts the exit code, so
the scenario passes when the command correctly rejects the malformed filter.

```bash
SPECSCORE="${SPECSCORE:-specscore}"
mkdir -p scn
printf '**Verifies:** demo#ac:one\n' > scn/a.md
set +e
"$SPECSCORE" rehearse run scn --filter demo > out.txt 2>&1
rc=$?
set -e
if [ "$rc" -ne 2 ]; then
  echo "expected exit 2, got $rc"; cat out.txt; exit 1
fi
echo "malformed filter correctly rejected with exit 2"
```

---
*This document follows the https://specscore.md/scenario-specification*
