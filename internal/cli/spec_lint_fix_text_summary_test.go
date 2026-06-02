package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

// AC: fix-text-summary-on-stderr — under --fix in default text format, a
// "Fixed N file(s):" summary naming the modified path goes to stderr while
// stdout carries only the remaining-violations report (no fixed summary).
func TestSpecLint_FixTextSummaryOnStderr(t *testing.T) {
	root := setupAutofixProject(t)
	stdout, stderr, _ := runSpec(t, "lint", "--project", root, "--rules=adherence-footer", "--fix")

	want := filepath.Join("features", "needsfix", "README.md")

	if !strings.Contains(stderr, "Fixed") {
		t.Fatalf("stderr should contain a Fixed summary, got: %q", stderr)
	}
	if !strings.Contains(stderr, want) {
		t.Fatalf("stderr should name the modified path %q, got: %q", want, stderr)
	}
	if strings.Contains(stdout, "Fixed") {
		t.Fatalf("stdout must not contain the fixed summary, got: %q", stdout)
	}
}

// AC: fix-text-summary-on-stderr — a second --fix run on the now-clean tree
// modifies zero files, so no Fixed summary is written to stderr.
func TestSpecLint_FixTextSummaryIdempotentNoStderr(t *testing.T) {
	root := setupAutofixProject(t)

	// First run fixes the violation.
	_, stderr1, _ := runSpec(t, "lint", "--project", root, "--rules=adherence-footer", "--fix")
	if !strings.Contains(stderr1, "Fixed") {
		t.Fatalf("first run stderr should contain a Fixed summary, got: %q", stderr1)
	}

	// Second run on the now-clean tree: zero files modified.
	_, stderr2, _ := runSpec(t, "lint", "--project", root, "--rules=adherence-footer", "--fix")
	if strings.Contains(stderr2, "Fixed") {
		t.Fatalf("second run stderr must have no Fixed summary, got: %q", stderr2)
	}
}
