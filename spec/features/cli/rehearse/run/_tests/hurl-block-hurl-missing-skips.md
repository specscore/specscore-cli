---
format: https://specscore.md/scenario-specification
---

# Rehearse: hurl-missing-skips

**Status:** pending
**Verifies:** cli/rehearse/run#ac:hurl-missing-skips (REQ: hurl-block)

Scenario source: [../README.md](../README.md) → `### AC: hurl-missing-skips`.

Given a PATH without `hurl` and a scenario containing a hurl block, when I run `specscore rehearse run <that-file>`, then the command exits 0, the scenario is reported `skipped`, and the warning names the `hurl` binary.

```bash
#!/usr/bin/env bash
# Rehearse: cli/rehearse/run#ac:hurl-missing-skips
# Requires: specscore on PATH (override with $SPECSCORE).
set -euo pipefail

SPECSCORE="${SPECSCORE:-specscore}"
# The nested invocation runs under a stripped PATH, so resolve the binary to
# an absolute path first.
SPECSCORE_ABS="$(command -v "$SPECSCORE")" \
  || { echo "FAIL: cannot resolve $SPECSCORE to an absolute path"; exit 1; }

# Given a scenario containing a hurl block — preceded by a bash step that
# would leave a marker file, pinning that the upfront scan skips the scenario
# before ANY of its steps run (the inner fence is assembled via $fence so
# this file stays parseable).
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"
marker="$workdir/step-ran-anyway"
fence='```'
cat > needs-hurl.md <<MD
# Rehearse: needs-hurl fixture

**Status:** pending
**Verifies:** demo/fixture#ac:needs-hurl

${fence}bash
touch "${marker}"
${fence}

${fence}hurl
GET http://127.0.0.1:1/
HTTP 200
${fence}
MD

# And a PATH without hurl
stripped_path="$workdir/empty-path"
mkdir -p "$stripped_path"

# When I run `specscore rehearse run needs-hurl.md` on that PATH
set +e
out="$(env PATH="$stripped_path" "$SPECSCORE_ABS" rehearse run needs-hurl.md 2>&1)"
exit_code=$?
set -e

# Then the command exits 0 (skips do not affect the exit code)
[ "$exit_code" -eq 0 ] || { echo "FAIL: exit code $exit_code, want 0; output: $out"; exit 1; }

# And the scenario is reported `skipped`
printf '%s\n' "$out" | grep -qE '^skipped[[:space:]]+.*needs-hurl\.md' \
  || { echo "FAIL: report does not mark the scenario skipped: $out"; exit 1; }
printf '%s\n' "$out" | grep -q '1 skipped' \
  || { echo "FAIL: totals line does not count the skip: $out"; exit 1; }

# And the warning names the `hurl` binary
printf '%s\n' "$out" | grep -q 'hurl' \
  || { echo "FAIL: no warning naming the hurl binary: $out"; exit 1; }

# And none of the scenario's steps ran — not even the earlier bash step
[ ! -f "$marker" ] || { echo "FAIL: a step ran despite the upfront skip"; exit 1; }

echo "PASS: hurl-missing-skips"
```

---
*This document follows the https://specscore.md/scenario-specification*
