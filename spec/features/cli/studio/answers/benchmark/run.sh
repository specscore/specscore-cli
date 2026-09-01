#!/usr/bin/env bash
# Benchmark runner (cli/studio/answers#req:benchmark-runner, #req:exit-gate-fixture-and-sneat).
#
# Executes all 50 instances of questions.jsonl against a fact store, driving
# `studio ask` (and, for the contradicts-facts it queries, running
# `studio contradictions` into the store first), and scores
# answered-with-citations / 50.
#
# An instance counts as ANSWERED when its verb exits 0 AND emits at least one
# citation. An `expected-unanswerable` instance counts as CORRECTLY-DECLINED
# when its verb exits non-zero (routes to the unanswerable path). The runner
# reports both counts and a per-template breakdown, enforces the
# `## Benchmark composition` table against the file, and FAILS if any
# `expected-unanswerable` instance is answered anyway (a silent hallucination is
# a harder failure than a miss).
#
# Usage:
#   run.sh --db <path>            score against an existing store
#   run.sh --workspace <yaml>     score against <workspace-dir>/.specscore-studio/facts.db
#   run.sh --fixture              index+seed the committed hermetic fixture and
#                                 score it, asserting the CI floor (every
#                                 fixture-answerable instance answered; every
#                                 expected-unanswerable declined)
#   run.sh --gate <N>             require at least N answered-with-citations
#                                 (the Sneat exit gate is --gate 40)
#
# Env:
#   SPECSCORE   the CLI under test (default: `specscore` on PATH)
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
QUESTIONS="$HERE/questions.jsonl"
FIXTURE_DIR="$HERE/testdata/fixture"
SPECSCORE="${SPECSCORE:-specscore}"

DB=""
WORKSPACE=""
FIXTURE=0
GATE=""

while [ $# -gt 0 ]; do
  case "$1" in
    --db)        DB="$2"; shift 2 ;;
    --workspace) WORKSPACE="$2"; shift 2 ;;
    --fixture)   FIXTURE=1; shift ;;
    --gate)      GATE="$2"; shift 2 ;;
    -h|--help)   grep '^#' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "run.sh: unknown argument $1" >&2; exit 2 ;;
  esac
done

SS_ABS="$(command -v "$SPECSCORE")" \
  || { echo "run.sh: cannot resolve $SPECSCORE on PATH (set \$SPECSCORE)" >&2; exit 2; }

# --- resolve the target store ---------------------------------------------
SCRATCH=""
cleanup() { if [ -n "$SCRATCH" ]; then rm -rf "$SCRATCH"; fi; }
trap cleanup EXIT

if [ "$FIXTURE" -eq 1 ]; then
  # Build the hermetic fixture store in a scratch copy (index + probe-stubbed
  # seed), then require the CI floor.
  command -v sqlite3 >/dev/null 2>&1 || { echo "run.sh: --fixture needs sqlite3" >&2; exit 2; }
  SCRATCH="$(mktemp -d)"
  cp -r "$FIXTURE_DIR"/. "$SCRATCH/"
  DB="$SCRATCH/facts.db"
  bash "$FIXTURE_DIR/seed.sh" "$SS_ABS" "$SCRATCH" "$DB" >/dev/null
  [ -z "$GATE" ] && GATE=41   # the fixture answers all 41 answerable instances
elif [ -n "$DB" ]; then
  :
elif [ -n "$WORKSPACE" ]; then
  DB="$(cd "$(dirname "$WORKSPACE")" && pwd)/.specscore-studio/facts.db"
else
  echo "run.sh: one of --db, --workspace or --fixture is required" >&2
  exit 2
fi

[ -f "$DB" ] || { echo "run.sh: store not found at $DB (run studio index first)" >&2; exit 2; }

# --- run contradictions into the store (contradicts-for instances need it) --
"$SS_ABS" studio contradictions --db "$DB" >/dev/null 2>&1 || true

# --- score every instance --------------------------------------------------
# The Python scorer drives `studio ask` per instance, enforces the composition
# table, and prints the report; its exit status is the runner's verdict.
SPECSCORE_BIN="$SS_ABS" DB_PATH="$DB" GATE="${GATE:-}" \
python3 - "$QUESTIONS" <<'PY'
import json, os, subprocess, sys
from collections import Counter

questions_path = sys.argv[1]
ss = os.environ["SPECSCORE_BIN"]
db = os.environ["DB_PATH"]
gate = os.environ.get("GATE", "")

