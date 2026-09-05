package trace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseFeatureNormalizesChildrenAndScenarios(t *testing.T) {
	root := t.TempDir()
	featureDir := filepath.Join(root, "spec", "features", "checkout")
	if err := os.MkdirAll(filepath.Join(featureDir, "_tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	readme := `---
format: https://specscore.md/feature-specification
status: Implementing
---

# Feature: Checkout

**Status:** Implementing

## Behavior

### Totals

#### REQ: totals

Totals MUST be calculated.

## Acceptance Criteria

### AC: discounted-total

**Requirements:** checkout#REQ:totals

The total includes discounts.

## Open Questions

None.
`
	path := filepath.Join(featureDir, "README.md")
	if err := os.WriteFile(path, []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}
	scenario := "# Scenario: discounted total\n\n**Validates:** [checkout#ac:discounted-total](../README.md#ac-discounted-total)\n\n## Steps\n\nGIVEN a cart\nWHEN discount applies\nTHEN total is reduced\n"
	if err := os.WriteFile(filepath.Join(featureDir, "_tests", "discounted-total.md"), []byte(scenario), 0o644); err != nil {
		t.Fatal(err)
	}
	record, err := ParseFeature(path)
	if err != nil {
		t.Fatal(err)
	}
	if record.ID != "checkout" || record.Title != "Checkout" || record.Status != "Implementing" {
		t.Fatalf("feature identity = %+v", record)
	}
	if len(record.Requirements) != 1 || record.Requirements[0].ID != "checkout#req:totals" || record.Requirements[0].Source.StartLine != 14 {
		t.Fatalf("requirements = %+v", record.Requirements)
	}
	if len(record.AcceptanceCriteria) != 1 || record.AcceptanceCriteria[0].ID != "checkout#ac:discounted-total" || len(record.AcceptanceCriteria[0].Requirements) != 1 || record.AcceptanceCriteria[0].Requirements[0] != "checkout#req:totals" {
		t.Fatalf("ACs = %+v", record.AcceptanceCriteria)
	}
	if len(record.Scenarios) != 1 || record.Scenarios[0].ID != "checkout#scenario:discounted-total" || record.Scenarios[0].Validates[0] != "checkout#ac:discounted-total" {
		t.Fatalf("scenarios = %+v", record.Scenarios)
	}
	if record.AcceptanceCriteria[0].Source.EndLine >= len(splitLines([]byte(readme))) {
		t.Fatalf("AC range should stop before trailing content: %+v", record.AcceptanceCriteria[0].Source)
	}
	encoded, err := json.Marshal(record)
	if err != nil || len(encoded) == 0 {
		t.Fatalf("JSON marshal = %v %s", err, encoded)
	}
}

func TestSnapshotForRootIncludesContractVersion(t *testing.T) {
	snapshot, err := SnapshotForRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Version != ContractVersion || snapshot.Features == nil {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestParseScenarioWithoutValidates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spec", "features", "auth", "_tests", "login.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# Scenario: login\n\n## Steps\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := ParseScenario(path)
	if err != nil {
		t.Fatal(err)
	}
	if r.FeatureID != "auth" || r.Slug != "login" || len(r.Validates) != 0 {
		t.Fatalf("scenario = %+v", r)
	}
}
