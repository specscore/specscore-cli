---
format: https://specscore.md/feature-specification
status: Implementing
---

# Feature: studio answers — contradictions, alias resolution, and the deterministic question router

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/answers?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/answers?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/answers?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/answers?op=request-change) |
**Status:** Implementing
**Source Ideas:** —
**Supersedes:** —
**Grade:** A

## Summary

Studio's unit of UX is the **answered question** (design 01 §"Design around questions, not entities"). This feature is the CLI's first answering engine — deterministic, offline, citation-carrying — and the mechanism that turns *disagreeing* evidence into content. It adds three sibling verbs to the `studio` group that read the same fact store `studio index` builds and `studio probe` enriches:

- `specscore studio contradictions` computes two proven detector classes over the store: **status-vs-behavior drift** (a `declared` claim contradicted by a `verified-behavior` observation — a feature `has-status` Stable while the latest `has-verification-status` is `fail`, or a registry-declared `serves-status` of `200` while the live probe says `down`: the write-only-lifecycle and dead-investor-CTA classes) and **same-predicate disagreement** (two `declared` facts with the same subject+predicate, different objects, different evidence pointers — the `ext-<id>` vs `<id>-contract` naming-conflict class). Each item carries **both** evidence sets and is itself written back as a `contradicts` fact so `studio facts` can query it.
- `specscore studio resolve <name>` maps a brand / domain / repo / package / product name to its canonical entity id, case-insensitively, using the registries adapter's `aliased-as` facts and the entity ids already in the store (the "SizeChart → sizeus" vocabulary layer of design 01 §Cognition).
- `specscore studio ask "<question>"` v0 — **not an LLM** — is a deterministic router over 13 parameterized question *templates* implemented as store queries. Every answer carries citations (the fact ids / evidence pointers it stands on); an unroutable question exits non-zero and prints the template list.

The committed **benchmark** (`benchmark/questions.jsonl` + `benchmark/run.sh`) holds exactly 50 concrete question instances derived from design 01's harvested question list, mapped to templates, with genuinely out-of-Phase-1 questions marked `expected-unanswerable` so the file stays honest. The Phase-1 exit gate — **≥40/50 answered with citations** — is checkable mechanically: a deterministic CI assertion over a committed fixture workspace, plus a scripted human-runnable run against the Sneat dogfood workspace. This is the second half of Studio Phase 1 (`docs/continuations/studio-phase-1-evidence-freshness.md` §2 goals 3–5, §exit gate); the first half (`cli/studio/probe`) produced the `verified-behavior` facts these detectors and answers consume.

## Problem

Phase 0 + `cli/studio/probe` built a fact store rich enough to answer real questions — 10,377 facts over ~85 Sneat repos, plus behavioral `serves-status`/`ci-status` and rehearse's `has-verification-status` — but nothing *reads* it as answers. Three gaps remain from the Sneat review, each named in design 01:

1. **Contradictions are invisible.** The store now holds a `declared` `serves-status` from `domains.json` next to a `verified-behavior` one from a live probe, and a `has-status` "Stable" next to a `has-verification-status` "fail" — coexisting by design (`cli/studio/probe#req:domain-liveness`, `cli/rehearse/evidence#req:verification-facts`). But disagreement between them is inert data. The review's entire docs-vs-reality chapter is exactly this: an investor-CTA domain "live" in a registry while dead; a status sitting at "Approved" for weeks after shipping; two `declared` facts naming the same subject differently (`ext-<id>` vs `<id>-contract`) that "sat unnoticed for 11 days in files." Design 01 §"Contradictions are content": *"it should not survive 11 minutes in Studio."*
2. **Vocabulary fails search.** Design 01 §Cognition names alias resolution the second cognitive layer — *"Without this layer, every search fails."* `aliased-as` facts exist (registries adapter) but there is no verb that takes "SizeChart" and returns `sizeus`.
3. **You must know the entity to find the answer.** `studio facts` is a fact filter, not a question answerer; the operator must already know the subject, predicate, and store idioms. Design 01 inverts this: the answered question is the destination, the entity page the fallback. Phase 1 owes a *deterministic* first cut (LLM answering is Phase 2 MCP) plus the 50-question benchmark that is the phase's exit gate.

## Behavior

### Contradiction detection

#### REQ: contradictions-verb

`specscore studio contradictions` is a new subcommand of the `studio` group, a sibling of `index`, `probe`, and `facts`. It reads the same fact store (`--workspace`, default `./studio.yaml`; `--db`, default `<workspace-dir>/.specscore-studio/facts.db`) and performs **no network I/O** — it is a pure query over the store `index`/`probe` already wrote. It computes the detector classes below, prints a human list by default and a JSON array with `--format json`, and (unless `--no-write`) writes each detected item back as a `contradicts` fact via the store merge (`contradiction-facts`). Running before any `index` exits 2 with the missing-store guidance (reusing `cli/studio/index#req:facts-query`). Each item — in both output formats — carries **both evidence sets**: for each of the two sides, the fact's subject, predicate, object, evidence class, evidence pointer, adapter id, and `observed_at`, plus a `detector` id naming which class flagged it. A store with no contradictions exits 0 and prints an empty list (JSON `[]`) — absence of conflict is a valid, reportable answer.

