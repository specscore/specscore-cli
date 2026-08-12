#!/usr/bin/env bash
# Classify a GitHub Actions event using only commits already present in the
# checkout. Unknown event/base state is deliberately fail-closed: every gate
# runs rather than making a required check appear green without evidence.
set -euo pipefail

emit_all() {
  printf 'go=true\ndogfood=true\n' >>"${GITHUB_OUTPUT:?GITHUB_OUTPUT is required}"
}

case "${EVENT_NAME:-}" in
pull_request)
  base="${PR_BASE_SHA:-}"
  ;;
push)
  base="${PUSH_BEFORE_SHA:-}"
  ;;
workflow_dispatch)
  # A manual run has no comparison base. Preserve the old behavior: run both.
  emit_all
  exit 0
  ;;
*)
  emit_all
  exit 0
  ;;
esac

head="${HEAD_SHA:-}"
if [[ -z "$base" || -z "$head" || "$base" =~ ^0+$ ]]; then
  emit_all
  exit 0
fi

if ! git cat-file -e "${base}^{commit}" 2>/dev/null || ! git cat-file -e "${head}^{commit}" 2>/dev/null; then
  emit_all
  exit 0
fi

changed="$(mktemp "${TMPDIR:-/tmp}/specscore-ci-paths.XXXXXX")"
trap 'rm -f "$changed"' EXIT
if ! git diff --name-status -z --find-renames --find-copies --find-copies-harder \
  --no-ext-diff --no-textconv "$base" "$head" >"$changed"; then
  emit_all
  exit 0
fi

go=false
dogfood=false

classify_path() {
  local path="$1"
  case "$path" in
  .github/workflows/go-ci.yml|.github/workflows/dogfood.yml|scripts/ci-classify-paths.sh|scripts/ci-aggregate.sh)
    go=true
    dogfood=true
    ;;
  .goreleaser.yml|go.mod|go.sum|*.go|spec/features/*/_tests/*)
    go=true
    ;;
  esac

  case "$path" in
  spec/*|cmd/*|internal/*|pkg/*|docs/lint-rules.md|go.mod|go.sum)
    dogfood=true
    ;;
  esac
}

# --name-status -z emits status, then one path for ordinary changes and both
# source and destination paths for renames/copies. Reading each NUL-delimited
# field keeps whitespace and newlines in valid Git paths unambiguous.
while IFS= read -r -d '' status; do
  case "$status" in
  R*|C*)
    if ! IFS= read -r -d '' source_path || ! IFS= read -r -d '' destination_path; then
      emit_all
      exit 0
    fi
    classify_path "$source_path"
    classify_path "$destination_path"
    ;;
  A|D|M|T|U|X|B)
    if ! IFS= read -r -d '' path; then
      emit_all
      exit 0
    fi
    classify_path "$path"
    ;;
  *)
    # Git adding a status we do not understand must make CI more conservative,
    # never silently reduce the applicable gate set.
    emit_all
    exit 0
    ;;
  esac
done <"$changed"

printf 'go=%s\ndogfood=%s\n' "$go" "$dogfood" >>"${GITHUB_OUTPUT:?GITHUB_OUTPUT is required}"
