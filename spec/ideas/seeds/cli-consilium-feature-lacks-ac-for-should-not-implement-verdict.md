---
captured_by: specstudio:plan
status: queued
---
# cli/consilium Feature has no AC exercising the should-not-implement terminal verdict

The `cli/consilium` gate engine (REQ `gate-engine-rule-order`) defines three terminal verdicts, but the Acceptance Criteria only exercise `should-implement` (AC `gate-engine-applies-rules-in-order`) and `needs-human-review` (ACs `low-confidence-abstain-caps-verdict`, `adversary-veto-blocks`). Step 12 — `should-not-implement` when builders AND customers both vote majority against — has no dedicated AC. The plan covers the path (Task 5 implements it; Task 7's snapshot suite exercises it), so this is a spec-completeness gap, not an implementation gap. Surfaced by the plan-document reviewer during specstudio:plan of cli-consilium.

Fix options to evaluate: (a) add an AC to `cli/consilium` that drives `should-not-implement` directly; (b) rely on the Task-7 snapshot fixture and add a coverage note; (c) leave as-is and document the asymmetry. Lean (a) — a terminal verdict with no AC is a verification blind spot. Needs a Feature revision via specstudio:specify.
