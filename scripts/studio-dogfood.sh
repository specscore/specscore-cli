#!/usr/bin/env bash
# Studio ecosystem dogfood: index -> probe -> contradictions -> benchmark over a
# real multi-repo workspace, emitting a dated markdown report and a regression
# exit code. This is the "docs-vs-reality runs continuously" loop from the Studio
# design (research/studio-design-2026-07/03), meant for a weekly schedule.
#
# It needs the actual repo checkouts (the probe hits live HTTP + `gh`, the
# registries come from an ops repo), so it runs where the workspace lives — not
# in stock CI. Everything is parameterised via env vars:
#
#   WORKSPACE_ROOT   dir whose immediate git subdirs form the workspace
#                    (default: ~/projects/sneat-co)
#   OPS_REPO         a repo carrying registries (ecosystem*.yaml / domains.json)
#                    that must be present for fronts/aliased-as/serves-status
#                    (default: $WORKSPACE_ROOT/sneat-ops; cloned if missing)
#   OPS_CLONE_URL    where to clone OPS_REPO from if absent
#                    (default: https://github.com/sneat-co/sneat-ops)
#   REPORT_DIR       where the markdown report is written
#                    (default: $WORKSPACE_ROOT/.studio-dogfood)
#   GATE             minimum benchmark answered-with-citations (default: 40)
#   CONTRADICTION_MAX total contradictions above which we flag a noise spike
#                    (default: 30 — the scoped detectors sit near 7 on Sneat)
#   SPECSCORE        prebuilt CLI to use (default: build from this checkout)
#   SKIP_PROBE       set to 1 to skip the live-probe phase (offline dry run)
#
# Exit 0 = healthy; exit 1 = regression (benchmark < GATE, or a hard failure);
# a contradiction spike is reported and flagged but does not by itself fail.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKSPACE_ROOT="${WORKSPACE_ROOT:-$HOME/projects/sneat-co}"
OPS_REPO="${OPS_REPO:-$WORKSPACE_ROOT/sneat-ops}"
OPS_CLONE_URL="${OPS_CLONE_URL:-https://github.com/sneat-co/sneat-ops}"
REPORT_DIR="${REPORT_DIR:-$WORKSPACE_ROOT/.studio-dogfood}"
GATE="${GATE:-40}"
CONTRADICTION_MAX="${CONTRADICTION_MAX:-30}"
SKIP_PROBE="${SKIP_PROBE:-0}"

stamp="$(date -u +%Y-%m-%dT%H-%M-%SZ)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$REPORT_DIR"
report="$REPORT_DIR/dogfood-$stamp.md"

log() { printf '%s\n' "$*" >&2; }
fail() { log "DOGFOOD FAIL: $*"; echo "- **RESULT: FAIL** — $*" >> "$report"; exit 1; }

# --- specscore binary -------------------------------------------------------
if [ -n "${SPECSCORE:-}" ] && [ -x "${SPECSCORE:-}" ]; then
  SS="$SPECSCORE"
else
  SS="$work/specscore"
  log "building specscore from $REPO_ROOT ..."
  ( cd "$REPO_ROOT" && go build -o "$SS" ./cmd/specscore ) || { log "build failed"; exit 1; }
fi

# --- ensure the ops/registries repo is present ------------------------------
if [ ! -d "$OPS_REPO/.git" ]; then
  log "ops repo missing at $OPS_REPO; cloning $OPS_CLONE_URL ..."
  git clone --quiet --depth 1 "$OPS_CLONE_URL" "$OPS_REPO" \
    || log "WARN: could not clone ops repo — registry-derived facts will be absent"
fi

# --- build the workspace.yaml over every git repo under WORKSPACE_ROOT -------
ws="$work/studio.yaml"
{
  echo "name: dogfood"
  echo "repos:"
  for d in "$WORKSPACE_ROOT"/*/; do
    [ -d "$d/.git" ] && printf '  - path: %s\n' "${d%/}"
  done
} > "$ws"
repo_count="$(grep -c 'path:' "$ws")"
[ "$repo_count" -gt 0 ] || fail "no git repos found under $WORKSPACE_ROOT"

db="$work/.specscore-studio/facts.db"

{
  echo "# Studio dogfood — $stamp"
  echo
  echo "- Workspace: \`$WORKSPACE_ROOT\` ($repo_count repos)"
  echo "- Ops/registries: \`$OPS_REPO\`"
  echo
} > "$report"

# --- index ------------------------------------------------------------------
log "indexing $repo_count repos ..."
if ! idx="$("$SS" studio index --workspace "$ws" 2>&1)"; then
  echo "$idx" | tail -5 >> "$report"; fail "studio index errored"
fi
facts="$("$SS" studio facts --db "$db" --count 2>/dev/null || echo '?')"
echo "- Total facts indexed: **$facts**" >> "$report"

# --- probe (live) -----------------------------------------------------------
if [ "$SKIP_PROBE" != "1" ]; then
  log "probing (live HTTP + gh) ..."
  "$SS" studio probe --workspace "$ws" >/dev/null 2>&1 || log "WARN: probe returned nonzero (partial results kept)"
  vb="$("$SS" studio facts --db "$db" --class verified-behavior --count 2>/dev/null || echo '?')"
  echo "- verified-behavior facts after probe: **$vb**" >> "$report"
else
  echo "- Probe: skipped (SKIP_PROBE=1)" >> "$report"
fi

# --- contradictions ---------------------------------------------------------
log "detecting contradictions ..."
cjson="$("$SS" studio contradictions --db "$db" --format json 2>/dev/null || echo '[]')"
read -r ctotal cstatus cnaming <<<"$(printf '%s' "$cjson" | python3 -c '
import json,sys,collections
try: items=json.load(sys.stdin)
except Exception: items=[]
c=collections.Counter(i.get("detector","?") for i in items)
print(len(items), c.get("status-drift",0), c.get("naming-conflict",0))
' 2>/dev/null || echo '? ? ?')"
{
  echo "- Contradictions: **$ctotal** total (status-drift $cstatus, naming-conflict $cnaming)"
} >> "$report"

# --- benchmark --------------------------------------------------------------
log "running the 50-question benchmark (gate $GATE) ..."
bench="$REPO_ROOT/spec/features/cli/studio/answers/benchmark/run.sh"
bench_rc=0
if [ -x "$bench" ]; then
  bench_out="$(SPECSCORE="$SS" "$bench" --db "$db" --gate "$GATE" 2>&1)" || bench_rc=$?
  score="$(printf '%s' "$bench_out" | grep -oE '[0-9]+ answered-with-citations' | head -1)"
  echo "- Benchmark: ${score:-unknown} (gate $GATE) — $([ $bench_rc -eq 0 ] && echo PASS || echo FAIL)" >> "$report"
else
  echo "- Benchmark: runner not found at $bench" >> "$report"; bench_rc=1
fi

# --- verdict ----------------------------------------------------------------
echo >> "$report"
if [ "${ctotal:-0}" != "?" ] && [ "${ctotal:-0}" -gt "$CONTRADICTION_MAX" ] 2>/dev/null; then
  echo "- ⚠️ contradiction spike: $ctotal > $CONTRADICTION_MAX (inspect for detector noise)" >> "$report"
fi
[ "$bench_rc" -eq 0 ] || fail "benchmark below gate $GATE"
echo "- **RESULT: PASS**" >> "$report"
log "report written: $report"
cat "$report" >&2
