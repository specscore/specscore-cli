// Package rehearse provides fixture-based tests for the rehearse adapter.
// Tests live in the same package so they can access internal test seams and
// constants (adapterID, adapterVersion).
//
// Feature: cli/rehearse/evidence (REQ: adapter-rehearse, REQ: verification-facts,
// REQ: verified-behavior-class, REQ: observed-at-run-time)
package rehearse

import (
	"errors"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/studio/fact"
)

const (
	goodRepo   = "testdata/goodrepo"
	brokenRepo = "testdata/brokenrepo"
)

func TestAdapter_Identity(t *testing.T) {
	a := New()
	if a.ID() != "rehearse" {
		t.Errorf("ID() = %q, want %q", a.ID(), "rehearse")
	}
	if a.Version() == "" {
		t.Error("Version() is empty")
	}
}

// TestIngest_MissingReport verifies absence of the report file is silent:
// no facts, no warnings (REQ: adapter-rehearse, AC: missing-report-silent).
func TestIngest_MissingReport(t *testing.T) {
	dir := t.TempDir() // no .specscore/rehearse/latest.json
	facts, warnings := New().Ingest(dir)
	if len(facts) != 0 {
		t.Errorf("Ingest(missing report) facts = %+v, want none", facts)
	}
	if len(warnings) != 0 {
		t.Errorf("Ingest(missing report) warnings = %+v, want none", warnings)
	}
}

// TestIngest_MalformedReport verifies an unreadable/unparsable file produces
// exactly one warning and no facts (REQ: adapter-rehearse, REQ: partial-tolerance,
// AC: malformed-report-warns).
func TestIngest_MalformedReport(t *testing.T) {
	facts, warnings := New().Ingest(brokenRepo)
	if len(facts) != 0 {
		t.Errorf("Ingest(broken repo) facts = %+v, want none", facts)
	}
	if len(warnings) != 1 {
		t.Fatalf("len(warnings) = %d, want 1: %+v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0].Message, "latest.json") {
		t.Errorf("warning message %q does not name the report file", warnings[0].Message)
	}
}

// TestIngest_ReadErrorWarns covers the os.ReadFile failure path via the test seam.
func TestIngest_ReadErrorWarns(t *testing.T) {
	old := osReadFileFn
	osReadFileFn = func(string) ([]byte, error) { return nil, errors.New("read boom") }
	t.Cleanup(func() { osReadFileFn = old })

	facts, warnings := New().Ingest(goodRepo)
	if len(facts) != 0 {
		t.Errorf("facts = %+v, want none", facts)
	}
	if len(warnings) != 1 {
		t.Fatalf("len(warnings) = %d, want 1: %+v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0].Message, "latest.json") {
		t.Errorf("warning message %q does not name the report file", warnings[0].Message)
	}
}

// TestIngest_GoodRepo exercises the full scenario set: pass emits two facts,
// fail emits two facts with object "fail", skipped/no-steps/empty-verifies
// emit nothing, and ObservedAt is the report's started_at for all facts.
//
// ACs: adapter-emits-verified-by, fail-status-fact, skipped-scenario-no-facts,
// observed-at-run-time.
func TestIngest_GoodRepo(t *testing.T) {
	facts, warnings := New().Ingest(goodRepo)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %+v", warnings)
	}

	const startedAt = "2026-01-02T03:04:05Z"
	const reportPtr = ".specscore/rehearse/latest.json"

	// pass scenario: spec/features/x/_tests/s.md verifies x#ac:y → 2 facts
	// fail scenario: spec/features/x/_tests/fail.md verifies x#ac:y, x#ac:z → 4 facts
	// skipped, no-steps, empty-verifies → 0 facts
	// total: 6 facts
	want := []fact.Fact{
		// pass scenario
		verifiedFact("#x#ac:y", "verified-by", "spec/features/x/_tests/s.md", reportPtr, startedAt),
		verifiedFact("#x#ac:y", "has-verification-status", "pass", reportPtr, startedAt),
		// fail scenario — two verifies entries
		verifiedFact("#x#ac:y", "verified-by", "spec/features/x/_tests/fail.md", reportPtr, startedAt),
		verifiedFact("#x#ac:y", "has-verification-status", "fail", reportPtr, startedAt),
		verifiedFact("#x#ac:z", "verified-by", "spec/features/x/_tests/fail.md", reportPtr, startedAt),
		verifiedFact("#x#ac:z", "has-verification-status", "fail", reportPtr, startedAt),
	}

	if len(facts) != len(want) {
		t.Fatalf("Ingest(goodRepo) = %d facts, want %d:\ngot:  %+v\nwant: %+v",
			len(facts), len(want), facts, want)
	}
	for i, f := range facts {
		if f != want[i] {
			t.Errorf("facts[%d] mismatch:\ngot:  %+v\nwant: %+v", i, f, want[i])
		}
	}
}

// TestIngest_ObservedAtFromReport verifies that every emitted fact's
// ObservedAt is the report's started_at, not any other timestamp.
// AC: observed-at-run-time.
func TestIngest_ObservedAtFromReport(t *testing.T) {
	facts, _ := New().Ingest(goodRepo)
	for _, f := range facts {
		if f.ObservedAt != "2026-01-02T03:04:05Z" {
			t.Errorf("fact ObservedAt = %q, want %q (report's started_at): %+v",
				f.ObservedAt, "2026-01-02T03:04:05Z", f)
		}
	}
}

// TestIngest_SkippedAndNoStepsEmitNothing proves the count-is-0 AC at the
// package level when the report contains only skipped/no-steps scenarios with
// non-empty verifies (AC: skipped-scenario-no-facts).
func TestIngest_SkippedAndNoStepsEmitNothing(t *testing.T) {
	old := osReadFileFn
	osReadFileFn = func(string) ([]byte, error) {
		return []byte(`{
  "runner_version": "0.3.0",
  "git_sha": "",
  "git_dirty": false,
  "started_at": "2026-01-02T03:04:05Z",
  "scenarios": [
    {"file": "s.md", "status": "skipped",  "verifies": ["x#ac:y"], "duration_ms": 0, "bag": {}, "steps": []},
    {"file": "t.md", "status": "no-steps", "verifies": ["x#ac:y"], "duration_ms": 0, "bag": {}, "steps": []}
  ]
}`), nil
	}
	t.Cleanup(func() { osReadFileFn = old })

	facts, warnings := New().Ingest(goodRepo)
	if len(facts) != 0 {
		t.Errorf("Ingest(skipped+no-steps) = %d facts, want 0: %+v", len(facts), facts)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %+v", warnings)
	}
}

// verifiedFact constructs the expected fact shape produced by the adapter.
// The Adapter field is left at zero value — the pipeline stamps id+version
// centrally (same pattern as the manifests/specscore/codegraph siblings).
func verifiedFact(subject, predicate, object, pointer, observedAt string) fact.Fact {
	return fact.Fact{
		Subject:    subject,
		Predicate:  predicate,
		Object:     object,
		Evidence:   fact.Evidence{Class: fact.VerifiedBehavior, Pointer: pointer},
		ObservedAt: observedAt,
	}
}
