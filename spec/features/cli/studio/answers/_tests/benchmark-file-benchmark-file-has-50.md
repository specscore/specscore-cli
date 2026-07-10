---
format: https://specscore.md/scenario-specification
---

# Rehearse: benchmark-file-has-50

**Status:** pass-capable
**Verifies:** cli/studio/answers#ac:benchmark-file-has-50 (REQ: benchmark-file)

Scenario source: [../README.md](../README.md) → `### AC: benchmark-file-has-50`.

Given the committed `benchmark/questions.jsonl`, when I count its lines and parse each as JSON with fields `id`, `question`, `template`, `parameter`, `expectation`, then there are exactly 50 instances, every `template` is one of the documented templates or empty (for `expected-unanswerable`), and the per-template counts match the `## Benchmark composition` table.

Pure file assertion over the committed benchmark (no network, no store). The
committed file is located via `$REPO_ROOT` (CI passes it) or the parent process
cwd.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:benchmark-file-has-50
# Requires: python3.
set -euo pipefail

repo="${REPO_ROOT:-${SPECSCORE_CLI_REPO:-}}"
if [ -z "$repo" ]; then
  if [ -e "/proc/$PPID/cwd" ]; then
    repo="$(readlink "/proc/$PPID/cwd")"
  else
    repo="$(pwd)"
  fi
fi
questions="$repo/spec/features/cli/studio/answers/benchmark/questions.jsonl"
[ -f "$questions" ] \
  || { echo "FAIL: cannot locate questions.jsonl (looked in '$questions'); set \$REPO_ROOT"; exit 1; }

QUESTIONS="$questions" python3 - <<'PY'
import json, os
from collections import Counter

# what-verifies carries no benchmark instances (no rehearse evidence exists in
# the Sneat dogfood workspace yet); the template stays in ask, unit-tested.
COMPOSITION = {
    "who-fronts": 5, "what-repos-implement": 4, "status-of": 5, "aliases-of": 3,
    "member-of": 3, "is-it-live": 4, "ci-status-of": 4,
    "contradictions-for": 4, "freshness-of": 2, "what-uses": 3, "version-pins": 2,
    "aliases-resolve": 2,
}
lines = [l for l in open(os.environ["QUESTIONS"]) if l.strip()]
assert len(lines) == 50, f"FAIL: expected 50 instances, got {len(lines)}"

by_template = Counter()
exp = Counter()
ids = set()
documented = set(COMPOSITION) | {""}
for n, l in enumerate(lines, 1):
    o = json.loads(l)
    for field in ("id", "question", "template", "parameter", "expectation"):
        assert field in o, f"FAIL: line {n} missing field {field!r}: {o}"
    assert o["id"] not in ids, f"FAIL: duplicate id {o['id']!r}"
    ids.add(o["id"])
    assert o["template"] in documented, \
        f"FAIL: line {n} template {o['template']!r} is not documented"
    assert o["expectation"] in ("answerable", "expected-unanswerable"), \
        f"FAIL: line {n} bad expectation {o['expectation']!r}"
    exp[o["expectation"]] += 1
    if o["expectation"] == "answerable":
        assert o["template"] != "", f"FAIL: answerable line {n} has empty template"
        by_template[o["template"]] += 1
    else:
        assert o["template"] == "", f"FAIL: expected-unanswerable line {n} names a template"

assert exp["answerable"] == 41, f"FAIL: answerable {exp['answerable']}, want 41"
assert exp["expected-unanswerable"] == 9, f"FAIL: unanswerable {exp['expected-unanswerable']}, want 9"
for tmpl, want in COMPOSITION.items():
    got = by_template.get(tmpl, 0)
    assert got == want, f"FAIL: template {tmpl} has {got}, table wants {want}"
print("benchmark composition matches the table: 41 answerable / 9 unanswerable")
PY

echo "PASS: benchmark-file-has-50"
```

---
*This document follows the https://specscore.md/scenario-specification*
