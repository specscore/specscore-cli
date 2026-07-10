# Handout: Rehearse v0.4 — evidence emission into `studio index`

**For:** a fresh Claude session on any machine with the standard `~/projects` layout.
**Written:** 2026-07-10, immediately after v0.3 shipped. **Repo:** you are reading this inside `specscore-cli` — all work lands here on `main` (direct commits pre-approved historically; confirm delegation with the founder at session start).

## 1 · Where things stand (verified state at handout time)

- **Rehearse v0.3 SHIPPED** on this repo's main: `specscore rehearse run` — five block kinds (`bash`, `hurl` HTTP-delegated, `sql` sqlite, `dtql` via dalgo+dalgo2sqlite, `graphql` compiled onto hurl), scenario-scoped **context bag** (textual `{{}}` for bash/sql/dtql; `--variable` flags for hurl-derived; captures via `$REHEARSE_CAPTURES` / `-- capture:` / Hurl `[Captures]` / `-- capture-jsonpath:`), human+JSON reports (`{file,status,verifies,duration_ms,bag,steps}`), exit 0/1/2, upfront missing-binary skip. Code: `internal/rehearse/{scenario,runner,blocks/*}` + `internal/cli/rehearse.go`. Spec: `spec/features/cli/rehearse/run/` (Status Implementing, grade A) + plan `spec/plans/cli-rehearse-run.md` (Implemented). 21-scenario self-hosting corpus green in CI (`Rehearse corpus` job in `.github/workflows/go-ci.yml`, hurl 8.0.1 pinned via .deb).
- **Studio Phase 0 SHIPPED** same repo: `specscore studio index|facts` — `internal/studio/{workspace,fact,store,adapters/*,ingr}`. Fact shape: subject/predicate/object + `evidence_class` (**currently only `declared`|`derived`**) + evidence_pointer + adapter{id,version} + observed_at + ecosystem. Four adapters (specscore, codegraph, manifests, registries) registered one-per-line in `internal/studio/adapters/adapters.go` `All()`. Spec: `spec/features/cli/studio/index/`; design: `~/projects/specscore/specstudio-skills/spec/research/studio-design-2026-07/` (esp. `03-entity-and-evidence-model.md` — the evidence ladder).
- **Product direction**: `~/projects/specscore/rehearse/spec/ideas/rehearse-evidence-layer.md` — the governing Idea (roadmap, coupling rationale, founder addenda). Coverage discipline: `scripts/coverage-gate.sh` must stay at 100%; specs are lint-gated (`specscore spec lint`).

## 2 · v0.4 goal

**Rehearse results become `verified-behavior` facts** — the top rung of the Studio evidence ladder. Success gate (from the Idea, verbatim): *"Studio `facts --class verified-behavior` returns real rows for cli/studio/index ACs."*

### Design decisions to make during specify (recommendations included)
1. **Producer/consumer decoupling (recommended):** `rehearse run` gains `--report-out <path>` (or a workspace-conventional location, e.g. `<repo>/.specscore/rehearse/latest.json`) persisting the JSON report; a **fifth studio adapter** (`internal/studio/adapters/rehearse`) ingests persisted reports during `studio index`, emitting one fact per scenario-AC pair: `(<ac-id>, verified-by, <scenario-file>)` and/or `(<ac-id>, has-verification-status, pass|fail)` with `evidence_class: verified-behavior`, `evidence_pointer` = report path + scenario file, `observed_at` = the run timestamp (NOT index time — staleness must be honest). Alternative rejected in the design: `studio index` running scenarios itself (probe-style) — couples indexing time to test time; keep them decoupled.
2. **New evidence class:** extend `internal/studio/fact` `Class` set with `verified-behavior`; `studio facts --class verified-behavior` must filter it (existing flag). Update REQ fact-shape via a feature revision (the Feature says "declared|derived **in this feature**" — v0.4 amends the studio feature or, cleaner, the rehearse-adapter feature declares the new class; follow whichever the specify reviewer prefers).
3. **Report retention:** recommend keeping only `latest.json` per repo in v0.4 (history is a Studio-hosted concern later); the report should include the runner version + git SHA of the working tree (`git rev-parse HEAD`, dirty-flag) so facts carry provenance.
4. **Also in v0.4 per the Idea (small):** a `file` assert block was originally roadmapped — OPTIONAL now (sql/dtql/captures were pulled forward into v0.3); include only if cheap, else park to v0.5.
5. **Standalone-binary decision** is due at v0.4 per the Idea's Open Questions — decide from demand (recommendation: skip the separate binary; `specscore rehearse` with no spec tree already IS standalone mode).

