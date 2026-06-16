---
captured_by: user
status: queued
---
# Feature template metadata is prettier-unstable: no blank line after the Studio blockquote lets prettier merge Status/Source Ideas into it, breaking lint

GitHub issue: https://github.com/specscore/specscore-cli/issues/71

Reproduced while running the specstudio flow in a repo whose pre-commit hook runs 'prettier --write' on markdown.

'specscore feature new' generates the Feature README with the metadata block immediately under the SpecScore.Studio blockquote, with NO blank line between them:

    > [SpecScore.Studio](...): | [Explore](...) | ... |
    **Status:** Approved
    **Source Ideas:** <slug>

Markdown lazy-continuation means prettier folds the plain **Status:**/**Source Ideas:** lines INTO the preceding blockquote (prefixing them with '> '). After that, 'specscore spec lint' can no longer read them and fails with:
- feature-index-row-sync (README declares Status 'Unknown')
- feature-source-ideas-required (missing **Source Ideas:** line)

Fix: emit a blank line between the Studio blockquote and the metadata block in the scaffold template (a single blank line is prettier-stable and lint still accepts it). Affects 'specscore feature new'. Consider auditing idea/plan scaffolds for the same lazy-continuation hazard.
