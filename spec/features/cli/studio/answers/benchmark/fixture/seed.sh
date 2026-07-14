#!/usr/bin/env bash
# Seed the hermetic benchmark fixture store (cli/studio/answers#req:exit-gate-fixture-and-sneat).
#
# `studio index` over benchmark/fixture/ produces every declared/derived fact
# the answerable templates need. The probe-only verified-behavior facts — each
# domain's live serves-status (including the dead investor-CTA domain's `down`)
# and each repo's ci-status — cannot be produced hermetically (probe does
# network / gh I/O), so this script seeds them with sqlite3, the same
# stubbed-seam pattern the probe corpus (`cli/studio/probe/_tests`) uses. The
# result is a store indistinguishable from one `studio index` + `studio probe`
# built, but with no network and no gh checkout. The seeded subjects/objects
# mirror the REAL Sneat dogfood store (the 4 status-drift domains, the 526
# agendum.app edge, green + red repos) so the one benchmark file scores
# identically on both.
#
# Usage: seed.sh <specscore-bin> <fixture-dir> <db-path>
#   Copying the fixture into a scratch dir first is the caller's job; this
#   script indexes <fixture-dir> in place into <db-path> and seeds the probe
#   facts.
set -euo pipefail

SS="${1:?usage: seed.sh <specscore-bin> <fixture-dir> <db-path>}"
FIXTURE="${2:?usage: seed.sh <specscore-bin> <fixture-dir> <db-path>}"
DB="${3:?usage: seed.sh <specscore-bin> <fixture-dir> <db-path>}"

command -v sqlite3 >/dev/null 2>&1 || { echo "seed.sh: sqlite3 not on PATH" >&2; exit 1; }

# Give the Backstage fixture the same canonical remote identity as the real
# repository. Repo IDs are remote-derived, so indexing an unconfigured scratch
# copy would otherwise produce a local/<name>-<path-hash> ID that cannot match
# the real-workspace benchmark questions.
git -C "$FIXTURE/backstage" init -q
git -C "$FIXTURE/backstage" remote add origin git@github.com:sneat-co/backstage.git

# 1. Index the fixture (declared + derived facts).
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

# Live product domains (fronted via ecosystem.yaml — the is-it-live two-step
# hop) plus agendum.app, a verified-only domain answering 526.
seed_fact "anymeter.app"    serves-status 200  probe-domain "https://anymeter.app/"
seed_fact "assetus.app"     serves-status 200  probe-domain "https://assetus.app/"
seed_fact "gameboard.live"  serves-status 200  probe-domain "https://gameboard.live/"
seed_fact "calendarius.app" serves-status 200  probe-domain "https://calendarius.app/"
seed_fact "agendum.app"     serves-status 526  probe-domain "https://agendum.app/"

# The four status-drift domains mirroring the real Sneat drift set: each has a
# disagreeing declared serves-status in sneat-ops/domains.json, so `studio
# contradictions` yields the same four status-drift items the dogfood store
# shows — the contradictions-for instances answer identically on both.
seed_fact "fillless.com"    serves-status down probe-domain "https://fillless.com/"
seed_fact "issuenumber.one" serves-status 200  probe-domain "https://issuenumber.one/"
seed_fact "agiledger.app"   serves-status 200  probe-domain "https://agiledger.app/"
seed_fact "sneat.ai"        serves-status down probe-domain "https://sneat.ai/"

# CI conclusions per repo (green + a red repo for the ci-status-of instances).
seed_fact "backstage"     ci-status success probe-ci "gh:repos/sneat-co/backstage/actions"
seed_fact "gameboard"     ci-status success probe-ci "gh:repos/sneat-co/gameboard/actions"
seed_fact "sneat-go-core" ci-status success probe-ci "gh:repos/sneat-co/sneat-go-core/actions"
seed_fact "sneat-libs"    ci-status failure probe-ci "gh:repos/sneat-co/sneat-libs/actions"

echo "seed.sh: indexed $FIXTURE and seeded 13 probe-stubbed verified-behavior facts into $DB"
