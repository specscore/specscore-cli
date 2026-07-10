---
format: https://specscore.md/scenario-specification
---

# Rehearse: ask-unroutable-lists-templates

**Status:** pass-capable
**Verifies:** cli/studio/answers#ac:ask-unroutable-lists-templates (REQ: ask-unroutable)

Scenario source: [../README.md](../README.md) → `### AC: ask-unroutable-lists-templates`.

Given any indexed store, when I run `specscore studio ask "why does contactus exist"`, then the command exits 1, prints a "no template matched" notice, and lists the routable templates (the same content as `specscore studio ask --list`).

Seam: any non-empty fixture store suffices; the question is a why-exists shape
with no template, so it routes to the unanswerable path.

```bash
#!/usr/bin/env bash
# Rehearse: cli/studio/answers#ac:ask-unroutable-lists-templates
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"

mkdir -p ops
cat > ops/ecosystem.yaml <<'YAML'
products:
  - id: contactus
    brand: ContactUs
YAML
cat > studio.yaml <<'YAML'
name: demo
repos:
  - ops
YAML
"$SPECSCORE_ABS" studio index >/dev/null

set +e
out="$("$SPECSCORE_ABS" studio ask "why does contactus exist" 2>&1)"
rc=$?
set -e
[ "$rc" -eq 1 ] || { echo "FAIL: exit code $rc, want 1 (unroutable); out: $out"; exit 1; }

case "$out" in
  *"no template matched"*) ;;
  *) echo "FAIL: no 'no template matched' notice: $out"; exit 1 ;;
esac

# Lists the routable templates — same content as --list. Compare the template
# ids in the unroutable output against `ask --list`.
list_out="$("$SPECSCORE_ABS" studio ask --list)"
for tmpl in who-fronts what-repos-implement status-of aliases-of member-of \
            is-it-live ci-status-of what-verifies contradictions-for \
            freshness-of what-uses version-pins aliases-resolve; do
  case "$out" in *"$tmpl"*) ;; *) echo "FAIL: unroutable output missing template $tmpl"; exit 1 ;; esac
  case "$list_out" in *"$tmpl"*) ;; *) echo "FAIL: --list missing template $tmpl"; exit 1 ;; esac
done

echo "PASS: ask-unroutable-lists-templates"
```

---
*This document follows the https://specscore.md/scenario-specification*
