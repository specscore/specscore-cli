# Handout: Studio Phase 1 — probes, freshness, contradictions

**For:** a fresh session (`~/projects` layout). **Written:** 2026-07-10 after Phase 0 shipped. Repo: `~/projects/specscore/specscore-cli`. Confirm founder delegation for gates/commits at session start. Read the sibling handout `rehearse-v0.4-evidence-emission.md` first — v0.4 may already be shipped by the time you run; its `verified-behavior` class and rehearse adapter are Phase-1 building blocks (coordinate, don't duplicate).

## 1 · Verified state (Phase 0)

`specscore studio index|facts` on main: `studio.yaml` workspace → four adapters (specscore, codegraph, manifests, registries) → rebuildable SQLite fact store + per-repo INGR export + `facts` filter verb. Fact shape: s/p/o + evidence_class (`declared`|`derived`) + pointer + adapter{id,version} + observed_at + ecosystem. Dogfood: 10,377 facts over ~85 Sneat repos, 0 warnings. Code `internal/studio/*`; spec `spec/features/cli/studio/index/` (Implementing, grade A). Design corpus: `~/projects/specscore/specstudio-skills/spec/research/studio-design-2026-07/` — **normative for this phase**: `03-entity-and-evidence-model.md` (evidence ladder, contradiction items, freshness) and `01-ux-philosophy.md` (the 50-question benchmark; Phase-1 exit = 40/50 answerable with citations).

## 2 · Phase-1 goals (CLI-first; no web UI this phase)

1. **Probes adapter → `verified-behavior` facts** (new evidence class; shared with rehearse v0.4): live HTTP checks of domain facts (`serves-status` verified by actually curling — the registries adapter's declared statuses get behavioral confirmation), CI state via `gh api` (repo → latest main run conclusion), deploy state where derivable. Probes are OPT-IN per run (`studio index --probe` or a separate `studio probe` verb — design call; recommendation: separate verb writing into the same store so `index` stays offline-pure and fast).
2. **Freshness as data**: `verified_at` distinct from `observed_at` where re-verification happens; `facts` gains `--stale <duration>` filter and renders age; per-evidence-class default cadences documented (design 03).
3. **Contradiction items**: `studio contradictions` verb computing at least the two proven detector classes from the Sneat review: (a) status-vs-behavior drift (spec `has-status` Approved/Draft while domain/deploy facts say live — the write-only-lifecycle detector), (b) same-predicate disagreement between sources (the `ext-<id>` vs `<id>-contract` naming-conflict class: two `declared` facts, same subject+predicate, different objects, different pointers). Output: human list + `--format json`; each item carries both evidence sets. These are FACTS too (`contradicts` predicate) so `facts` can query them.
4. **Alias resolution**: `aliased-as` facts exist (registries adapter); add `studio resolve <name>` (or `facts --resolve`) mapping brand/domain/repo/package → canonical entity id; used by the benchmark questions ("SizeChart" → sizeus).
5. **Question library seed**: `studio ask "<question>"` v0 — NOT an LLM; a router over a small library of parameterized DTQL/SQL question templates (who-fronts, what-uses, version-pins, status-of, contradictions-for) with citations (fact ids + pointers). The 50-question benchmark file drives which templates exist. LLM-backed answering is Phase 2 (MCP) — keep this deterministic.

**Exit gate:** a benchmark script (commit it: `spec/features/cli/studio/**/benchmark/` or docs) runs the 50 questions from the design's 01 file against a Sneat workspace index; ≥40 answered with citations. Plus: `studio contradictions` on the Sneat workspace surfaces at least the known real drift class without false-positive flood (spot-check ≤20% noise).

## 3 · Design decisions to make at specify (recommendations)

- Probe scheduling stays manual/CI-cron in this phase (no daemon).
- Contradiction suppression: an `attested-exception` mechanism is Phase 2 (needs the durable attestation log per design 10) — Phase 1 may ship a simple ignore-list file in the workspace dir.
- The 50-question benchmark needs ~8-12 question *templates*; map benchmark→template coverage in the Feature so the 40/50 gate is checkable mechanically.
- Feature slugs: suggest `cli/studio/probe`, `cli/studio/contradictions`, `cli/studio/resolve`, `cli/studio/ask` as sibling features under `cli/studio/` (one plan can span? NO — one Feature per plan; either one umbrella feature `cli/studio/phase-1` with topics, or 2 features (probe+freshness / contradictions+resolve+ask). Specify-time call; the reviewer will police scope.)

## 4 · Pipeline, prerequisites, pointers

House flow: `/specstudio:specify` → plan → serial implement subagents; gates in `specscore.yaml` (ai reviewer prompt at `.specscore/reviewers/specify-feature-reviewer.md` + human). Coverage gate 100% (`scripts/coverage-gate.sh`). Prereqs: Go, gh authed, network for probes, a Sneat checkout for dogfooding (`studio.yaml` over `~/projects/sneat-co/*`). Pointers: design `~/projects/specscore/specstudio-skills/spec/research/studio-design-2026-07/` (01, 03, 10, 11); governing roadmap `11-roadmap.md`; session ledger `~/.claude/projects/-Users-alex-projects/memory/ecosystem-review-2026-07.md`.

## 5 · Post-Phase-1 (envisioned, from the design)

Phase 2: MCP server (`orient()`, `answer()`, `gotchas()`, write-back with quarantine + attestation log in inGitDB) — the agents-as-co-users bet and Studio's product/feature decision point. Phase 3: ModelSpec projection discovery + trace panels. Phase 4: OVDB-backed multi-tenant + enterprise lenses. The web UI enters at Phase 1.5–2 as a thin renderer of the same engine (islands/freshness dots per design 01/09).
