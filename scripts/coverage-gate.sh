#!/usr/bin/env bash
# Single source of truth for the test-coverage gate.
#
# Used by both CI (.github/workflows/go-ci.yml) and the local pre-push hook
# (.githooks/pre-push). Always produces an isolated, uncached coverage profile
# from the current tree, then fails when total statement coverage is below
# THRESHOLD. A reused cover.out is not evidence for the candidate being gated.
set -euo pipefail

THRESHOLD=100

COVER_PROFILE=$(mktemp "${TMPDIR:-/tmp}/specscore-coverage.XXXXXX")
trap 'rm -f "$COVER_PROFILE"' EXIT

go test ./... -count=1 -coverprofile="$COVER_PROFILE" -covermode=atomic

PCT=$(go tool cover -func="$COVER_PROFILE" | awk '/^total:/ {gsub(/%/,""); print $NF}')
echo "Total coverage: ${PCT}%"
awk "BEGIN { if (${PCT}+0 < ${THRESHOLD}) { print \"Coverage ${PCT}% is below ${THRESHOLD}% threshold\"; exit 1 } }"
