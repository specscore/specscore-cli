---
format: https://specscore.md/scenario-specification
---

# Rehearse: stable-remote-repo-ids

**Status:** pending
**Verifies:** cli/studio/index#ac:stable-remote-repo-ids (REQ: repo-identity)

Scenario source: [../README.md](../README.md) → `### AC: stable-remote-repo-ids`.

Given two workspace repositories both checked out into directories named `backstage`, with origins `https://github.com/sneat-co/backstage.git` and `git@github.com:dal-go/backstage.git`, when I index them in either workspace order, then their facts use repository IDs `github.com/sneat-co/backstage` and `github.com/dal-go/backstage` in both runs, with no basename collision suffix.

---
*This document follows the https://specscore.md/scenario-specification*
