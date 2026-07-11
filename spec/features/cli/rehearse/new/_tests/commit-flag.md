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
`feat(rehearse): scaffold my-case scenario` and a `Verifies:` trailer; the
scaffold file is tracked with no pending changes.

### Step: [TODO — implement the scenario steps]

A placeholder bash block with guidance:

```bash
# TODO: Implement steps to verify the scenario's Given/When/Then.
# Set up a throwaway git repo + a demo feature README with `### AC: my-case`,
# run `$SPECSCORE rehearse new demo#ac:my-case --commit`, then assert:
#   git log -1 --pretty=%s  contains "scaffold my-case scenario"
#   git status --porcelain spec/features/demo/_tests/my-case.md  is empty
echo "TODO: implement scenario steps"
```
