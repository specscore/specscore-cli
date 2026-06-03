---
type: sidekick-seed
slug: cli-consilium-feature-lacks-ac-for-should-not-implement-verdict
captured_at: 2026-06-03T12:30:00Z
captured_by: specstudio:plan
captured_during: spec/features/cli/consilium
trigger: heuristic
status: queued
synchestra_task: null
---
# cli/consilium Feature has no AC exercising the should-not-implement terminal verdict

The `cli/consilium` gate engine (REQ `gate-engine-rule-order`) defines three terminal verdicts, but the Acceptance Criteria only exercise `should-implement` (AC `gate-engine-applies-rules-in-order`) and `needs-human-review` (ACs `low-confidence-abstain-caps-verdict`, `adversary-veto-blocks`). Step 12 — `should-not-implement` when builders AND customers both vote majority against — has no dedicated AC. The plan covers the path (Task 5 implements it; Task 7's snapshot suite exercises it), so this is a spec-completeness gap, not an implementation gap. Surfaced by the plan-document reviewer during specstudio:plan of cli-consilium.

Fix options to evaluate: (a) add an AC to `cli/consilium` that drives `should-not-implement` directly; (b) rely on the Task-7 snapshot fixture and add a coverage note; (c) leave as-is and document the asymmetry. Lean (a) — a terminal verdict with no AC is a verification blind spot. Needs a Feature revision via specstudio:specify.
