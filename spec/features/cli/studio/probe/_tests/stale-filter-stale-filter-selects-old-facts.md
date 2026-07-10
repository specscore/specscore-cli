---
format: https://specscore.md/scenario-specification
---

# Rehearse: stale-filter-selects-old-facts

**Status:** pass-capable
**Verifies:** cli/studio/probe#ac:stale-filter-selects-old-facts (REQ: stale-filter)

Scenario source: [../README.md](../README.md) → `### AC: stale-filter-selects-old-facts`.

Given a store containing one verified-behavior fact with `verified_at` 48 hours ago and one with `verified_at` 1 hour ago, when I run `specscore studio facts --class verified-behavior --stale 24h --count`, then the count is 1 (only the 48-hour-old fact).

Seam note: the store is a rebuild-only disposable SQLite cache with a fixed
schema (`REQ: stale-filter`), so the two dated facts are seeded directly into an
`studio index`-built store via `sqlite3` (on PATH in CI). The `verified_at`
timestamps are computed relative to now with `python3`, so the 24h cutoff is
exercised against real wall-clock ages — no clock injection required.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:stale-filter-selects-old-facts
# Requires: specscore on PATH (override with $SPECSCORE), python3, sqlite3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"

# An indexed store gives us the exact facts-table schema to seed into.
mkdir -p repo-a/spec/features/x
cat > repo-a/specscore.yaml <<'YAML'
project:
  title: A
YAML
cat > repo-a/spec/features/x/README.md <<'MD'
# Feature: X

**Status:** Approved
MD
cat > studio.yaml <<'YAML'
name: demo
repos:
  - repo-a
YAML
"$SPECSCORE_ABS" studio index >/dev/null
db=".specscore-studio/facts.db"

# Two verified-behavior facts: one 48h old, one 1h old.
old="$(python3 -c 'import datetime;print((datetime.datetime.now(datetime.timezone.utc)-datetime.timedelta(hours=48)).strftime("%Y-%m-%dT%H:%M:%SZ"))')"
fresh="$(python3 -c 'import datetime;print((datetime.datetime.now(datetime.timezone.utc)-datetime.timedelta(hours=1)).strftime("%Y-%m-%dT%H:%M:%SZ"))')"
sqlite3 "$db" "INSERT INTO facts VALUES('old.app','serves-status','200','verified-behavior','http://old.app/','probe-domain','dev','$old','$old','demo');"
sqlite3 "$db" "INSERT INTO facts VALUES('fresh.app','serves-status','200','verified-behavior','http://fresh.app/','probe-domain','dev','$fresh','$fresh','demo');"

# When I run facts --class verified-behavior --stale 24h --count.
count="$("$SPECSCORE_ABS" studio facts --class verified-behavior --stale 24h --count)"

# Then the count is 1 (only the 48-hour-old fact).
[ "$count" -eq 1 ] || { echo "FAIL: --stale 24h count = $count, want 1"; exit 1; }

echo "PASS: stale-filter-selects-old-facts"
```

---
*This document follows the https://specscore.md/scenario-specification*
