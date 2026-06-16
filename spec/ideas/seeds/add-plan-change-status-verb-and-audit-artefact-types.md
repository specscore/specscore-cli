---
captured_by: user
status: queued
---
# Add plan change-status verb (and audit artefact types lacking change-status: plans and sidekick seeds have none)

GitHub issue: https://github.com/specscore/specscore-cli/issues/69 (follow-up to #68).

Surfaced driving the specstudio plan flow: the Plan artifact Status (Draft -> Under Review -> Approved) had to be hand-edited because 'specscore plan' has no change-status verb, re-introducing the #68 mirror-drift risk.

change-status coverage audit (v0.10.1):
- idea: has 'idea change-status'
- feature: has 'feature change-status'
- plan: MISSING (only info/list/new) -> Status hand-edited
- proposal: relies on 'idea change-status' (proposals are Ideas)
- sidekick seed: MISSING (status: queued has no transition verb; promote covers only promotion)

Ask:
1. Add 'specscore plan change-status <slug> --to=<status>' mirroring idea/feature (validate transition matrix, run lint --fix to sync index + frontmatter mirror).
2. Decide a transition path for sidekick seeds (or document promote/drop is the only lifecycle).
3. Confirm proposals use 'idea change-status'.

Downstream: once shipped, ai-plugin-specscore plan + change-status skills and specstudio-skills/skills/plan must call the verb instead of hand-editing. Companion seed in ai-plugin-specscore.
