---
format: https://specscore.md/scenario-specification
---

# Rehearse: Commit Flag

**Status:** pending
**Verifies:** cli/rehearse/new#ac:commit-flag

Scenario source: [../README.md](../README.md) → `### AC: commit-flag`.

Scenario: --commit stages and commits the new scaffold
Given a git repository with a feature `demo` that defines an AC `my-case`
When I run `specscore rehearse new demo#ac:my-case --commit`
Then the scaffold is written, staged with `git add`, and committed with subject
`feat(rehearse): scaffold my-case scenario`; the scaffold file is tracked with
no pending changes.

```bash
SPECSCORE="${SPECSCORE:-specscore}"
git init -q
git config user.email ci@example.com
git config user.name CI
git commit -q --allow-empty -m init
mkdir -p spec/features/demo
cat > spec/features/demo/README.md <<'MD'
# Feature: demo

### AC: my-case

Given a thing

## Next
MD
"$SPECSCORE" rehearse new demo#ac:my-case --commit
git log -1 --pretty=%s | grep -q "scaffold my-case scenario" \
  || { echo "commit subject missing"; git log --oneline; exit 1; }
if [ -n "$(git status --porcelain spec/features/demo/_tests/my-case.md)" ]; then
  echo "scaffold not committed cleanly"; git status --porcelain; exit 1
fi
echo "scaffold committed and tracked"
```

---
*This document follows the https://specscore.md/scenario-specification*
