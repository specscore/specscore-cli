#!/usr/bin/env bash
# Pre-PR verify gate for specstudio:pull-request.
#
# This is the command behind the `type: deterministic` reviewer in
# gates.pull_request.pre_dispatch (specscore.yaml). It runs the project's
# pre-PR checks before a pull request is opened; its exit code is the gate
# verdict (0 -> Approved; non-zero -> Issues Found, PR creation blocked).
#
# Today it delegates to the shared coverage gate (the same gate CI and the
# pre-push hook enforce). Add any further PR-specific checks below as they
# arise — keep coverage-gate.sh as the single source of truth for coverage.
set -euo pipefail

cd "$(dirname "$0")/.."   # run from repo root regardless of caller cwd

./scripts/coverage-gate.sh
