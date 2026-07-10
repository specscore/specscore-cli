---
format: https://specscore.md/scenario-specification
---

# Rehearse: age-column-rendered

**Status:** pass-capable
**Verifies:** cli/studio/probe#ac:age-column-rendered (REQ: age-rendering)

Scenario source: [../README.md](../README.md) → `### AC: age-column-rendered`.

Given a store containing a verified-behavior fact whose `verified_at` is 3 hours ago, when I run `specscore studio facts --class verified-behavior`, then the table includes a `VERIFIED` column and the row shows a human age (e.g. `3h`) for that fact.

Seam note: the dated fact is seeded directly into an `studio index`-built store
via `sqlite3` (the store's schema is fixed and rebuild-only). The `verified_at`
is set to 3h1m ago so the age column rounds to `3h` regardless of the few-second
gap between seeding and rendering.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/probe#ac:age-column-rendered
# Requires: specscore on PATH (override with $SPECSCORE), python3, sqlite3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"

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

# A verified-behavior fact whose verified_at is ~3 hours ago.
threeh="$(python3 -c 'import datetime;print((datetime.datetime.now(datetime.timezone.utc)-datetime.timedelta(hours=3,minutes=1)).strftime("%Y-%m-%dT%H:%M:%SZ"))')"
sqlite3 "$db" "INSERT INTO facts VALUES('aged.app','serves-status','200','verified-behavior','http://aged.app/','probe-domain','dev','$threeh','$threeh','demo');"

# When I run facts --class verified-behavior (table output).
out="$("$SPECSCORE_ABS" studio facts --class verified-behavior)"

# Then the table has a VERIFIED column ...
case "$out" in
  *"VERIFIED"*) ;;
  *) echo "FAIL: table lacks a VERIFIED column: $out"; exit 1 ;;
esac

# ... and the aged.app row shows a 3h age.
row="$(printf '%s\n' "$out" | grep 'aged.app' || true)"
[ -n "$row" ] || { echo "FAIL: no row for aged.app: $out"; exit 1; }
case "$row" in
  *"3h"*) ;;
  *) echo "FAIL: aged.app row does not show a 3h age: $row"; exit 1 ;;
esac

echo "PASS: age-column-rendered"
```

---
*This document follows the https://specscore.md/scenario-specification*
