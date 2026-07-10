---
format: https://specscore.md/scenario-specification
---

# Rehearse: sql-assert-rows

**Status:** pending
**Verifies:** cli/rehearse/run#ac:sql-assert-rows (REQ: sql-block)

Scenario source: [../README.md](../README.md) → `### AC: sql-assert-rows`.

Given a sqlite fixture database with 3 rows in table `t` and a scenario whose sql block queries `t` with `-- assert-rows: 3`, when I run `specscore rehearse run <that-file>`, then the command exits 0 and the scenario is `pass`.

```bash
#!/usr/bin/env bash
# Rehearse: cli/rehearse/run#ac:sql-assert-rows
# Requires: specscore on PATH (override with $SPECSCORE), sqlite3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given a sqlite fixture database with 3 rows in table `t`
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"
sqlite3 fixture.db "CREATE TABLE t (id INTEGER, username TEXT);
INSERT INTO t VALUES (1, 'alice'), (2, 'bob'), (3, 'carol');"

# And a scenario whose sql block queries `t` with `-- assert-rows: 3`
# (the sql block runs in a scenario-scoped temp dir, so the DSN carries the
# fixture's absolute path; the inner fence is assembled via $fence so this
# file stays parseable).
fence='```'
cat > scenario.md <<MD
# Rehearse: sql fixture

**Status:** pending
**Verifies:** demo/fixture#ac:sql-rows

${fence}sql dsn=sqlite:${workdir}/fixture.db
SELECT * FROM t;
-- assert-rows: 3
${fence}
MD

# When I run `specscore rehearse run scenario.md`
set +e
out="$("$SPECSCORE" rehearse run scenario.md 2>&1)"
exit_code=$?
set -e

# Then the command exits 0
[ "$exit_code" -eq 0 ] || { echo "FAIL: exit code $exit_code, want 0; output: $out"; exit 1; }

# And the scenario is `pass`
printf '%s\n' "$out" | grep -qE '^pass[[:space:]]+.*scenario\.md' \
  || { echo "FAIL: report does not mark the scenario pass: $out"; exit 1; }
printf '%s\n' "$out" | grep -q '1 pass' \
  || { echo "FAIL: totals line does not count the pass: $out"; exit 1; }

echo "PASS: sql-assert-rows"
```

---
*This document follows the https://specscore.md/scenario-specification*
