---
kind: relationship
id: team-member
name: teamMember
status: draft
from: identity.team
to: directory.contact
cardinality: many-to-many
metadata:
  role: modelspec://identity.TeamRole
summary: A Contact participates in a Team with a role.
---

# Relationship: teamMember

## Description

A Contact participates in a Team with a role.

## Open Questions

None at this time.
