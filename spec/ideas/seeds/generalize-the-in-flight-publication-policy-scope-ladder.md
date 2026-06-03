---
type: sidekick-seed
slug: generalize-the-in-flight-publication-policy-scope-ladder
captured_at: 2026-06-03T10:22:37Z
captured_by: user
captured_during: spec/plans/approval-autonomy.md
trigger: explicit
status: queued
synchestra_task: null
---

# Generalize the in-flight publication-policy scope-ladder config resolver into a shared specscore CLI config resolver (get/set/resolve across run/session/project/user) that also serves autonomy.implement.* knobs like commit_cadence

Surfaced while shipping specstudio 0.0.8 (approval-autonomy layer). The implement Feature spec'd `autonomy.implement.commit_cadence` as resolvable across the **run / session / project / user** scope ladder — the same ladder the in-flight `cli/publication-policy` Feature (Draft; partial code in `pkg/publication/`, `internal/cli/publication.go`; not yet in shipped 0.5.0) builds for publication policy. Project scope already works by hand-editing `specscore.yaml`; the run/session/user scopes need a state store + resolver that today only publication is building.

Autonomy is the **second consumer** of scope-ladder config, which tips the publication-policy Feature's own Open Question ("config under `specscore publication set` vs a broader `specscore config set ...` namespace?") toward a **shared resolver**. Proposal: land the publication-policy resolver, then generalize it (or extract a common `config get/set/resolve --scope ...` core) so `autonomy.implement.*` reuses it rather than duplicating ladder logic. Note: the skills read this config directly from YAML, so the CLI is needed only for the non-file (run/session/user) scopes and deterministic resolution output — not for basic project-scope use.