# The authoritative composition table (## Benchmark composition) the runner
# enforces against the file — a drift between file and table is a hard failure.
# what-verifies carries no benchmark instances (no rehearse evidence exists in
# the dogfood workspace yet); the template stays in ask, unit-tested.
COMPOSITION = {
    "who-fronts": 5, "what-repos-implement": 4, "status-of": 5, "aliases-of": 3,
    "member-of": 3, "is-it-live": 4, "ci-status-of": 4,
    "contradictions-for": 4, "freshness-of": 2, "what-uses": 3, "version-pins": 2,
    "aliases-resolve": 2,
}
ANSWERABLE_TOTAL = 41
UNANSWERABLE_TOTAL = 9

instances = []
for n, line in enumerate(open(questions_path), 1):
    line = line.strip()
    if not line:
        continue
    try:
        o = json.loads(line)
    except json.JSONDecodeError as e:
        print(f"run.sh: malformed questions.jsonl line {n}: {e}", file=sys.stderr)
        sys.exit(2)
    for field in ("id", "question", "template", "parameter", "expectation"):
        if field not in o:
            print(f"run.sh: line {n} missing field {field!r}", file=sys.stderr)
            sys.exit(2)
    instances.append(o)

# --- composition check (table <-> file) ---
if len(instances) != 50:
    print(f"run.sh: expected 50 instances, got {len(instances)}", file=sys.stderr)
    sys.exit(2)
answerable = [i for i in instances if i["expectation"] == "answerable"]
unanswerable = [i for i in instances if i["expectation"] == "expected-unanswerable"]
if len(answerable) != ANSWERABLE_TOTAL or len(unanswerable) != UNANSWERABLE_TOTAL:
    print(f"run.sh: split is {len(answerable)}/{len(unanswerable)}, "
          f"want {ANSWERABLE_TOTAL}/{UNANSWERABLE_TOTAL}", file=sys.stderr)
    sys.exit(2)
by_template = Counter(i["template"] for i in answerable)
comp_ok = True
for tmpl, want in COMPOSITION.items():
    got = by_template.get(tmpl, 0)
    if got != want:
        print(f"run.sh: composition mismatch — {tmpl}: file has {got}, table wants {want}",
              file=sys.stderr)
        comp_ok = False
extra = set(by_template) - set(COMPOSITION)
if extra:
    print(f"run.sh: composition mismatch — file has undocumented templates {sorted(extra)}",
          file=sys.stderr)
    comp_ok = False
if not comp_ok:
    sys.exit(2)

def ask(question):
    """Run `studio ask` for a question; return (exit_code, citation_count)."""
    p = subprocess.run(
        [ss, "studio", "ask", question, "--db", db, "--format", "json"],
        capture_output=True, text=True)
    ncites = 0
    if p.returncode == 0 and p.stdout.strip():
        try:
            ncites = len(json.loads(p.stdout).get("citations", []))
        except json.JSONDecodeError:
            ncites = 0
    return p.returncode, ncites

answered = 0
answered_by_template = Counter()
missed = []
correctly_declined = 0
hallucinated = []

for inst in instances:
    rc, ncites = ask(inst["question"])
    is_answered = (rc == 0 and ncites > 0)
    if inst["expectation"] == "answerable":
        if is_answered:
            answered += 1
            answered_by_template[inst["template"]] += 1
        else:
            missed.append((inst["id"], inst["template"], inst["question"], rc))
    else:  # expected-unanswerable
        if is_answered:
            hallucinated.append((inst["id"], inst["question"]))
        else:
            correctly_declined += 1

# --- report ---
print(f"answered-with-citations {answered} / 50")
print(f"  answerable answered:      {answered} / {ANSWERABLE_TOTAL}")
print(f"  honesty (declined):       {correctly_declined} / {UNANSWERABLE_TOTAL} expected-unanswerable")
print("  per-template:")
for tmpl in COMPOSITION:
    print(f"    {tmpl:22s} {answered_by_template.get(tmpl,0)} / {COMPOSITION[tmpl]}")
if missed:
    print("  missed answerable instances:")
    for iid, tmpl, q, rc in missed:
        print(f"    {iid} [{tmpl}] rc={rc}: {q}")

# --- verdict ---
if hallucinated:
    print("FAIL: expected-unanswerable instances that were answered (hallucination):",
          file=sys.stderr)
    for iid, q in hallucinated:
        print(f"    {iid}: {q}", file=sys.stderr)
    sys.exit(1)

if gate:
    need = int(gate)
    if answered < need:
        print(f"FAIL: {answered} answered-with-citations < gate {need}", file=sys.stderr)
        sys.exit(1)
    print(f"PASS: {answered} answered-with-citations >= gate {need}")

sys.exit(0)
PY
