---
format: https://specscore.md/scenario-specification
---

# Rehearse: Error Handling Same

**Status:** pending
**Verifies:** cli/rehearse/new-dry-run#ac:error-handling-same

Scenario source: [../README.md](../README.md) → `### AC: error-handling-same`.

Scenario: error paths unchanged under --dry-run
Given a feature that does not exist
When I run `specscore rehearse new missing-feature#ac:nonexistent --dry-run`
Then the command exits 2 with an error, exactly as it would without --dry-run.
The bash step asserts the exit code itself, so the scenario passes when the
command correctly errors.

```bash
SPECSCORE="${SPECSCORE:-specscore}"
set +e
"$SPECSCORE" rehearse new missing-feature#ac:nonexistent --dry-run > out.txt 2>&1
rc=$?
set -e
if [ "$rc" -ne 2 ]; then
  echo "expected exit 2, got $rc"; cat out.txt; exit 1
fi
echo "dry-run preserved the exit-2 error path"
```

---
*This document follows the https://specscore.md/scenario-specification*
