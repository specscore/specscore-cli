// Package rehearse is the rehearse-report ingestion adapter: it reads
// `.specscore/rehearse/latest.json` from the repo and emits `verified-behavior`
// facts — one `verified-by` and one `has-verification-status` pair for every
// scenario–AC combination whose status is `pass` or `fail`.
//
// Feature: cli/rehearse/evidence (REQ: adapter-rehearse, REQ: verification-facts,
// REQ: verified-behavior-class, REQ: observed-at-run-time)
//
// Subjects are emitted repo-relative (`#<verifies-entry>`, e.g.
// `#x#ac:y`); the pipeline prefixes them with the workspace-scoped repo
// slug. Scenarios with status `skipped` or `no-steps`, or with an empty
// `verifies` list, emit no facts. A missing report file is silent; an
// unreadable or unparsable file emits one warning (REQ: partial-tolerance).
package rehearse

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/specscore/specscore-cli/internal/studio/fact"
)

const (
	adapterID = "rehearse"
	// adapterVersion bumps when the emitted fact vocabulary or shapes change.
	adapterVersion = "0.1.0"

	// reportPath is the conventional repo-relative path of the persisted
	// rehearse report (REQ: adapter-rehearse).
	reportPath = ".specscore/rehearse/latest.json"
)

// Test seams — package-level vars wrapping external functions.
// Production code calls these vars; tests replace them via t.Cleanup.
var osReadFileFn = os.ReadFile

// Adapter is the rehearse-report ingestion adapter. The zero value is ready
// to use.
type Adapter struct{}

// New returns the adapter.
func New() Adapter { return Adapter{} }

// ID returns the stable adapter id stamped into every emitted fact.
func (Adapter) ID() string { return adapterID }

// Version returns the adapter version stamped into every emitted fact.
func (Adapter) Version() string { return adapterVersion }

// runReport is the minimal decode shape for the persisted report envelope.
// Field names match the JSON produced by internal/rehearse/runner/provenance.go.
// This package does NOT import the runner package; the JSON file is the contract.
type runReport struct {
	StartedAt string           `json:"started_at"`
	Scenarios []scenarioReport `json:"scenarios"`
}

// scenarioReport is the per-scenario entry in the report.
type scenarioReport struct {
	File     string   `json:"file"`
	Status   string   `json:"status"`
	Verifies []string `json:"verifies"`
}

// Ingest reads the rehearse report at <repoPath>/.specscore/rehearse/latest.json
// and emits two facts per scenario–AC pair for pass/fail scenarios:
//
//  1. subject `#<verifies-entry>`, predicate `verified-by`, object = scenario file
//  2. subject `#<verifies-entry>`, predicate `has-verification-status`, object = status
//
// A missing report is silently ignored. An unreadable or unparsable report
// produces one warning and no facts (REQ: partial-tolerance).
func (Adapter) Ingest(repoPath string) ([]fact.Fact, []fact.Warning) {
	relPath := reportPath
	absPath := filepath.Join(repoPath, filepath.FromSlash(relPath))

	data, err := osReadFileFn(absPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil // absence is normal
	}
	if err != nil {
		return nil, []fact.Warning{{Message: fmt.Sprintf("reading %s: %v", relPath, err)}}
	}

	var report runReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, []fact.Warning{{Message: fmt.Sprintf("parsing %s: %v", relPath, err)}}
	}

	var facts []fact.Fact
	ev := fact.Evidence{Class: fact.VerifiedBehavior, Pointer: relPath}
	for _, sc := range report.Scenarios {
		if sc.Status != "pass" && sc.Status != "fail" {
			continue // skipped / no-steps → no evidence
		}
		for _, ref := range sc.Verifies {
			subject := "#" + ref
			facts = append(facts,
				fact.Fact{
					Subject:    subject,
					Predicate:  "verified-by",
					Object:     sc.File,
					Evidence:   ev,
					ObservedAt: report.StartedAt,
				},
				fact.Fact{
					Subject:    subject,
					Predicate:  "has-verification-status",
					Object:     sc.Status,
					Evidence:   ev,
					ObservedAt: report.StartedAt,
				},
			)
		}
	}
	return facts, nil
}
