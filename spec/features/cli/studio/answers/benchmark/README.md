# studio answers — benchmark

The Phase-1 exit gate for [`cli/studio/answers`](../README.md): 50 concrete
question instances (`questions.jsonl`), one runner (`run.sh`), and a hermetic
fixture workspace (`fixture/`). The runner scores **answered-with-citations /
50** and enforces the `## Benchmark composition` table against the file.

- `questions.jsonl` — exactly 50 instances (`{id, question, template, parameter,
  expectation}`); **41 answerable / 9 expected-unanswerable**. The 9 are
  genuinely out-of-Phase-1 shapes (why-exists, gotchas, deploy-method,
  commercial) marked `expected-unanswerable` so the file honestly represents the
  full question surface.
- `run.sh` — drives `studio ask` per instance (running `studio contradictions`
  into the store first, since `contradictions-for` instances query `contradicts`
  facts). An instance is **answered** when `ask` exits 0 with ≥1 citation; an
  `expected-unanswerable` instance is **correctly-declined** when `ask` exits
  non-zero. The runner fails if any `expected-unanswerable` instance is answered
  (a hallucination is a harder failure than a miss).
- `fixture/` — the hermetic CI workspace: tiny repos + registries + rehearse
  reports, a miniature of the Sneat ecosystem using honest ids (sizeus,
  gameboard, contactus, sneat-libs, sneat-go-core, …). `seed.sh` indexes it and
  seeds the probe-only `verified-behavior` facts (live/dead `serves-status`,
  per-repo `ci-status`) with `sqlite3` — no network, no `gh`.

## CI gate (hermetic, continuous)

```
SPECSCORE=/path/to/specscore ./run.sh --fixture
```

Indexes + seeds the fixture into a scratch store, runs the benchmark, and
asserts the floor: every answerable instance answered (41/41), every
expected-unanswerable declined (9/9). Hermetic — this is what CI runs. Also
exercised by the `benchmark-runner-scores-fixture` Rehearse scenario.

## Sneat dogfood gate (manual, the phase gate)

The reviewer-runnable ≥40/50 gate over the real Sneat workspace
(`~/projects/sneat-co/*`). Not a CI job — the Sneat checkout and live network
are not CI-available.

```bash
# 1. Point a workspace at the Sneat repos (or reuse an existing studio.yaml).
cat > /tmp/sneat-studio.yaml <<'YAML'
name: sneat
repos:
  - ~/projects/sneat-co/gameboard
  - ~/projects/sneat-co/contactus
  - ~/projects/sneat-co/sneat-libs
  - ~/projects/sneat-co/sneat-go-backend
  # …plus the ops repo(s) carrying domains.json / ecosystem*.yaml
YAML

# 2. Index (declared + derived + rehearse facts) and probe (live serves-status,
#    ci-status via gh) into the store.
specscore studio index   --workspace /tmp/sneat-studio.yaml
specscore studio probe    --workspace /tmp/sneat-studio.yaml   # network + gh

# 3. Score the benchmark against the Sneat store, requiring the ≥40/50 gate.
SPECSCORE=$(command -v specscore) \
  ./run.sh --workspace /tmp/sneat-studio.yaml --gate 40
```

The runner runs `studio contradictions` into the Sneat store itself, so the
`contradictions-for` instances resolve. The parameters in `questions.jsonl` are
real Sneat-ecosystem ids, so a Sneat index answers them; the file's honesty
ceiling is 41, so the ≥40 gate is clearable while 9 questions remain genuinely
out of scope.

Alongside the score, spot-check `specscore studio contradictions --workspace
/tmp/sneat-studio.yaml` for the known real drift class without a false-positive
flood (≤20% noise), per the feature's `## Exit gate`.
