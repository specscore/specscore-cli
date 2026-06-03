package lint

import (
	"os"
	"path/filepath"
	"testing"
)

// registerRule must reject a duplicate ID (the construction-time guard panics).
func TestRegisterRule_DuplicatePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected registerRule to panic on a duplicate id")
		}
	}()
	// "readme-exists" is already registered as a built-in rule.
	registerRule(Rule{ID: "readme-exists", Description: "dup", Family: "core", Severity: "error"})
}

// A custom checker that reports an empty severity must fall back to "error" in
// its synthesized registry entry.
func TestRegisterChecker_EmptySeverityDefaultsToError(t *testing.T) {
	defer ResetCustomCheckers()
	RegisterChecker(&mockCustomChecker{n: "empty-sev-custom", s: ""})

	found := false
	for _, r := range AllRules() {
		if r.ID == "empty-sev-custom" {
			found = true
			if r.Severity != "error" {
				t.Errorf("empty-severity custom checker registered with severity %q, want error", r.Severity)
			}
		}
	}
	if !found {
		t.Fatal("expected empty-sev-custom in registry after RegisterChecker")
	}
}

// WriteCatalog must surface the directory-creation error when the catalog's
// parent path cannot be created (here, a regular file occupies the docs path).
func TestWriteCatalog_MkdirError(t *testing.T) {
	dir := t.TempDir()
	// Place a regular file where the "docs" directory would go so MkdirAll fails.
	if err := os.WriteFile(filepath.Join(dir, "docs"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seeding docs file: %v", err)
	}
	if err := WriteCatalog(dir); err == nil {
		t.Fatal("expected WriteCatalog to fail when the docs path is a file")
	}
}
