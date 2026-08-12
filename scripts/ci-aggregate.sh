#!/usr/bin/env bash
# Reduce conditional CI jobs to one stable required check. A skipped job is
# accepted only when the path classifier declared that job inapplicable.
set -euo pipefail

fail() {
  printf '::error::%s\n' "$*" >&2
  exit 1
}

if [[ "${CLASSIFY_RESULT:-}" != success ]]; then
  fail "path classifier did not succeed: ${CLASSIFY_RESULT:-missing}"
fi

case "${GO_APPLICABLE:-}" in
true|false) ;;
*) fail "invalid Go applicability: ${GO_APPLICABLE:-missing}" ;;
esac
case "${DOGFOOD_APPLICABLE:-}" in
true|false) ;;
*) fail "invalid dogfood applicability: ${DOGFOOD_APPLICABLE:-missing}" ;;
esac

require_gate() {
  local name="$1"
  local applicable="$2"
  local result="$3"

  case "${applicable}:${result}" in
  true:success | false:skipped) return ;;
  true:*) fail "$name was applicable but did not succeed: ${result:-missing}" ;;
  false:*) fail "$name was inapplicable but was not skipped: ${result:-missing}" ;;
  *) fail "$name has invalid applicability/result: ${applicable:-missing}/${result:-missing}" ;;
  esac
}

require_gate "Build, vet, test" "$GO_APPLICABLE" "${TEST_RESULT:-}"
require_gate "Windows event process-tree lifecycle" "$GO_APPLICABLE" "${WINDOWS_RESULT:-}"
require_gate "Build advertised release targets" "$GO_APPLICABLE" "${RELEASE_TARGETS_RESULT:-}"
require_gate "Rehearse corpus" "$GO_APPLICABLE" "${REHEARSE_CORPUS_RESULT:-}"
require_gate "Dogfood lint" "$DOGFOOD_APPLICABLE" "${DOGFOOD_RESULT:-}"
