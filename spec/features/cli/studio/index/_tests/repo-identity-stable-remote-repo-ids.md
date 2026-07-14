---
format: https://specscore.md/scenario-specification
---

# Rehearse: stable-remote-repo-ids

**Status:** pending
**Verifies:** cli/studio/index#ac:stable-remote-repo-ids (REQ: repo-identity)

Scenario source: [../README.md](../README.md) → `### AC: stable-remote-repo-ids`.

Given two workspace repositories both checked out into directories named `backstage`, with origins `https://github.com/sneat-co/backstage.git` and `git@github.com:dal-go/backstage.git`, when I index them in either workspace order, then their facts use repository IDs `github.com/sneat-co/backstage` and `github.com/dal-go/backstage` in both runs, with no basename collision suffix.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/index#ac:stable-remote-repo-ids
# Requires: specscore and git on PATH (override specscore with $SPECSCORE), python3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"

# Given two same-basename checkouts with distinct forge remotes and one
# manifest fact each.
mkdir -p sneat-co/backstage dal-go/backstage
for repo in sneat-co/backstage dal-go/backstage; do
  git -C "$repo" init -q
done
git -C sneat-co/backstage remote add origin https://github.com/sneat-co/backstage.git
git -C dal-go/backstage remote add origin git@github.com:dal-go/backstage.git
cat > sneat-co/backstage/go.mod <<'GOMOD'
module example.com/sneat-backstage

go 1.22
GOMOD
cat > dal-go/backstage/go.mod <<'GOMOD'
module example.com/dalgo-backstage

go 1.22
GOMOD

assert_remote_ids() {
  json="$("$SPECSCORE" studio facts --format json)"
  FACTS_JSON="$json" python3 - <<'PY'
import json
import os

facts = json.loads(os.environ["FACTS_JSON"])
repository_ids = {fact["subject"].split("#", 1)[0] for fact in facts}
expected = {
    "github.com/sneat-co/backstage",
    "github.com/dal-go/backstage",
}
assert repository_ids == expected, f"FAIL: repository IDs {repository_ids}, want {expected}"
PY
}

# When I index them in the first order, their facts use forge-coordinate IDs.
cat > studio.yaml <<'YAML'
name: demo
repos:
  - sneat-co/backstage
  - dal-go/backstage
YAML
"$SPECSCORE" studio index >/dev/null
assert_remote_ids

# And the same IDs are preserved when the workspace order is reversed.
cat > studio.yaml <<'YAML'
name: demo
repos:
  - dal-go/backstage
  - sneat-co/backstage
YAML
"$SPECSCORE" studio index >/dev/null
assert_remote_ids

echo "PASS: stable-remote-repo-ids"
```

---
*This document follows the https://specscore.md/scenario-specification*
