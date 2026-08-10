package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/projectdef"
)

func TestSpecLint_OpenQuestionsMigrationIsExplicitBoundedAndIdempotent(t *testing.T) {
	root := t.TempDir()
	if err := projectdef.WriteSpecConfig(root, projectdef.SpecConfig{}); err != nil {
		t.Fatal(err)
	}
	featureDir := filepath.Join(root, "spec", "features", "legacy-heading")
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(featureDir, "README.md")
	legacy := []byte("# Feature: Legacy heading\n\n## Summary\n\nKeep this prose about Outstanding Questions unchanged.\n\n## Outstanding Questions\n\n- Which migration owns this?\n")
	if err := os.WriteFile(legacyPath, legacy, 0o644); err != nil {
		t.Fatal(err)
	}
	unrelatedPath := filepath.Join(root, "spec", "notes.md")
	unrelated := []byte("# Notes\n\nThe phrase Outstanding Questions in prose is historical.\n")
	if err := os.WriteFile(unrelatedPath, unrelated, 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := runSpec(t, "lint", "--project", root, "--rules=oq-section")
	if err == nil {
		t.Fatal("read-only lint should diagnose the obsolete heading")
	}
	if got, readErr := os.ReadFile(legacyPath); readErr != nil || !bytes.Equal(got, legacy) {
		t.Fatalf("lint without --fix changed legacy bytes: err=%v", readErr)
	}

	_, _, err = runSpec(t, "lint", "--project", root, "--rules=oq-section", "--fix")
	if err != nil {
		t.Fatalf("explicit Open Questions migration: %v", err)
	}
	want := bytes.Replace(legacy, []byte("## Outstanding Questions"), []byte("## Open Questions"), 1)
	if got, readErr := os.ReadFile(legacyPath); readErr != nil || !bytes.Equal(got, want) {
		t.Fatalf("explicit fixer changed more than the heading: err=%v\n%s", readErr, got)
	}
	if got, readErr := os.ReadFile(unrelatedPath); readErr != nil || !bytes.Equal(got, unrelated) {
		t.Fatalf("explicit fixer changed unrelated prose: err=%v\n%s", readErr, got)
	}

	beforeSecond := append([]byte(nil), want...)
	_, stderr, err := runSpec(t, "lint", "--project", root, "--rules=oq-section", "--fix")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stderr, "Fixed") {
		t.Fatalf("second explicit fix was not a no-op: %s", stderr)
	}
	if got, _ := os.ReadFile(legacyPath); !bytes.Equal(got, beforeSecond) {
		t.Fatal("second explicit fix changed bytes")
	}
}
