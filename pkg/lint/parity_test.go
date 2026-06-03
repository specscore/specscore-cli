package lint

import (
	"strings"
	"testing"
)

// TestCheckRegistryParity_RealCodebase asserts the live registry and the set
// of IDs the default linter can emit are in full parity.
func TestCheckRegistryParity_RealCodebase(t *testing.T) {
	if err := CheckRegistryParity(); err != nil {
		t.Fatalf("CheckRegistryParity() returned error, want nil parity: %v", err)
	}
}

// TestCheckRegistryParity_OrphanRegistryEntry proves the check fails and names
// the offending ID when a registry entry has no emitting checker.
func TestCheckRegistryParity_OrphanRegistryEntry(t *testing.T) {
	const orphan = "fake-orphan-rule-xyz"
	registerRule(Rule{
		ID:          orphan,
		Description: "synthetic orphan for parity test",
		Family:      "test",
		Severity:    "error",
	})
	defer delete(ruleRegistry, orphan)

	err := CheckRegistryParity()
	if err == nil {
		t.Fatal("CheckRegistryParity() returned nil, want error for orphan registry entry")
	}
	if !strings.Contains(err.Error(), orphan) {
		t.Fatalf("error %q does not name the orphan ID %q", err.Error(), orphan)
	}
}

// TestCheckParity_UnregisteredEmission proves the core helper fails and names
// an ID a checker can emit that is absent from the registry.
func TestCheckParity_UnregisteredEmission(t *testing.T) {
	const emitted = "fake-emitted-rule-xyz"
	registry := map[string]bool{"known-rule": true}
	emittable := map[string]bool{"known-rule": true, emitted: true}

	err := checkParity(registry, emittable)
	if err == nil {
		t.Fatal("checkParity() returned nil, want error for unregistered emission")
	}
	if !strings.Contains(err.Error(), emitted) {
		t.Fatalf("error %q does not name the unregistered emitted ID %q", err.Error(), emitted)
	}
}

// TestCheckParity_OrphanNamed proves the core helper names an orphan registry
// entry (registered but not emittable).
func TestCheckParity_OrphanNamed(t *testing.T) {
	const orphan = "fake-orphan-rule-xyz"
	registry := map[string]bool{"known-rule": true, orphan: true}
	emittable := map[string]bool{"known-rule": true}

	err := checkParity(registry, emittable)
	if err == nil {
		t.Fatal("checkParity() returned nil, want error for orphan entry")
	}
	if !strings.Contains(err.Error(), orphan) {
		t.Fatalf("error %q does not name the orphan ID %q", err.Error(), orphan)
	}
}

// TestCheckParity_FullParity asserts equal sets yield nil.
func TestCheckParity_FullParity(t *testing.T) {
	registry := map[string]bool{"a": true, "b": true}
	emittable := map[string]bool{"a": true, "b": true}
	if err := checkParity(registry, emittable); err != nil {
		t.Fatalf("checkParity() returned %v, want nil for equal sets", err)
	}
}