#### REQ: status-vs-behavior-drift

The **status-drift** detector flags a `declared` claim that a `verified-behavior` observation contradicts, in two branches — both grounded on facts the pipeline actually emits:

- **lifecycle-vs-verification (the write-only-lifecycle class):** a spec entity carries a `has-status` fact (the specscore adapter's subjects: `<repo-slug>#<feature-id>` / `<repo-slug>#ideas/<slug>`) whose object is a *shipped-implying* status (`Approved`, `Stable`, `Implementing` — the "declared as done/live" set) while a joinable `verified-behavior` `has-verification-status` fact is `fail`. The join is by subject prefix: a `has-status` on `<repo-slug>#<feature-id>` pairs with any `has-verification-status` of `fail` on `<repo-slug>#<feature-id>#ac:<id>` (the AC-ref subject scheme the rehearse adapter mints, `cli/rehearse/evidence#req:verification-facts`).
- **declared-vs-verified disagreement (the dead-investor-CTA class):** two facts share the same `subject` and `predicate`, one side `declared` and the other `verified-behavior`, with **different objects** — most concretely the registries adapter's `declared` `(<domain>, serves-status, 200)` from `domains.json` against the probe's `verified-behavior` `(<domain>, serves-status, down)`, the exact coexisting pair `cli/studio/probe#req:domain-liveness` produces as this feature's raw material. A `declared` and a `verified-behavior` fact *agreeing* on the object is behavioral confirmation, never flagged.

Each flagged pair becomes one contradiction item whose two sides are the `declared` fact and the contradicting `verified-behavior` fact. The class partition keeps the noise discipline (the ≤20%-noise exit-gate condition): only `verified-behavior` evidence can overturn a `declared` claim here — `declared`-vs-`declared` is the *other* detector's job (`same-predicate-disagreement`), and `verified-behavior`-vs-`verified-behavior` is supersession, flagged by neither. A store with no probe/rehearse facts therefore yields no status-drift items (no false positives from declaration alone).

#### REQ: same-predicate-disagreement

The **naming-conflict** detector flags two facts that share a `subject` and a **single-valued** `predicate` but assert **different objects** from **different evidence pointers** — the `ext-<id>` vs `<id>-contract` class where two human-authored registries disagree on the same relationship. It fires **only on the fixed single-valued declared predicate set** `{has-status, serves-status, fronts}` — predicates whose semantics admit exactly one object per subject (a subject has one lifecycle status; a domain serves one status; a domain fronts one product), so two declared objects genuinely disagree. Legitimately **multi-valued** predicates (`has-ac`, `contains`, `implemented-by`, `aliased-as`, `member-of`, `consumes`, …) are excluded: two different objects there are two valid values, not a disagreement — without this scoping every multi-AC feature self-"conflicts" (the Sneat dogfood run flooded 5,758 items, 92% of them `has-ac`; see Autonomous Decisions). It fires only when *both* sides are `declared`: a `declared` side disagreeing with a `verified-behavior` side belongs to `status-vs-behavior-drift`'s declared-vs-verified branch, and two behavioral observations of the same predicate with different objects are supersession, not contradiction — a domain that was `200` and is now `down` is `cli/studio/probe`'s changed-object case, expressly flagged by neither detector. Facts that share subject+predicate+object (the same claim from two sources — agreement) are never flagged. Each flagged pair becomes one contradiction item; when N>2 sources disagree, the detector emits one item per distinct unordered pair so every disagreement is individually citeable.

#### REQ: contradiction-facts

Each detected item is written back into the store as a `contradicts` fact so `studio facts --predicate contradicts` can query the conflict set. The fact shape (recorded as an autonomous decision): **subject** = the first side's fact ref `<subject>|<predicate>|<object>` (a stable, pointer-free triple key), **object** = the second side's fact ref in the same form, **predicate** = `contradicts`, **evidence_class** = `derived` (a contradiction is computed from other facts, not observed by executing the system), **evidence_pointer** = the detector id (`status-drift` or `naming-conflict`) — the "query that reproduces the observation" is the named detector, **adapter id** = `contradictions`, version = the CLI version, **observed_at**/**verified_at** = the run time. The two ordered sides are canonicalised (lexicographically smaller fact-ref first) so the same conflict yields one stable `contradicts` fact across runs and the store merge (`cli/studio/probe#req:probe-merge`, keyed on subject+predicate+object+class+adapter) is idempotent — re-running `contradictions` refreshes `verified_at`, never duplicates. `--no-write` computes and prints items without touching the store.

#### REQ: suppression-ignore-list

An operator can suppress known, accepted contradictions with a plain ignore-list file at `<workspace-dir>/.specscore-studio/contradictions-ignore.txt`: one contradiction identity per line (the canonical `<side-a-ref>  <side-b-ref>` pair, `#`-prefixed comments and blank lines ignored), overridable with `--ignore-file <path>`. A suppressed item is omitted from both the printed list and the `contradicts` write-back; a `--show-ignored` flag lists what was suppressed (identity + the reason comment on the matching line, if any) so suppression is never silent. This is a deliberately simple, workspace-local mechanism; the durable `attested-exception` log is Phase 2 (`## Not Doing`).

### Alias resolution

#### REQ: resolve-verb

`specscore studio resolve <name>` maps a brand, domain, repo slug, package, or product name to its canonical entity id, reading only the store (offline). Resolution is **case-insensitive** and draws candidates from two sources: (a) the object of every `aliased-as` fact (brand → its product-id subject) and (b) the entity ids the store already knows as subjects/objects (product ids, repo slugs, domain ids). A unique match prints the canonical id (and, with `--format json`, an object naming the id, the kind inferred from its shape, and the citation — the `aliased-as` fact ref or the id's own first fact ref) and exits 0. The benchmark's canonical case — `studio resolve SizeChart` → `sizeus` via the `aliased-as` fact `(sizeus, aliased-as, SizeChart)` — is the acceptance anchor.

#### REQ: resolve-ambiguous-and-unknown

When a name resolves to **more than one** candidate canonical id (e.g. a brand string reused across two products), `resolve` lists all candidates (each with its citation) and exits 5 (the house `AmbiguousSlug` code — the caller must disambiguate). When a name resolves to **nothing**, `resolve` exits 3 (`NotFound`) with guidance naming what was searched (aliases + entity ids) and suggesting `studio facts --object '<name>*'` to explore. An empty or whitespace-only `<name>` is a usage error (exit 2). Resolution never invents an id it cannot cite.

### The deterministic question router

#### REQ: ask-verb-and-router

`specscore studio ask "<question>"` answers a natural-language question by routing it — **deterministically, with no LLM** — to one of a fixed library of parameterized *templates*, each backed by a store query. The router lowercases the question, matches it against each template's trigger pattern (keyword + a captured parameter, e.g. "who fronts <X>", "what repos implement <X>", "status of <X>", "is <X> live", "aliases of <X>"), resolves the captured parameter through `resolve` semantics (`resolve-verb`) where the template targets an entity, runs the template's store query, and renders the answer. A routed answer always carries **citations**: the evidence pointer and adapter id of every fact the answer is built from (`ask-citations`). `--format json` returns `{question, template, parameter, answer, citations}`; human format prints the answer then an "Evidence:" block. The template library is documented in `## Question templates` and surfaced by `studio ask --list`.

#### REQ: ask-citations

Every answer `studio ask` returns is backed by store facts and names them: the `citations` array lists, per supporting fact, its subject, predicate, object, evidence class, evidence pointer, and adapter id. An answer with zero supporting facts is **not** an answer — it routes to the unanswerable path (`ask-unroutable`), never a citation-free assertion (design 01 §"Never render a claim without its chip"). This is the property the benchmark's "answered *with citations*" gate checks: `ask` output counts toward the 40/50 only when `citations` is non-empty.

#### REQ: ask-unroutable

A question matching no template exits 1 and prints the routable template list (the same content as `--list`) with a one-line "no template matched" notice — the honest "I can't answer that yet" the design demands over a hallucinated answer. A question that routes to a template but whose parameter resolves to no facts (a real entity with no data, or a mis-typed name) exits 3 with "routed to <template> but found no facts for <parameter>" — distinguishing *unroutable* (exit 1, no template) from *routed-but-empty* (exit 3, no data), so the benchmark runner can tell "template gap" from "data gap".

### The committed benchmark and exit gate

#### REQ: benchmark-file

A committed benchmark file `benchmark/questions.jsonl` holds **exactly 50** question instances, one JSON object per line: `{id, question, template, parameter, expectation}` where `expectation` is `answerable` or `expected-unanswerable`. The 50 are derived from design 01's harvested question list (§"The question benchmark"), parameterised per entity to reach 50 honest instances. Questions genuinely outside Phase-1 scope — "why does this exist", operational gotchas ("what secrets does this pipeline need"), commercial plan-signals ("what plan-signal is the waitlist showing") — are **included and marked `expected-unanswerable`**, so the file honestly represents the full question surface and the gate measures real coverage, not a curated-to-pass subset. The mapping of instances → templates and the answerable/unanswerable split is documented in `## Benchmark composition` and MUST stay in sync with the file (a benchmark-composition check is part of the runner, `benchmark-runner`).

#### REQ: benchmark-runner

A committed runner `benchmark/run.sh` executes all 50 instances against a fact store built from a workspace, driving `studio ask`/`studio resolve`/`studio contradictions` per each instance's template, and reports `answered-with-citations / 50`. An instance counts as **answered** when its verb exits 0 **and** emits at least one citation (`ask-citations`); an `expected-unanswerable` instance counts as **correctly-handled** when its verb exits non-zero (routes to the unanswerable path) — the runner reports both the answered count and an "honesty" count (expected-unanswerables that correctly did not answer), and fails if any `expected-unanswerable` instance is answered anyway (a silent hallucination is a harder failure than a miss). Because `contradictions-for` instances query `contradicts` facts that only exist after detection has run, the runner runs `specscore studio contradictions` (writing its facts into the target store) before evaluating any instance. The runner takes a `--db`/`--workspace` target so it runs against either the committed fixture or a live Sneat index.

#### REQ: exit-gate-fixture-and-sneat

The Phase-1 exit gate is **≥40/50 answered with citations**, asserted two ways for honesty and CI determinism:

- **CI (deterministic):** a committed fixture workspace under `benchmark/testdata/fixture/` (a small, hand-authored set of repos/registries/reports exercising every answerable template) is indexed and probed with stubbed seams, then `benchmark/run.sh --db <fixture-db>` asserts a fixed floor over the fixture-answerable subset (every `answerable` instance whose entity exists in the fixture is answered; every `expected-unanswerable` correctly declines). This runs in CI and is hermetic (no network).
- **Sneat dogfood (manual gate):** `benchmark/run.sh` against a Sneat workspace index (`~/projects/sneat-co/*`, the dogfood target) must report **≥40/50**. This is a scripted, human-runnable scenario (documented in `## Exit gate`), not a CI job — the Sneat checkout and live network are not CI-available, so the 40/50 figure is the reviewer-runnable phase gate while the fixture assertion guards regressions continuously.

Both share one runner and one benchmark file — the fixture proves the machinery, the Sneat run proves the coverage.

## Question templates

The router (`ask-verb-and-router`) supports these 13 templates. Each is a store query over predicates the Phase-0/probe/rehearse adapters already emit (verified against the adapter code); `Cites` names the fact(s) each answer stands on.

| Template | Trigger (lowercased) | Store query | Cites |
|---|---|---|---|
| `who-fronts` | "who fronts X" / "what fronts X" | `fronts` where object = resolve(X) | the `fronts` fact(s) |
| `what-repos-implement` | "what repos implement X" / "which repo implements X" | `implemented-by` where subject = resolve(X) | the `implemented-by` fact(s) |
| `status-of` | "status of X" / "is X approved/stable" | `has-status` where subject = resolve(X) | the `has-status` fact |
| `aliases-of` | "aliases of X" / "what is X called" | `aliased-as` where subject = resolve(X) | the `aliased-as` fact(s) |
| `member-of` | "what vertical is X in" / "member of" | `member-of` where subject = resolve(X) | the `member-of` fact |
| `is-it-live` | "is X live" / "does X serve" | `serves-status` (`verified-behavior`) for domains of resolve(X) | the probed `serves-status` fact |
| `ci-status-of` | "ci status of X" / "is X green" | `ci-status` (`verified-behavior`) where subject = resolve(X) | the `ci-status` fact |
| `what-verifies` | "what verifies X" / "is X tested" | `verified-by` / `has-verification-status` where subject prefix = resolve(X) | the rehearse facts |
| `contradictions-for` | "contradictions for X" / "does X conflict" | `contradicts` where side references resolve(X) | the `contradicts` fact(s) |
| `freshness-of` | "how fresh is X" / "when was X verified" | any fact about resolve(X), reporting max `verified_at` | the freshest fact |
| `what-uses` | "what uses X" / "who consumes X" | `consumes` where object prefix = resolve(X), reporting the consuming subjects | the manifests `consumes` fact(s) |
| `version-pins` | "what version of X" / "which version pins X" | `consumes` where object prefix = resolve(X), reporting the pinned versions | the manifests `consumes` fact(s) |
| `aliases-resolve` | "what is X" / "resolve X" (bare id lookup) | `resolve(X)` | the resolving `aliased-as` / id fact |

Out-of-Phase-1 question shapes (why-does-this-exist, deploy-method, gotchas-of, plan-signal/commercial) have **no template** and route to the unanswerable path — they appear in the benchmark as `expected-unanswerable` (`benchmark-file`).

## Benchmark composition

The 50 instances in `benchmark/questions.jsonl` map to templates as follows: **41 `answerable`, 9 `expected-unanswerable`**. The honest answering ceiling of this file is therefore 41, so the ≥40/50 Sneat gate is clearable only if at most one answerable instance misses — the gate is deliberately tight against the file's own ceiling.

| Template | Answerable instances | Notes |
|---|---|---|
| `who-fronts` | 5 | one per fronted product/domain (anymeter.app, fillless.com, …) |
| `what-repos-implement` | 4 | anymeter→trackus class + siblings |
| `status-of` | 5 | feature and idea spec entities (the only `has-status` subjects any adapter emits) |
| `aliases-of` | 3 | anymeter, assetus, gameboard |
| `member-of` | 3 | vertical bundling |
| `is-it-live` | 4 | includes the dead investor-CTA case (fillless.com) |
| `ci-status-of` | 4 | green + red repos |
| `contradictions-for` | 4 | status-drift subjects from the real drift set |
| `freshness-of` | 2 | last-verified questions |
| `what-uses` | 3 | consumer fan-in (sneat-go-core-class questions) |
| `version-pins` | 2 | platform-version pins across consumers |
| `aliases-resolve` | 2 | bare "what is X" |
| **answerable total** | **41** | |
| `expected-unanswerable` | 9 | why-exists (3), gotchas (2), deploy-method (2), commercial (2) |
| **file total** | **50** | |

(The exact per-line counts are authoritative in `benchmark/questions.jsonl`; this table is the human-readable summary the composition check enforces against the file. The split is 41 answerable / 9 unanswerable, so the 40/50 gate is clearable while the file still honestly carries 9 genuinely-out-of-scope questions. The `what-verifies` template carries **no benchmark instances**: the Sneat dogfood workspace holds zero `verified-by`/`has-verification-status` facts — no Sneat repo has adopted `rehearse run --report-out` yet (v0.5 adoption pending), so a what-verifies instance would be unanswerable there through no fault of the router. The template itself stays in `ask` — it is exercised by the router unit tests and answerable against fixture stores carrying rehearse reports; its former 3 instances are rebalanced to `who-fronts` +1, `ci-status-of` +1, and `contradictions-for` +1.)

## Exit gate

The Phase-1 exit gate (`docs/continuations/studio-phase-1-evidence-freshness.md` §exit gate) for this feature: `benchmark/run.sh` reports **≥40/50 answered with citations** against a Sneat workspace index, and `studio contradictions` on that same workspace surfaces the known real drift class without a false-positive flood (spot-check ≤20% noise). The Sneat run is human-runnable (a `## Exit gate` scenario documenting the exact `studio index`/`studio probe`/`benchmark/run.sh` command sequence over `~/projects/sneat-co/*`); CI runs the hermetic fixture assertion (`exit-gate-fixture-and-sneat`) continuously.

## Architecture & Components

Go packages inside specscore-cli (no new binary):

- `internal/studio/contradictions` — the two detectors as pure functions over `[]fact.Fact` (input: the full store query result; output: `[]Item` where an `Item` holds the two `fact.Fact` sides + detector id). `status-drift` joins shipped-implying `has-status` facts against failing `has-verification-status` facts by subject prefix, and groups same-(subject, predicate) facts across evidence classes to pair a `declared` object against a differing `verified-behavior` one; `naming-conflict` groups `declared` facts by (subject, predicate) and pairs differing objects. Pure, fixture-testable; no store or network access. A `ToFacts([]Item) []fact.Fact` helper mints the canonicalised `contradicts` facts (`contradiction-facts`), and an ignore-list filter (`suppression-ignore-list`) reads the workspace file.
- `internal/studio/resolve` — `Resolve(facts []fact.Fact, name string) (Result, error)` returning unique / ambiguous / unknown, case-insensitive, over `aliased-as` objects and known entity ids. Pure; consumed by both the `resolve` verb and the `ask` router.
- `internal/studio/ask` — the template registry (each template: trigger matcher + a store-query builder + a renderer) and the router (`Route(question) (Template, param, bool)`). Templates are declarative so `--list` and the benchmark-composition check enumerate them. Pure of CLI wiring; the store query is injected.
- `internal/cli/studio.go` — three new subcommands (`contradictions`, `resolve`, `ask`) wired into `studioCommand()`'s `AddCommand`, sharing `--workspace`/`--db` resolution (`studioFactsStorePath`) and the `store.Query`/`store.Merge` seams already present; `contradictions` reuses the `storeMergeFn` seam for its `contradicts` write-back.
- `spec/features/cli/studio/answers/benchmark/` — `questions.jsonl` (the 50), `run.sh` (the runner), `testdata/fixture/` (the hermetic CI workspace: tiny repos + registries + a rehearse report), and `README.md` (how to run both gates).

Data flow: `studio index` (+ `studio probe`) → fact store → `contradictions` (reads store, writes `contradicts` facts back via merge) / `resolve` (reads store) / `ask` (reads store, routes via `resolve`, renders with citations) → `benchmark/run.sh` drives all three and scores answered-with-citations against the 50.

## Error Handling & Failure Modes

- No store yet (`contradictions`/`resolve`/`ask` before `index`) → exit 2, message names the store path and suggests `studio index` (reuses `cli/studio/index#req:facts-query`).
- `contradictions` on a clean store → exit 0, empty list / JSON `[]` (no conflict is a valid answer, `contradictions-verb`).
- `contradictions` write-back (store merge) failure → exit 1; the prior store is left intact via the atomic temp-file swap (`cli/studio/probe#req:probe-merge`).
- Malformed ignore-list line → the line is skipped with a `--show-ignored`-visible notice; a missing ignore file is normal (no suppression). An unreadable ignore file → exit 2 naming the path.
- `resolve` unknown name → exit 3 (`NotFound`) with guidance; ambiguous name → exit 5 (`AmbiguousSlug`) listing candidates; empty name → exit 2 (`resolve-ambiguous-and-unknown`).
- `ask` unroutable question → exit 1 + template list; routed-but-no-data → exit 3 (`ask-unroutable`); empty question → exit 2.
- Benchmark runner: any `expected-unanswerable` instance that *is* answered → runner exits non-zero (a hallucination is a hard failure, `benchmark-runner`); a malformed `questions.jsonl` line → runner exits 2 naming the line.
- `--format` other than the verb's accepted set → exit 2 (mirrors the existing studio verbs).

## Testing Strategy

Unit tests per package with fixtures, no network: the two detectors against fact-slice fixtures (status-drift's lifecycle-vs-verification and declared-vs-verified branches, declared-agreeing-with-verified not flagged, naming-conflict positive, agreement-not-flagged, behavioral-supersession-not-flagged, N-way pairing); `contradicts` fact canonicalisation + merge-idempotence; ignore-list suppression + `--show-ignored`; `resolve` unique/ambiguous/unknown/case-insensitive; the router's trigger matching, parameter capture, citation assembly, unroutable vs routed-but-empty; CLI flag/exit-code wiring for all three verbs. The benchmark's composition check (table ↔ `questions.jsonl`) is a unit test. E2e: Rehearse scenarios per testable AC under `_tests/` (scaffolded, `**Status:** pending`) driving the three verbs over a fixture store, plus a scenario running `benchmark/run.sh` against `benchmark/testdata/fixture/`. Coverage gate stays 100% (`scripts/coverage-gate.sh`).

## Not Doing / Out of Scope

- **LLM-backed answering** — `ask` is a deterministic template router this phase; natural-language understanding and generative answers are Phase 2 (the MCP `answer()` server, design 11 roadmap). Keeping v0 deterministic makes the benchmark mechanically checkable.
- **`attested-exception` / durable attestation log** — contradiction suppression is a plain workspace-local ignore file this phase (`suppression-ignore-list`); the durable, who/when-recorded attestation log is Phase 2 (design 10).
- **Web UI / freshness-dot rendering / home health strip** — this feature emits the `contradicts` facts and citation-carrying answers the UI renders from; the renderer is Phase 1.5–2 (design 01/09).
- **New probe kinds / live verification inside these verbs** — `contradictions`/`resolve`/`ask` read the store only; producing `verified-behavior` facts is `cli/studio/probe`'s job.
- **Contradiction *resolution* actions** — the one-click "fix the doc / retract the fact / attest an exception" verbs of design 01 are Phase 2 write-back; this feature *detects and reports*, it does not mutate specs.
- **Run history / contradiction time-series** — the store holds the current contradiction set; historical drift is a later Studio-hosted concern (mirrors `cli/studio/probe`'s no-history stance). Corollary: under the store's merge-only semantics a `contradicts` fact for a since-resolved conflict lingers (with an aging `verified_at`) until the next full `studio index` rebuild wipes it — this feature ships no prune logic.
- **Daemon / scheduled re-evaluation** — the verbs run once per invocation; scheduling is the operator's / CI cron's job.

## Rehearse Integration

Every AC has a CLI-observable surface (verb output, exit codes) or a fixture-store / benchmark-file surface; Rehearse stubs are scaffolded under `_tests/` (one per testable AC) with `**Status:** pending`. Detector and router scenarios use a fixture store and committed benchmark fixture so the corpus stays hermetic (no real network, no Sneat checkout in CI). The Sneat 40/50 run is documented as a human-runnable scenario (`## Exit gate`), not a CI-gated one.

## Acceptance Criteria

### AC: status-drift-verified-fail

Scenario: a Stable feature whose latest run failed is a contradiction
Given an indexed store with a `declared` `(<repo>#feat/x, has-status, Stable)` fact and a `verified-behavior` `(<repo>#feat/x#ac:y, has-verification-status, fail)` fact
When I run `specscore studio contradictions --format json`
Then the command exits 0 and the JSON contains an item with detector `status-drift` whose two sides are the `has-status` fact and the `has-verification-status` fact, each side carrying its subject, object, evidence_class, evidence_pointer, and observed_at

### AC: status-drift-dead-domain

Scenario: a registry-declared live domain that probes dead is a contradiction
Given an indexed store with `(dead.example, serves-status, 200)` with evidence_class `declared` (pointer `domains.json`) and `(dead.example, serves-status, down)` with evidence_class `verified-behavior` (the probe fact)
When I run `specscore studio contradictions --format json`
Then the JSON contains a `status-drift` item whose two sides are the declared `200` fact and the verified `down` fact, each side carrying its evidence_class, evidence_pointer, and observed_at

### AC: naming-conflict-declared-disagreement

Scenario: two declared facts disagreeing on the same single-valued predicate are a contradiction
Given an indexed store with `(d.example, fronts, ext-foo)` and `(d.example, fronts, foo-contract)`, both evidence_class `declared` with different evidence pointers (`fronts` is in the single-valued set)
When I run `specscore studio contradictions --format json`
Then the JSON contains a `naming-conflict` item whose two sides are those two facts, and the item is absent when the two facts share the same object

### AC: multi-valued-not-flagged

Scenario: two values of a multi-valued predicate are not a contradiction
Given an indexed store with two `declared` `has-ac` facts sharing a subject but carrying different objects and different evidence pointers (a feature with two acceptance criteria)
When I run `specscore studio contradictions --format json`
Then the command exits 0 and no `naming-conflict` item references those two facts (`has-ac` is not in the single-valued predicate set — two ACs are two valid values, not a disagreement)

### AC: agreement-not-flagged

Scenario: two sources asserting the same fact are not a contradiction
Given an indexed store where two `declared` facts share subject, predicate, and object but come from different evidence pointers, and no other disagreement exists
When I run `specscore studio contradictions --format json`
Then the command exits 0 and the JSON is an empty array

### AC: behavioral-supersession-not-flagged

Scenario: a changed probe result is supersession, not a same-predicate contradiction
Given an indexed store with `(d, serves-status, 200)` and `(d, serves-status, down)`, both evidence_class `verified-behavior`
When I run `specscore studio contradictions --format json`
Then no contradiction item of any detector references those two facts (differing objects across two behavioral observations are supersession — flagged by neither `naming-conflict` nor `status-drift`)

### AC: contradicts-fact-written

Scenario: detected contradictions are written back as queryable facts
Given an indexed store containing exactly one detectable contradiction
When I run `specscore studio contradictions` and then `specscore studio facts --predicate contradicts --format json`
Then the facts JSON contains a `contradicts` fact whose subject and object are the two sides' fact refs, evidence_class `derived`, adapter id `contradictions`, and evidence_pointer naming the detector, and re-running `contradictions` does not create a duplicate (same subject/predicate/object)

### AC: contradictions-ignore-suppresses

Scenario: an ignore-list entry suppresses a known contradiction
Given a store with one detectable contradiction and a `<workspace-dir>/.specscore-studio/contradictions-ignore.txt` whose single line is that contradiction's canonical `<side-a>  <side-b>` identity
When I run `specscore studio contradictions --format json`
Then the item is absent from the JSON and no `contradicts` fact is written for it, and `specscore studio contradictions --show-ignored` lists that identity as suppressed

### AC: resolve-alias-to-canonical

Scenario: a brand name resolves to its canonical entity id
Given an indexed store containing the `declared` fact `(sizeus, aliased-as, SizeChart)`
When I run `specscore studio resolve SizeChart`
Then the command exits 0 and prints `sizeus`, and `specscore studio resolve sizechart` (different case) resolves identically

### AC: resolve-ambiguous-lists-candidates

Scenario: an ambiguous name lists candidates and exits 5
Given an indexed store where the name `shared` is an `aliased-as` brand for two different product ids
When I run `specscore studio resolve shared`
Then the command exits 5 and lists both candidate canonical ids, each with a citation

### AC: resolve-unknown-guides

Scenario: an unknown name exits 3 with guidance
Given any indexed store not containing the name `nonexistent`
When I run `specscore studio resolve nonexistent`
Then the command exits 3 with a message naming what was searched (aliases and entity ids) and suggesting a `studio facts` exploration

### AC: ask-routes-with-citations

Scenario: a routable question returns an answer with citations
Given an indexed store with `(acme.app, fronts, acme)` declared
When I run `specscore studio ask "who fronts acme.app" --format json`
Then the command exits 0, the JSON `template` is `who-fronts`, the `answer` names `acme`, and `citations` is a non-empty array whose entry names the `fronts` fact's evidence_pointer and adapter id

### AC: ask-unroutable-lists-templates

Scenario: an unroutable question declines honestly and lists templates
Given any indexed store
When I run `specscore studio ask "why does contactus exist"`
Then the command exits 1, prints a "no template matched" notice, and lists the routable templates (the same content as `specscore studio ask --list`)

### AC: ask-routed-but-empty

Scenario: a routed question with no matching data exits 3, not 0
Given an indexed store with no `fronts` fact for `unknown.example`
When I run `specscore studio ask "who fronts unknown.example"`
Then the command exits 3 with a message distinguishing routed-but-no-data from unroutable, and emits no citation-free answer

### AC: benchmark-file-has-50

Scenario: the committed benchmark holds exactly 50 well-formed instances
Given the committed `benchmark/questions.jsonl`
When I count its lines and parse each as JSON with fields `id`, `question`, `template`, `parameter`, `expectation`
Then there are exactly 50 instances, every `template` is one of the documented templates or empty (for `expected-unanswerable`), and the per-template counts match the `## Benchmark composition` table

### AC: benchmark-runner-scores-fixture

Scenario: the runner scores the hermetic fixture and rejects hallucinations
Given the committed `benchmark/testdata/fixture/` indexed and probed with stubbed seams into a fixture store
When I run `benchmark/run.sh --db <fixture-db>`
Then the runner prints an `answered-with-citations / 50` line, every `expected-unanswerable` instance is reported as correctly-declined, and the runner exits non-zero if any `expected-unanswerable` instance was answered

### AC: contradictions-without-index-errors

Scenario: contradiction detection before indexing is an actionable error
Given a workspace directory where `studio index` has never run
When I run `specscore studio contradictions`
Then the command exits 2 with a message naming the expected store path and suggesting `specscore studio index`

## Open Questions

- **Trigger-matching robustness of the router** — the exact keyword/regex patterns per template (word-order tolerance, synonym coverage) are a plan detail; the ACs pin the routing *contract* (routable → citations; unroutable → exit 1 + list; routed-but-empty → exit 3), not the specific patterns, so the plan can tune matching without changing behavior.
- **Fixture size vs template coverage** — how many repos the hermetic `benchmark/testdata/fixture/` needs to exercise all 13 answerable templates is a plan sizing detail; `exit-gate-fixture-and-sneat` fixes the property (every fixture-answerable instance answers), not the repo count.

## Autonomous Decisions

- **One feature, three verbs + benchmark** (orchestrator decision) — `contradictions`, `resolve`, and `ask` are the three consumers of the same store and share the router (`ask` routes through `resolve`, and `contradictions-for` is an `ask` template over `contradicts` facts); splitting them would duplicate the resolve/citation machinery and fracture the one benchmark that is the phase's single exit gate. The handout's §3 explicitly leaves this a specify-time call.
- **`contradicts` fact shape: subject/object = fact-ref triple keys, class `derived`, pointer = detector id, adapter `contradictions`** (recorded per the scope directive) — a contradiction is *computed from* facts, so `derived` is the honest class (not `verified-behavior`, which means executed-and-observed); the sides are pointer-free `<subject>|<predicate>|<object>` refs so the fact is stable across pointer churn, canonicalised (smaller ref first) so the store merge is idempotent and re-running never duplicates.
- **Evidence-class partition between the detectors** (reviewer-driven regrounding) — status-drift owns `declared`-vs-`verified-behavior` (its two branches: lifecycle-vs-verification and same-subject+predicate declared-vs-verified disagreement, e.g. declared `serves-status` `200` vs probed `down`); naming-conflict owns `declared`-vs-`declared`; `verified`-vs-`verified` is supersession and flagged by neither. An earlier draft joined a product `has-status` to domain liveness via `fronts` — dropped because **no adapter emits a product-subject `has-status` fact** (the specscore adapter's `has-status` subjects are spec refs only; the registries adapter's product entries carry no status), so the branch was regrounded on the declared-vs-verified `serves-status` pair the probe feature explicitly hands to this one.
- **Behavioral supersession is expressly not a same-predicate contradiction** — two `verified-behavior` `serves-status` facts with different objects are `cli/studio/probe`'s changed-object case (append-with-supersession), not a disagreement between authored sources; the naming-conflict detector requires both sides `declared`.
- **Suppression via a plain workspace-local ignore file** (alternative: attested-exception log) — Phase 2 owns the durable attestation log (design 10); Phase 1 ships the simple ignore file, with `--show-ignored` so suppression is never silent (`suppression-ignore-list`, `## Not Doing`).
- **`ask` is a deterministic template router, not an LLM** (handout §2 goal 5, §5 Phase-2 MCP) — keeps v0 offline, hermetic, and mechanically benchmarkable; the LLM `answer()` is Phase 2.
- **13 templates, chosen to cover the benchmark's answerable subset** — the set (who-fronts, what-repos-implement, status-of, aliases-of, member-of, is-it-live, ci-status-of, what-verifies, contradictions-for, freshness-of, what-uses, version-pins, aliases-resolve) maps 1:1 to predicates the Phase-0/probe/rehearse adapters already emit (verified against the adapter code — `what-uses`/`version-pins` both read the manifests adapter's `consumes` facts; no adapter emits `depends-on`), so every template's query is over facts that actually exist. `status-of` targets spec entities only (features and ideas) — product-status questions have no producing fact and would be dishonest benchmark instances.
- **Benchmark is 50 instances, 41 answerable / 9 expected-unanswerable, one committed file + one runner, two gate assertions** — the file stays honest by including genuinely out-of-scope questions marked `expected-unanswerable`; CI asserts the hermetic fixture floor continuously while the ≥40/50 Sneat run is the human-runnable phase gate (network + Sneat checkout are not CI-available). This is the reviewer-acceptable honest formulation of the handout's exit-gate DESIGN CALL.
- **Exit-code vocabulary taken from `pkg/exitcode`** (reviewer-recommended, adopted) — `resolve`: unknown name → 3 (`NotFound`), ambiguous name → 5 (`AmbiguousSlug`), usage / no store → 2 (`InvalidArgs`); `ask`: unroutable → 1 (no template — a router miss, not a store miss), routed-but-empty → 3 (`NotFound`). No new codes are invented.
- **Naming-conflict scoped to a fixed single-valued declared predicate set** (evidence-driven, post-approval revise-in-place) — the detector as first specified fired on ANY same-subject+predicate declared pair with differing objects/pointers, and the Sneat dogfood run proved that shape wrong: **5,758 naming-conflict items** (5,328 `has-ac` + 406 `contains` + 24 `implemented-by`) — 92% of the flood was every ordinary multi-AC feature "conflicting" with itself, against 7 genuine status-drift items. Predicates like `has-ac`, `contains`, `implemented-by`, `aliased-as`, `member-of`, and `consumes` are legitimately multi-valued: two objects are two values, not a disagreement. The detector now fires only on `{has-status, serves-status, fronts}` — the declared predicates that are single-valued per subject, where a second differing object IS a disagreement between authored sources. On the same dogfood store the scoped detector yields 7 status-drift + 0 naming-conflict items, all genuine (0% noise, well under the ≤20% exit-gate ceiling). The `what-verifies` benchmark row was removed in the same evidence pass: the dogfood store holds zero rehearse facts (no Sneat repo runs `rehearse run --report-out` yet), so its 3 instances were honest-unanswerable through no router fault; they are rebalanced to who-fronts/ci-status-of/contradictions-for and the template remains routable and unit-tested.

---
*This document follows the https://specscore.md/feature-specification*
