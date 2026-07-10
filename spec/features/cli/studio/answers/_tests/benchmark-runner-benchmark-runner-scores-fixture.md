---
format: https://specscore.md/scenario-specification
---

# Rehearse: benchmark-runner-scores-fixture

**Status:** pass-capable
**Verifies:** cli/studio/answers#ac:benchmark-runner-scores-fixture (REQ: benchmark-runner)

Scenario source: [../README.md](../README.md) → `### AC: benchmark-runner-scores-fixture`.

Given the committed `benchmark/fixture/` indexed and probed with stubbed seams into a fixture store, when I run `benchmark/run.sh --db <fixture-db>`, then the runner prints an `answered-with-citations / 50` line, every `expected-unanswerable` instance is reported as correctly-declined, and the runner exits non-zero if any `expected-unanswerable` instance was answered.

Seam: `run.sh --fixture` indexes the committed fixture and seeds the probe-only
verified-behavior facts with `sqlite3` (its own hermetic seed) — no network. The
hallucination-rejection half is proven by re-running the runner against a mutated
questions file in which an `expected-unanswerable` instance is rewritten to an
answerable question, and asserting a non-zero exit. The committed fixture and
benchmark are located via `$REPO_ROOT` (CI passes it) or the parent process cwd.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:benchmark-runner-scores-fixture
# Requires: specscore on PATH (override with $SPECSCORE), python3, sqlite3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }
command -v sqlite3 >/dev/null 2>&1 || { echo "FAIL: sqlite3 not on PATH"; exit 1; }

repo="${REPO_ROOT:-${SPECSCORE_CLI_REPO:-}}"
if [ -z "$repo" ]; then
  if [ -e "/proc/$PPID/cwd" ]; then
    repo="$(readlink "/proc/$PPID/cwd")"
  else
    repo="$(pwd)"
  fi
fi
bench="$repo/spec/features/cli/studio/answers/benchmark"
[ -f "$bench/run.sh" ] \
  || { echo "FAIL: cannot locate benchmark/run.sh (looked in '$bench'); set \$REPO_ROOT"; exit 1; }

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

# 1. The runner scores the hermetic fixture and prints the report line + floor.
out="$(SPECSCORE="$SPECSCORE_ABS" bash "$bench/run.sh" --fixture 2>&1)"
rc=$?
echo "$out"
[ "$rc" -eq 0 ] || { echo "FAIL: runner exit $rc on the fixture floor"; exit 1; }
case "$out" in
  *"answered-with-citations "*"/ 50"*) ;;
  *) echo "FAIL: no 'answered-with-citations N / 50' line"; exit 1 ;;
esac
case "$out" in
  *"honesty (declined):       9 / 9"*) ;;
  *) echo "FAIL: not all 9 expected-unanswerable reported as declined"; exit 1 ;;
esac

# 2. Hallucination rejection: rewrite an expected-unanswerable instance into an
#    answerable question and assert the runner exits non-zero. Build a scratch
#    benchmark dir that reuses the committed fixture but a mutated questions file.
scratch="$workdir/bench"
mkdir -p "$scratch"
cp -r "$bench/fixture" "$scratch/fixture"
cp "$bench/run.sh" "$scratch/run.sh"
# Rewrite q42 (why-does-contactus-exist, expected-unanswerable) into a routable,
# answerable question, keeping the expectation expected-unanswerable → the runner
# must flag it as a hallucination.
python3 - "$bench/questions.jsonl" "$scratch/questions.jsonl" <<'PY'
import json, sys
src, dst = sys.argv[1], sys.argv[2]
out = []
for l in open(src):
    l = l.strip()
    if not l:
        continue
    o = json.loads(l)
    if o["id"] == "q42":
        o["question"] = "aliases of anymeter"  # routes + answers, but still marked unanswerable
    out.append(json.dumps(o))
open(dst, "w").write("\n".join(out) + "\n")
PY

set +e
mut_out="$(SPECSCORE="$SPECSCORE_ABS" bash "$scratch/run.sh" --fixture 2>&1)"
mut_rc=$?
set -e
[ "$mut_rc" -ne 0 ] \
  || { echo "FAIL: runner did not reject a hallucinated expected-unanswerable; out: $mut_out"; exit 1; }
case "$mut_out" in
  *hallucination*|*q42*) ;;
  *) echo "FAIL: runner did not name the hallucinated instance: $mut_out"; exit 1 ;;
esac

echo "PASS: benchmark-runner-scores-fixture"
```

---
*This document follows the https://specscore.md/scenario-specification*
