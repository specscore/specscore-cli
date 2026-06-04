#!/usr/bin/env bash
# Single source of truth for the test-coverage gate.
#
# Used by both CI (.github/workflows/go-ci.yml) and the local pre-push hook
# (.githooks/pre-push). Generates cover.out if it is not already present, then
# fails when total statement coverage is below THRESHOLD.
set -euo pipefail

THRESHOLD=100

if [ ! -f cover.out ]; then
  go test ./... -count=1 -coverprofile=cover.out -covermode=atomic
fi

PCT=$(go tool cover -func=cover.out | awk '/^total:/ {gsub(/%/,""); print $NF}')
echo "Total coverage: ${PCT}%"
awk "BEGIN { if (${PCT}+0 < ${THRESHOLD}) { print \"Coverage ${PCT}% is below ${THRESHOLD}% threshold\"; exit 1 } }"
