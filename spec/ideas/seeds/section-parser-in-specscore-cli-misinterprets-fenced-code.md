---
captured_by: user
status: resolved
---

# section parser in specscore CLI misinterprets fenced code blocks containing literal `##` lines as new sections, breaking section-scoped lint rules like idea-hmw-framing

---

**Triage 2026-06-03 — CONFIRMED OPEN at HEAD.** Root cause: `func Parse` in `pkg/idea/parse.go` (line scan ~223-263) does not track fenced-code-block state, so a `## ` (or `# `) line inside a ``` ``` ```/`~~~` fence is misclassified as a section heading — creating a spurious section AND truncating the real section's `Body` at the fence. Affected lint rules (all in `pkg/lint/idea.go`): `idea-hmw-framing` (l.474), `idea-required-sections` (l.393), `Not Doing (and Why)` (l.439), `Key Assumptions to Validate` (l.450). Same un-guarded pattern at `pkg/idea/discover.go:214`. Minimal fix: track an `inFence` flag in the Parse loop and skip heading/item detection while inside a fence.

**Resolved 2026-06-03:** `pkg/idea/parse.go::Parse` fixed in PR #28; `pkg/idea/discover.go::parseSourceIdeas` fixed in the follow-up PR (both now track fence state).
