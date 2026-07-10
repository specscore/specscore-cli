---
format: https://specscore.md/scenario-specification
---

# Rehearse: context-bag-chains

**Status:** pending
**Verifies:** cli/rehearse/run#ac:context-bag-chains (REQ: context-bag)

Scenario source: [../README.md](../README.md) → `### AC: context-bag-chains`.

Given a scenario whose bash block writes `uid=42` to `$REHEARSE_CAPTURES`, followed by an sql block against a sqlite fixture querying `... WHERE id = {{uid}}` with `-- assert-rows: 1` and `-- capture: name = username`, followed by a bash block asserting `[ "{{name}}" = "alice" ]`, when I run `specscore rehearse run <that-file>`, then the command exits 0, the scenario is `pass`, and the JSON report's final bag contains `uid` and `name`.

```bash
#!/usr/bin/env bash
# Rehearse: cli/rehearse/run#ac:context-bag-chains
# Requires: specscore on PATH (override with $SPECSCORE), sqlite3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"

# Given a scenario whose bash block writes `uid=42` to `$REHEARSE_CAPTURES`
# (the same step creates the sqlite fixture inline via sqlite3 — steps share
# one scenario-scoped workdir, so the sql block's relative DSN finds it),
# followed by an sql block querying by the uid placeholder and capturing
# `name`, followed by a bash block asserting the name placeholder is alice
# (the inner fence is assembled via $fence so this file stays parseable, and
# the double-brace placeholders via $lb/$rb so this wrapper's own bash step
# does not try to interpolate them from its bag).
fence='```'
lb='{{'
rb='}}'
cat > chain.md <<MD
# Rehearse: context-bag chain fixture

**Status:** pending
**Verifies:** demo/fixture#ac:bag-chain

${fence}bash
sqlite3 fixture.db "CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT);
INSERT INTO users (id, username) VALUES (42, 'alice');"
echo "uid=42" >> "\$REHEARSE_CAPTURES"
${fence}

${fence}sql dsn=sqlite:fixture.db
SELECT username FROM users WHERE id = ${lb}uid${rb};
-- assert-rows: 1
-- capture: name = username
${fence}

${fence}bash
[ "${lb}name${rb}" = "alice" ]
${fence}
MD

# When I run `specscore rehearse run chain.md`
set +e
out="$("$SPECSCORE" rehearse run chain.md --format json 2>&1)"
exit_code=$?
set -e

# Then the command exits 0
[ "$exit_code" -eq 0 ] || { echo "FAIL: exit code $exit_code, want 0; output: $out"; exit 1; }

# And the scenario is `pass`
printf '%s\n' "$out" | grep -q '"status": "pass"' \
  || { echo "FAIL: JSON report does not mark the scenario pass: $out"; exit 1; }

# And the JSON report's final bag contains `uid` and `name`
printf '%s\n' "$out" | grep -q '"uid": "42"' \
  || { echo "FAIL: final bag lacks uid=42: $out"; exit 1; }
printf '%s\n' "$out" | grep -q '"name": "alice"' \
  || { echo "FAIL: final bag lacks name=alice: $out"; exit 1; }

echo "PASS: context-bag-chains"
```

---
*This document follows the https://specscore.md/scenario-specification*
