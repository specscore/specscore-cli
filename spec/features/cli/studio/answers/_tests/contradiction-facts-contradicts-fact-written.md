---
format: https://specscore.md/scenario-specification
---

# Rehearse: contradicts-fact-written

**Status:** pass-capable
**Verifies:** cli/studio/answers#ac:contradicts-fact-written (REQ: contradiction-facts)

Scenario source: [../README.md](../README.md) → `### AC: contradicts-fact-written`.

Given an indexed store containing exactly one detectable contradiction, when I run `specscore studio contradictions` and then `specscore studio facts --predicate contradicts --format json`, then the facts JSON contains a `contradicts` fact whose subject and object are the two sides' fact refs, evidence_class `derived`, adapter id `contradictions`, and evidence_pointer naming the detector, and re-running `contradictions` does not create a duplicate (same subject/predicate/object).

Seam: the one contradiction is a naming-conflict — two ecosystem maps fronting
the domain `d.example` with different products (`fronts` is in the detector's
single-valued predicate set) — so exactly one `contradicts` fact is written.
Idempotence is proven by running `contradictions` twice and asserting the
`contradicts` count is unchanged.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:contradicts-fact-written
# Requires: specscore on PATH (override with $SPECSCORE), python3.
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"

mkdir -p ops ops-legacy
cat > ops/ecosystem.yaml <<'YAML'
products:
  - id: ext-foo
    domains:
      - d.example
YAML
cat > ops-legacy/ecosystem-legacy.yaml <<'YAML'
products:
  - id: foo-contract
    domains:
      - d.example
YAML
cat > studio.yaml <<'YAML'
name: demo
repos:
  - ops
  - ops-legacy
YAML
"$SPECSCORE_ABS" studio index >/dev/null

# First run writes the contradicts fact.
"$SPECSCORE_ABS" studio contradictions >/dev/null
json1="$("$SPECSCORE_ABS" studio facts --predicate contradicts --format json)"

# Second run must be idempotent (no duplicate).
"$SPECSCORE_ABS" studio contradictions >/dev/null
json2="$("$SPECSCORE_ABS" studio facts --predicate contradicts --format json)"

JSON1="$json1" JSON2="$json2" python3 - <<'PY'
import json, os
c1 = json.loads(os.environ["JSON1"])
c2 = json.loads(os.environ["JSON2"])
assert len(c1) == 1, f"FAIL: expected exactly one contradicts fact, got {c1}"
f = c1[0]
assert f["predicate"] == "contradicts", f
assert f["evidence_class"] == "derived", f"FAIL: class must be derived; got {f}"
assert f["adapter"]["id"] == "contradictions", f"FAIL: adapter must be contradictions; got {f}"
assert f["evidence_pointer"] == "naming-conflict", f"FAIL: pointer must name the detector; got {f}"
assert "|" in f["subject"] and "|" in f["object"], f"FAIL: sides must be fact refs; got {f}"
# Idempotence: no duplicate triple.
def triples(facts):
    return {(x["subject"], x["predicate"], x["object"]) for x in facts}
assert triples(c1) == triples(c2), f"FAIL: re-run duplicated contradicts facts: {c2}"
assert len(c2) == 1, f"FAIL: re-run created a duplicate; got {c2}"
PY

echo "PASS: contradicts-fact-written"
```

---
*This document follows the https://specscore.md/scenario-specification*
