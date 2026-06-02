package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLintWithResult_FixReportsOnlyChangedFiles verifies that a fix pass over a
// tree with exactly one autofixable violation (a feature README missing its
// adherence footer) reports that one file's specRoot-relative path in Fixed and
// no other paths. A second pass over the now-clean tree reports an empty Fixed.
//
// Verifies: cli/spec/lint#ac:fix-reports-only-changed-files
func TestLintWithResult_FixReportsOnlyChangedFiles(t *testing.T) {
	tmp := t.TempDir()

	// One feature README missing its footer (autofixable).
	writeFeatureReadme(t, tmp, "needsfix", "# Feature: Needs Fix\n\n**Status:** Draft\n\n## Summary\nNo footer here.\n")
	// A second feature README that already has its footer (must NOT appear).
	writeFeatureReadme(t, tmp, "clean", "# Feature: Clean\n\n**Status:** Draft\n\n## Summary\nHas footer.\n\n---\n*This document follows the "+validFooterURL+"*\n")

	res, err := LintWithResult(Options{
		SpecRoot: tmp,
		Rules:    []string{"adherence-footer"},
		Fix:      true,
	})
	if err != nil {
		t.Fatalf("LintWithResult returned error: %v", err)
	}

	want := filepath.Join("features", "needsfix", "README.md")
	if len(res.Fixed) != 1 || res.Fixed[0] != want {
		t.Fatalf("Fixed = %v, want exactly [%s]", res.Fixed, want)
	}

	// The fix must actually have been applied on disk.
	b, err := os.ReadFile(filepath.Join(tmp, "features", "needsfix", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), validFooterURL) {
		t.Fatalf("expected footer to be appended, got:\n%s", string(b))
	}

	// Second pass on the now-clean tree: Fixed must be empty (idempotence).
	res2, err := LintWithResult(Options{
		SpecRoot: tmp,
		Rules:    []string{"adherence-footer"},
		Fix:      true,
	})
	if err != nil {
		t.Fatalf("LintWithResult (second pass) returned error: %v", err)
	}
	if len(res2.Fixed) != 0 {
		t.Fatalf("second pass Fixed = %v, want empty", res2.Fixed)
	}
}

// TestLintWithResult_NoFixEmptyFixed verifies Fixed is empty when Fix is false.
func TestLintWithResult_NoFixEmptyFixed(t *testing.T) {
	tmp := t.TempDir()
	writeFeatureReadme(t, tmp, "needsfix", "# Feature: Needs Fix\n\n**Status:** Draft\n\n## Summary\nNo footer.\n")

	res, err := LintWithResult(Options{
		SpecRoot: tmp,
		Rules:    []string{"adherence-footer"},
		Fix:      false,
	})
	if err != nil {
		t.Fatalf("LintWithResult returned error: %v", err)
	}
	if len(res.Fixed) != 0 {
		t.Fatalf("Fixed = %v, want empty when Fix is false", res.Fixed)
	}
}
