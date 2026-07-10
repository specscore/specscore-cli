---
format: https://specscore.md/feature-specification
status: Approved
---

# Feature: studio probe — live probes + freshness as data

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/probe?op=explore) | [Edit](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/probe?op=edit) | [Ask question](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/probe?op=ask) | [Request change](https://specscore.studio/app/github.com/specscore/specscore-cli/spec/features/cli/studio/probe?op=request-change) |
**Status:** Approved
**Source Ideas:** —
**Supersedes:** —
**Grade:** A

## Summary

`specscore studio probe` runs live checks against the ecosystem and writes `verified-behavior` facts into the same fact store `studio index` builds — turning the registries adapter's *declared* `serves-status` (an HTTP status copied verbatim out of `domains.json`) into behavioral truth by actually requesting the domain, and adding CI-run state read from `gh api`. Probes are opt-in and manual/CI-cron (no daemon); `studio index` stays offline-pure. Each probe fact carries an honest `observed_at` (the probe's execution time) and an evidence pointer that reproduces the observation (the URL requested / the `gh api` path). To make freshness answerable, the fact model gains `verified_at` (distinct from `observed_at` — re-probing an unchanged fact refreshes `verified_at` without moving `observed_at`), `studio facts` gains a `--stale <duration>` filter and renders fact age, and per-evidence-class re-verification cadences are documented (design 03 §Freshness). This is the first half of Studio Phase 1 (`docs/continuations/studio-phase-1-evidence-freshness.md` §2 goals 1–2); the confidence ladder's top rung (`verified-behavior`) landed yesterday via `cli/rehearse/evidence`.

## Problem

Phase 0's registries adapter emits `serves-status` and CI/deploy expectations as **declared** facts — human-authored registry entries that lag reality. The Sneat review's poster child: an investor-CTA domain sat at a "live" `http_status` in `domains.json` while the domain was actually dead; a `curl` beat every document. Studio's evidence ladder names `verified-behavior` as the top rung, but Phase 0 has no producer for the *live-check* half of it (rehearse covers test/CI-pass execution; nothing curls a domain or reads a repo's latest CI conclusion). Worse, without a `verified_at` distinct from `observed_at`, the store cannot answer "is this still true?" — the whole point of the freshness dots in the UX design (§freshness: green < 24h, grey < 30d, amber stale). Re-running a probe that finds no change would either falsely age the fact or falsely refresh its observation time. Freshness has to be *data*, not a render-time guess.

## Behavior

### The probe verb

#### REQ: probe-verb

`specscore studio probe` is a new subcommand of the `studio` group, a sibling of `index` and `facts`. It reads the same `studio.yaml` workspace (`--workspace`, default `./studio.yaml`) and writes into the same fact store (`--db`, default `<workspace-dir>/.specscore-studio/facts.db`) that `studio index` built. Unlike `index`, `probe` performs live network I/O and does **not** rebuild the store from scratch: it reads the probe *targets* from the facts already in the store and from the workspace's repos (every `serves-status` subject in the store is a domain to check; every resolved workspace repo with a GitHub `origin` remote is a CI target — `ci-state`), runs the selected probe kinds, and merges the resulting `verified-behavior` facts back in (`probe-merge`). Probe facts carry per-kind adapter identities — id `probe-domain` or `probe-ci`, version the specscore CLI version — so merging one kind's facts never touches the other kind's. Running `probe` before any `index` exits 2 with the missing-store message (reusing `cli/studio/index#req:facts-query`'s guidance to run `studio index`). `--kind <domain|ci|all>` selects which probe families run (default `all`); `--format <human|json>` controls the run summary: the JSON summary is an object with `kinds` (the kinds that ran), `facts_written`, `verified_refreshed`, and `warnings` (a list of strings); the human summary is free-form prose over the same data. `studio index` is unchanged and never performs network I/O.

#### REQ: probe-opt-in

Probes never run as a side effect of `index`. There is no daemon, scheduler, or watch mode: `studio probe` runs once per invocation and exits. Re-verification cadence is the operator's or CI cron's responsibility; the documented per-class cadences (`freshness-cadences`) are guidance for how often to schedule the verb, not an in-process timer.

### Probe kinds

#### REQ: domain-liveness