## 3 · How to run the work (house pipeline)

1. Read this handout + the governing Idea + `spec/features/cli/rehearse/run/README.md` + `spec/features/cli/studio/index/README.md` + `internal/studio/adapters/adapters.go`.
2. `/specstudio:specify` a new feature — suggested slug `cli/rehearse/evidence` (or `cli/studio/index/rehearse-adapter` — specify-time call) with REQs for: report persistence (`--report-out` + conventional path), the adapter (discovery of reports, fact emission incl. AC-id parsing from `verifies`, pass AND fail facts — failures are evidence too), the new class end-to-end through `facts`, provenance fields, and the success-gate AC (index this repo's own workspace after a corpus run → `facts --class verified-behavior` non-empty for `cli/studio/index#ac:*`). Gates: `gates.specify` = ai reviewer (`.specscore/reviewers/specify-feature-reviewer.md`) + human (founder delegation if granted).
3. `/specstudio:plan` (~4 tasks: report-out; fact class + adapter; facts/query surface + provenance; self-hosting scenario + CI + docs) → implement via serial subagents (the proven recipe: stage-only subagents, batch commits with `Verifies:` trailers, coverage gate before every commit).
4. Machine prerequisites: `specscore` CLI on PATH (self-update), `hurl` (brew install hurl), `gh` authed, Go per go.mod.

## 4 · Post-v0.4 roadmap (envisioned, from the Idea + Studio design)

- **v0.5 — authoring ergonomics:** `specscore rehearse new <ac-id>` scaffolds a scenario from an AC's Given/When/Then; `specstudio:implement` skill instructs subagents to author scenarios via it (institutionalizing what the Studio/Rehearse builds did by hand). Gate: a full specify→implement pipeline run produces passing scenarios with zero hand-authoring.
- **v1.0 — declared stable:** format spec published in `~/projects/specscore/rehearse` (rehearse.ink becomes truthful — currently marketing-ahead-of-substance); ≥3 in-house repos with scenario suites in CI; standalone quickstart. Decide the repo/module home (`github.com/synchestra-io/rehearse` module path is orphaned — likely: rehearse repo keeps the format spec, runner code stays here).
- **Studio-side follow-ups that consume v0.4:** freshness dots driven by `observed_at` on verified-behavior facts; contradiction items when a fact's AC has `has-status: Stable` but latest verification is `fail` (the docs-vs-reality detector, now for behavior); `studio ask` (Phase-0's sibling feature) citing verification facts in answers.
- **Known nits inherited:** duplicate `fronts` rows when two registry files agree (dedup policy); studio Feature wording `internal/cli/studio` vs actual `internal/cli/studio.go` (recap note); rehearse scenarios assume `sqlite3`/`python3` on PATH (documented).

## 5 · Context pointers (full paths, same on every machine)

- This repo: `~/projects/specscore/specscore-cli`
- Governing Idea: `~/projects/specscore/rehearse/spec/ideas/rehearse-evidence-layer.md`
- Studio design (evidence ladder): `~/projects/specscore/specstudio-skills/spec/research/studio-design-2026-07/03-entity-and-evidence-model.md`
- Ecosystem session ledger (memory): `~/.claude/projects/-Users-alex-projects/memory/ecosystem-review-2026-07.md`
- Sneat workspace for dogfooding an index run: any `studio.yaml` over `~/projects/sneat-co/*`
