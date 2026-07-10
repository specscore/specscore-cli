---
format: https://specscore.md/scenario-specification
---

# Rehearse: rebuild-drops-removed-repo

**Status:** pending
**Verifies:** cli/studio/index#ac:rebuild-drops-removed-repo (REQ: rebuild-only)

Scenario source: [../README.md](../README.md) → `### AC: rebuild-drops-removed-repo`.

Given a fact store previously indexed from a workspace of two repos, when I remove one repo from `studio.yaml` and run `specscore studio index` again, then `specscore studio facts --subject <removed-repo>*` reports zero facts.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/index#ac:rebuild-drops-removed-repo
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"

# Given a fact store previously indexed from a workspace of two repos
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"
mkdir -p repo-a repo-b
cat > repo-a/go.mod <<'GOMOD'
module example.com/a

go 1.22

require example.com/dep v1.0.0
GOMOD
cat > repo-b/go.mod <<'GOMOD'
module example.com/b

go 1.22

require example.com/m v1.2.3
GOMOD
cat > studio.yaml <<'YAML'
name: demo
repos:
  - repo-a
  - repo-b
YAML
"$SPECSCORE" studio index >/dev/null

# Sanity: the store holds facts for repo-b before it is removed.
before="$("$SPECSCORE" studio facts --subject 'repo-b*' --count)"
if [ "$before" -eq 0 ]; then
  echo "FAIL: expected repo-b facts before removal, got $before"
  exit 1
fi

# When I remove one repo from studio.yaml and run `specscore studio index` again
cat > studio.yaml <<'YAML'
name: demo
repos:
  - repo-a
YAML
"$SPECSCORE" studio index >/dev/null

# Then `specscore studio facts --subject <removed-repo>*` reports zero facts
after="$("$SPECSCORE" studio facts --subject 'repo-b*' --count)"
if [ "$after" -ne 0 ]; then
  echo "FAIL: expected zero repo-b facts after re-index, got $after"
  "$SPECSCORE" studio facts --subject 'repo-b*'
  exit 1
fi

echo "PASS: rebuild-drops-removed-repo"
```

---
*This document follows the https://specscore.md/scenario-specification*