The **domain** probe kind issues an HTTP(S) `GET` (following redirects, with a bounded timeout) to each domain that carries a `serves-status` fact in the store — i.e. the domains the registries adapter declared. For each, it emits a `(<domain>, serves-status, <http-status-code>)` fact with `evidence_class: verified-behavior`, `evidence_pointer` set to the URL actually requested (e.g. `https://example.app/`), and `observed_at` set to the probe's execution time (UTC, RFC 3339). This behaviorally confirms — or contradicts — the same-predicate `declared` fact from `domains.json`: both facts coexist in the store (append, not overwrite), differing by `evidence_class` and pointer, which is exactly the raw material the sibling contradiction feature consumes. Scheme policy: the probe requests `https://<domain>/` first; on a transport-level failure (no HTTP response) it retries `http://<domain>/`, and the evidence pointer records whichever URL produced the response. Only when both schemes yield no response is the domain `down` (`network-failure-is-data`).

#### REQ: network-failure-is-data

A probe that fails to get an HTTP response over either scheme (DNS failure, connection refused, TLS error, timeout — per `domain-liveness`'s https-then-http policy) is itself evidence — it proves *non-liveness*. The domain probe records this as a `(<domain>, serves-status, down)` fact (`down` a reserved non-numeric object distinguishing "no response" from any numeric HTTP status, including 5xx which *is* a response), `evidence_class: verified-behavior`, with the failure reason in the evidence pointer's detail and `observed_at` at execution time. Non-liveness is a fact, never fact absence — a missing fact means "not probed", a `down` fact means "probed, no service". The verb's exit code is unaffected by individual `down` results (a dead domain is a successful observation); only an internal error (unreadable store, unusable workspace) is a non-zero exit.

#### REQ: ci-state

The **ci** probe kind reads each repository's latest default-branch CI run via `gh api` (the GitHub CLI, house exec-seam pattern). Targets are resolved from the workspace's repos, not from store facts (the store's repo IDs are `fact.RepoSlugger` basename slugs with no remote coordinates): for each resolved workspace repo the probe runs `git remote get-url origin` (through the exec test-seam) and parses the GitHub `org/name` from the remote URL; a repo with no `origin` remote or a non-GitHub remote is skipped, and each skip is visible as a per-repo notice in the run summary. For each GitHub-remoted repo the probe queries the latest default-branch Actions run via `gh api` and emits a `(<repo-slug>, ci-status, success|failure|<conclusion>)` fact — the **subject is the same repo slug the store already uses**, so consumers (including the sibling contradiction feature) can join CI state against the repo's other facts — with `evidence_class: verified-behavior`, `evidence_pointer` set to the `gh api` path queried (e.g. `repos/<org>/<name>/actions/runs?branch=<default>&per_page=1`), and `observed_at` at execution time. A repo with no runs, or a `gh api` call that fails (unauthenticated, rate-limited, repo not found) emits no `ci-status` fact and records a per-repo warning in the summary — an unreachable CI is "not probed", not "down" (CI liveness is out of this probe's claim). `gh` absent from `PATH` skips the `ci` kind entirely with one summary warning.

#### REQ: deploy-state-folded

Deploy-state verification is **folded into the domain and ci kinds**, not a separate probe: a fronted domain answering `2xx` is the cheapest available proof a deployment serves, and a green default-branch CI run is the cheapest proof a deploy pipeline succeeded. No `wrangler`/Cloudflare-API probe ships this phase — it needs per-account credentials and a deploy-target registry Phase 0 does not model. This is recorded as an autonomous decision, not a silent omission.

### Freshness as data

#### REQ: verified-at-field

The stored fact gains a `verified_at` field, distinct from `observed_at`. `observed_at` is when the underlying behavior was observed (the moment a probe request completed, or a rehearse run started); `verified_at` is the timestamp of the fact's most recent successful re-verification. On first write the two are equal. When a probe re-runs and finds a fact whose `(subject, predicate, object, evidence_class)` is unchanged, it **refreshes `verified_at` to the new run time while preserving the original `observed_at`** — the behavior was last confirmed now, but it was first observed then. When the object changes (a domain that was `200` now `down`), a new fact is written with both timestamps at the new run time (it is a fresh observation, not a re-verification of the old one). Facts written by `index` adapters carry `verified_at == observed_at` at index time; `declared`/`derived` facts re-verify on the next `index` run.

#### REQ: probe-merge

`studio probe` merges its facts into the existing store rather than rebuilding it: `index` facts and prior probe facts survive a probe run. Merge is keyed on `(subject, predicate, object, evidence_class, adapter_id)` — with the per-kind adapter ids `probe-domain`/`probe-ci` (`probe-verb`) in the key, one kind's merge can never refresh or clobber the other kind's facts, nor any index adapter's. A matching existing fact has its `verified_at` refreshed (`verified-at-field`); a fact with a new object is inserted alongside its siblings (append-only per the design's supersession model). A `probe` run never deletes `index`-written facts — only a subsequent `studio index` rebuild does that, and a probe re-run after an index rebuild re-establishes the `verified-behavior` facts. The merge is atomic (temp-file swap, mirroring `cli/studio/index#req:rebuild-only`), so a failed probe leaves the prior store intact.

#### REQ: stale-filter

`specscore studio facts` gains a `--stale <duration>` flag (Go duration syntax, e.g. `24h`, `720h`) selecting only facts whose `verified_at` is older than `now − duration`. It composes with the existing `--subject/--predicate/--object/--class/--adapter` filters (AND semantics) and with `--count`. Every fact in a current store carries `verified_at`: the store is a rebuild-only disposable cache (`cli/studio/index#req:rebuild-only`), so rebuilding IS the schema migration — no legacy-store compatibility path exists or is needed. `--stale` with a malformed duration exits 2 with an actionable message.

#### REQ: age-rendering

`studio facts` human (table) output renders each fact's freshness as a human age derived from `verified_at` (e.g. `3h`, `12d`, `stale`), in a new `VERIFIED` column, so an operator sees at a glance which facts are fresh. JSON output includes both `observed_at` and `verified_at` verbatim (RFC 3339) — no derived age, so downstream consumers compute their own freshness bands. The table's age uses the same thresholds the UX design names for the freshness dots (fresh < 24h, aging < 30d, else stale), rendered as text since the CLI has no color primitive.

#### REQ: freshness-cadences

The per-evidence-class re-verification cadences from design 03 §Freshness are documented in the verb's help text and this spec (`## Freshness cadences`): `verified-behavior` — hours to a day (the probe verb, scheduled by CI cron); `derived` — on push / on `index` re-run; `declared` — on repo change / on `index` re-run; `claimed` — never (flagged as decaying); `attested` — quarterly (Phase 2). These cadences are guidance for scheduling, not enforced timers this phase.

## Freshness cadences

| Evidence class | Re-verification cadence | Producer this phase |
|---|---|---|
| `verified-behavior` | hours–1 day (CI cron) | `studio probe` (this feature), `rehearse` adapter |
| `derived` | on push / `index` re-run | `codegraph`, `manifests` adapters |
| `declared` | on repo change / `index` re-run | `specscore`, `registries` adapters |
| `claimed` | never — rendered as decaying | (no producer this phase) |
| `attested` | quarterly nag | (Phase 2) |

`verified_at` drives the age column and the `--stale` filter; the KPI the design names is *% of rendered facts with fresh/aging freshness* — Studio's own SLA.

## Architecture & Components

Go packages inside specscore-cli (no new binary):

- `internal/studio/probe` — the probe engine: domain-target extraction from the store (subjects carrying `serves-status`), CI-target resolution from the workspace repos (`git remote get-url origin` → GitHub `org/name`, with the same `fact.RepoSlugger` slugs the index pipeline mints as fact subjects), the HTTP domain prober, and the `gh api` CI prober. Probers are behind test seams — a package-level `httpGetFn` for the domain kind and an `execCommandFn` wrapping `git`/`gh` for the ci kind (house exec-seam pattern, mirroring `internal/rehearse/runner`'s `ExecCommandFn`/`execNewCommandFn`). Facts are stamped with per-kind adapter ids `probe-domain`/`probe-ci` and the CLI version. Pure of CLI wiring; unit-testable with stubbed HTTP/exec.
- `internal/studio/fact` — `Fact` gains `VerifiedAt string` (`json:"verified_at"`) alongside `ObservedAt`.
- `internal/studio/store` — schema gains a `verified_at` column; a new `Merge(path, facts)` entry point performs the `verified_at`-refresh-or-insert (`probe-merge`) inside the atomic temp-file swap already used by `Rebuild`; `Query`/`Filter` gain the `--stale` cutoff (a `StaleBefore time.Time`). `Rebuild` stamps `verified_at = observed_at` for index facts.
- `internal/studio/adapters` — `Run` stamps `verified_at` equal to each fact's `observed_at` (both honest at index time), leaving the probe engine as the only writer that advances `verified_at` past `observed_at`.
- `internal/cli/studio.go` — the new `studio probe` subcommand (flags `--kind`, `--format`; shares `--workspace`/`--db`), the `--stale` flag and `VERIFIED` column on `studio facts`, and the exec/http seam wiring for probe.

Data flow: `studio index` (offline) → store of `declared`/`derived` facts → `studio probe` (live HTTP + `gh api`) reads domain targets from that store and CI targets from the workspace repos' git remotes → `verified-behavior` facts merged back with honest `observed_at`/`verified_at` → `studio facts --stale`/age column surfaces freshness. `probe` reads the store to find targets and writes the store; it never rebuilds it.

## Error Handling & Failure Modes

- No store yet (`probe` before `index`) → exit 2, message names the store path and suggests `studio index` (reuses `facts-query` guidance).
- A single domain request failing → a `serves-status=down` fact (`network-failure-is-data`), not an error; the run continues and exits 0.
- A workspace repo with no `origin` remote or a non-GitHub remote → per-repo skip notice in the run summary, no `ci-status` fact; the run continues (`ci-state`).
- A `gh api` call failing for one repo (auth, rate limit, 404) → per-repo warning, no `ci-status` fact for that repo; the run continues.
- `gh` not on `PATH` → the `ci` kind is skipped with one summary warning; the `domain` kind still runs (`--kind all`).
- Malformed `--stale` duration → exit 2 with an actionable message (`stale-filter`).
- Store write (merge) failure → exit 1; the prior store is left intact via the atomic temp-file swap (`probe-merge`).
- A workspace that is missing/unparsable/empty-resolution → exit 2, reusing `cli/studio/index#req:workspace-errors`.

## Testing Strategy

Unit tests per package with the network and exec seams stubbed: the domain prober against a stub `httpGetFn` returning 200 / 5xx / transport error (proving the https→http fallback and the `down` mapping); the CI prober against a stub `execCommandFn` covering `git remote get-url origin` outputs (GitHub remote / non-GitHub remote / no remote) and `gh api` outcomes (JSON body / non-zero exit / absent `gh`); the store `Merge` proving `verified_at` refresh-without-`observed_at`-drift and new-object insertion; the `--stale` cutoff and age rendering; CLI flag wiring. No test performs real network I/O or invokes `gh`. E2e: Rehearse scenarios per testable AC under `_tests/` (scaffolded, `**Status:** pending`) driving `studio probe`/`studio facts` over a fixture store. Coverage gate stays 100% (`scripts/coverage-gate.sh`).

## Not Doing / Out of Scope

- **Daemon / scheduler / watch mode** — probes are opt-in per invocation; scheduling is the operator's / CI cron's job (`probe-opt-in`).
- **Web UI / freshness dots as color** — this feature emits the `verified_at` data the dots render from; the renderer is Phase 1.5–2 (design 01/09). The CLI shows text age only.
- **`attested-exception` / attestation log** — Phase 2 (needs the durable attestation store per design 10).
- **Contradiction detection** — the sibling Phase-1 feature (`cli/studio/contradictions`) consumes the coexisting declared-vs-verified `serves-status` facts this feature produces; detection is not in scope here.
- **Deploy-target (Cloudflare/wrangler) probing** — folded into domain+ci kinds this phase (`deploy-state-folded`); a dedicated deploy prober needs credentials and a deploy registry Phase 0 does not model.
- **LLM anything** — probes are deterministic HTTP/exec checks.
- **Run history / probe-result retention beyond the latest** — the store holds the latest `verified_at`; time-series history is a later Studio-hosted concern.

## Rehearse Integration

Every AC has a CLI-observable surface (probe run summaries, `facts` output, exit codes) or a fixture-store surface; Rehearse stubs are scaffolded under `_tests/` (one per AC) with `**Status:** pending`. Network- and `gh`-dependent scenarios use a fixture store and the stubbed seams so the corpus stays hermetic (no real network in CI).

## Acceptance Criteria

### AC: probe-writes-verified-serves-status

Scenario: a live domain probe records a verified-behavior serves-status
Given an indexed store containing a `declared` `serves-status` fact for domain `example.app`, and a domain probe stubbed to return HTTP 200 for `https://example.app/`
When I run `specscore studio probe --kind domain` and then `specscore studio facts --predicate serves-status --class verified-behavior --format json`
Then the command exits 0 and the JSON contains a fact with subject `example.app`, predicate `serves-status`, object `200`, evidence_class `verified-behavior`, adapter id `probe-domain`, an evidence_pointer of `https://example.app/`, and a non-empty `observed_at`

### AC: declared-and-verified-coexist

Scenario: the probe fact does not overwrite the declared fact
Given the same store after a domain probe
When I run `specscore studio facts --predicate serves-status --subject example.app --format json`
Then the JSON contains both a fact with evidence_class `declared` (pointer `domains.json`) and a fact with evidence_class `verified-behavior` (pointer the probed URL) for the same subject and predicate

### AC: network-failure-records-down

Scenario: an unreachable domain proves non-liveness
Given an indexed store with a `serves-status` fact for domain `dead.example`, and a domain probe stubbed to return a connection error for `https://dead.example/`
When I run `specscore studio probe --kind domain` and then `specscore studio facts --subject dead.example --predicate serves-status --class verified-behavior --format json`
Then the command exits 0 and the JSON contains a fact with object `down` and evidence_class `verified-behavior`

### AC: ci-state-fact

Scenario: the ci probe records a repo's latest run conclusion
Given an indexed workspace repo `widget` whose `origin` remote is `https://github.com/acme/widget.git`, with the exec seam stubbed so `git remote get-url origin` returns that URL and `gh api` returns a latest default-branch run with conclusion `success`
When I run `specscore studio probe --kind ci` and then `specscore studio facts --predicate ci-status --format json`
Then the command exits 0 and the JSON contains a fact with subject `widget` (the store's repo slug), object `success`, evidence_class `verified-behavior`, adapter id `probe-ci`, and an evidence_pointer naming the queried `gh api` path

### AC: non-github-repo-skipped

Scenario: a repo without a GitHub remote is visibly skipped
Given an indexed workspace repo with the exec seam stubbed so `git remote get-url origin` fails (no remote configured)
When I run `specscore studio probe --kind ci`
Then the command exits 0, the run summary contains a per-repo notice that the repo was skipped for having no GitHub remote, and no `ci-status` fact is written for it

### AC: gh-absent-skips-ci

Scenario: missing gh CLI skips the ci kind without failing
Given an indexed store and no `gh` binary on `PATH`
When I run `specscore studio probe --kind ci`
Then the command exits 0 and the run summary reports one warning that the `ci` kind was skipped because `gh` was not found, and no `ci-status` facts are written

### AC: probe-preserves-index-facts

Scenario: probing merges rather than rebuilds
Given an indexed store with declared and derived facts from `studio index`
When I run `specscore studio probe` and then query facts by any index adapter
Then the command exits 0 and the pre-existing `declared` and `derived` facts are still present in the store alongside the new `verified-behavior` facts

### AC: reprobe-refreshes-verified-at

Scenario: re-verifying an unchanged fact advances verified_at but not observed_at
Given a store already probed at time T1 with a `serves-status` `200` verified-behavior fact for `example.app` whose `observed_at` and `verified_at` both equal T1
When the domain probe is stubbed to again return 200 at a later time T2 and I run `specscore studio probe --kind domain` and then `specscore studio facts --subject example.app --class verified-behavior --format json`
Then the fact's `observed_at` is still T1 and its `verified_at` is T2

### AC: changed-object-new-observation

Scenario: a changed result is a fresh observation, not a re-verification
Given the same store whose prior verified fact was `serves-status` `200` at T1
When the domain probe is stubbed to return a connection error at T2 and I run `specscore studio probe --kind domain` and then `specscore studio facts --subject example.app --class verified-behavior --format json`
Then a `serves-status` `down` fact exists whose `observed_at` and `verified_at` both equal T2

### AC: stale-filter-selects-old-facts

Scenario: --stale selects only facts older than the duration
Given a store containing one verified-behavior fact with `verified_at` 48 hours ago and one with `verified_at` 1 hour ago
When I run `specscore studio facts --class verified-behavior --stale 24h --count`
Then the count is 1 (only the 48-hour-old fact)

### AC: stale-filter-malformed-duration

Scenario: a bad --stale duration is a usage error
Given any indexed store
When I run `specscore studio facts --stale notaduration`
Then the command exits 2 with a message naming the invalid duration

### AC: age-column-rendered

Scenario: the facts table shows a freshness age
Given a store containing a verified-behavior fact whose `verified_at` is 3 hours ago
When I run `specscore studio facts --class verified-behavior`
Then the table includes a `VERIFIED` column and the row shows a human age (e.g. `3h`) for that fact

### AC: probe-without-index-errors

Scenario: probing before indexing is an actionable error
Given a workspace directory where `studio index` has never run
When I run `specscore studio probe`
Then the command exits 2 with a message naming the expected store path and suggesting `specscore studio index`

### AC: cadences-in-help

Scenario: the per-class cadences are surfaced in the verb's help
Given the specscore CLI
When I run `specscore studio probe --help`
Then the help text names a re-verification cadence for each of `verified-behavior`, `derived`, `declared`, `claimed`, and `attested`

## Open Questions

- **Concurrency / rate-limiting of the domain prober** — bounded parallelism and per-host politeness are implementation details for the plan; the ACs only require correct per-domain facts, so no behavior is pinned here.
- **Default HTTP timeout value** — a sensible bounded default (a few seconds) is a plan detail; `network-failure-is-data` covers the timeout→`down` mapping regardless of the exact value.

## Autonomous Decisions

- **Separate `studio probe` verb, not `studio index --probe`** (orchestrator decision, recorded here) — keeps `index` offline-pure and fast; `probe` does the live I/O and merges into the same store. `index` stays deterministic and network-free.
- **Deploy-state folded into domain+ci kinds** (alternative: a third `deploy` kind) — a 2xx fronted domain and a green default-branch CI run are the cheapest available deploy proofs; a credentialed Cloudflare/wrangler prober is parked (`deploy-state-folded`, `## Not Doing`) because Phase 0 models no deploy-target registry.
- **CI targets resolve from git remotes, not store facts** (reviewer-recommended, orchestrator-approved) — the store's repo IDs are basename slugs with no remote coordinates (`fact.RepoSlugger`); the ci probe runs `git remote get-url origin` per workspace repo through the exec seam, parses the GitHub `org/name`, and emits facts whose subject is the store's existing repo slug so the contradiction sibling can join (`ci-state`). Repos without a GitHub remote are skipped visibly.
- **Scheme policy: https first, http fallback** (alternative: https-only) — an http-only service is still a live service; liveness is the claim, not TLS hygiene. The evidence pointer records the URL that actually answered; `down` means neither scheme responded (`domain-liveness`).
- **Per-kind adapter ids `probe-domain`/`probe-ci`, version = CLI version** (alternative: one `probe` id) — the adapter id is part of the merge key, so per-kind ids guarantee one kind's merge never refreshes or clobbers the other's facts (`probe-verb`, `probe-merge`).
- **Network failure is a `serves-status=down` fact, not fact absence** — a `down` fact means "probed, no service"; a missing fact means "not probed". This keeps non-liveness first-class and feeds the contradiction sibling (`network-failure-is-data`).
- **`verified_at` refresh preserves `observed_at`** (alternative: bump `observed_at` on every probe) — staleness must be honest: re-confirming an unchanged fact means it was *last verified* now but *first observed* then; a changed object is a new observation with both stamps fresh (`verified-at-field`).
- **`probe` merges; only `index` rebuilds** — probe facts and index facts share one store; probe never deletes index facts, an index rebuild drops probe facts (which the next probe re-establishes). Merge is atomic like the index rebuild (`probe-merge`).
- **Contradiction *detection* is the sibling feature's job** — this feature produces the coexisting declared-vs-verified facts; it does not flag them (`## Not Doing`).

---
*This document follows the https://specscore.md/feature-specification*
