#!/usr/bin/env bash
# Seed the hermetic benchmark fixture store (cli/studio/answers#req:exit-gate-fixture-and-sneat).
#
# `studio index` over benchmark/fixture/ produces every declared/derived fact
# the answerable templates need. The probe-only verified-behavior facts — a
# domain's live serves-status (and the dead investor-CTA domain's `down`) and
# each repo's ci-status — cannot be produced hermetically (probe does network /
# gh I/O), so this script seeds them with sqlite3, the same stubbed-seam pattern
# the probe corpus (`cli/studio/probe/_tests`) uses. The result is a store
# indistinguishable from one `studio index` + `studio probe` built, but with no
# network and no gh checkout.
#
# Usage: seed.sh <specscore-bin> <fixture-dir> <db-path>
#   Copies the fixture into a scratch dir first is the caller's job; this script
#   indexes <fixture-dir> in place into <db-path> and seeds the probe facts.
set -euo pipefail

SS="${1:?usage: seed.sh <specscore-bin> <fixture-dir> <db-path>}"
FIXTURE="${2:?usage: seed.sh <specscore-bin> <fixture-dir> <db-path>}"
DB="${3:?usage: seed.sh <specscore-bin> <fixture-dir> <db-path>}"

command -v sqlite3 >/dev/null 2>&1 || { echo "seed.sh: sqlite3 not on PATH" >&2; exit 1; }

# 1. Index the fixture (declared + derived + rehearse verified-behavior facts).
( cd "$FIXTURE" && "$SS" studio index --db "$DB" --no-ingr >/dev/null )

# 2. Seed the probe-stubbed verified-behavior facts (the network/gh half).
#    Timestamps are fixed so the fixture is deterministic.
STAMP="2026-07-03T08:00:00Z"
seed_fact() {
  # subject predicate object adapter pointer
  sqlite3 "$DB" "INSERT INTO facts
    (subject, predicate, object, evidence_class, evidence_pointer,
     adapter_id, adapter_version, observed_at, verified_at, ecosystem)
    VALUES ('$1','$2','$3','verified-behavior','$5','$4','0.1.0','$STAMP','$STAMP','sneat');"
}

# Live domains (probe agreed with the declared 200) and the dead investor-CTA
# domain (declared 200 in domains.json, probes `down` → status-drift + a real
# is-it-live "dead" answer).
seed_fact "sizeus.app"     serves-status 200  probe-domain "https://sizeus.app/"
seed_fact "gameboard.app"  serves-status 200  probe-domain "https://gameboard.app/"
seed_fact "contactus.app"  serves-status 200  probe-domain "https://contactus.app/"
seed_fact "invest.example" serves-status down probe-domain "https://invest.example/"

# CI conclusions per repo (green + a red repo for the ci-status-of instances).
seed_fact "gameboard"  ci-status success probe-ci "gh:repos/sneat-co/gameboard/actions"
seed_fact "contactus"  ci-status success probe-ci "gh:repos/sneat-co/contactus/actions"
seed_fact "sneat-libs" ci-status failure probe-ci "gh:repos/sneat-co/sneat-libs/actions"

echo "seed.sh: indexed $FIXTURE and seeded 7 probe-stubbed verified-behavior facts into $DB"
