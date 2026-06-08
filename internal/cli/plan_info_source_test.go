package cli

import (
	"strings"
	"testing"
)

// AC: `plan info --format text` prints a `Source:` line for a non-Feature
// source (idea-sourced or source-less); Feature-sourced plans omit it.
func TestPlanInfo_TextShowsNonFeatureSource(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	if _, _, err := runPlan(t, "new", "loose"); err != nil {
		t.Fatalf("plan new (source-less): %v", err)
	}
	stdout, _, err := runPlan(t, "info", "loose", "--format", "text")
	if err != nil {
		t.Fatalf("plan info: %v", err)
	}
	if !strings.Contains(stdout, "Source:") || !strings.Contains(stdout, "none") {
		t.Errorf("text output should carry a Source: none line, got:\n%s", stdout)
	}
}
