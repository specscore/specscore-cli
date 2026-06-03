package lint

import "testing"

// Task 2 — parse the "**Implements:**" line by reusing the source-references
// specscore: grammar, including the cross-repo "@{host}/{org}/{repo}" suffix.
func TestParseImplementsRef(t *testing.T) {
	cases := []struct {
		name          string
		line          string
		wantSlug      string
		wantCrossRepo bool
		wantErr       bool
	}{
		{
			name:     "same-repo feature reference",
			line:     "**Implements:** specscore:feature/dashboards",
			wantSlug: "dashboards",
		},
		{
			name:          "cross-repo feature reference carries suffix",
			line:          "**Implements:** specscore:feature/dashboards@github.com/datatug/datatug",
			wantSlug:      "dashboards",
			wantCrossRepo: true,
		},
		{
			name:     "tolerates leading whitespace and trailing text",
			line:     "  **Implements:** specscore:feature/dashboards  (the capability)",
			wantSlug: "dashboards",
		},
		{
			name:    "no recognizable reference",
			line:    "**Implements:** dashboards",
			wantErr: true,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseImplementsRef(tc.line)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseImplementsRef(%q) = %+v, want error", tc.line, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseImplementsRef(%q) unexpected error: %v", tc.line, err)
			}
			if got.crossRepo != tc.wantCrossRepo {
				t.Errorf("crossRepo = %v, want %v", got.crossRepo, tc.wantCrossRepo)
			}
			if got.featureSlug() != tc.wantSlug {
				t.Errorf("featureSlug = %q, want %q", got.featureSlug(), tc.wantSlug)
			}
		})
	}
}

// Task 2 — a same-repo Implements reference that resolves to an existing
// Capability reports no violation (capability-and-platform-implementations#
// ac:implements-single-same-repo).
func TestImplementsReference_SameRepoResolvesToCapability_NoViolation(t *testing.T) {
	tmp := t.TempDir()
	writeFeatureReadme(t, tmp, "dashboards",
		"# Feature: Dashboards\n\n**Status:** Approved\n\n## Implementation Matrix\n\n| Platform | Status | Brief | Link |\n| --- | --- | --- | --- |\n")
	writeFeatureReadme(t, tmp, "dashboards-cli",
		"# Feature: Dashboards (CLI)\n\n**Status:** Approved\n**Implements:** specscore:feature/dashboards\n\n## Summary\n")

	violations := runImplementsReferenceCheck(t, tmp)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %d: %+v", len(violations), violations)
	}
}

// Task 2 — a well-formed cross-repo Implements reference is accepted without
// attempting to fetch the remote repo (capability-and-platform-implementations#
// ac:implements-cross-repo-suffix).
func TestImplementsReference_CrossRepoSuffix_NoViolation(t *testing.T) {
	tmp := t.TempDir()
	writeFeatureReadme(t, tmp, "dashboards-cli",
		"# Feature: Dashboards (CLI)\n\n**Status:** Approved\n**Implements:** specscore:feature/dashboards@github.com/datatug/datatug\n\n## Summary\n")

	violations := runImplementsReferenceCheck(t, tmp)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations for well-formed cross-repo reference, got %d: %+v", len(violations), violations)
	}
}

func runImplementsReferenceCheck(t *testing.T, specRoot string) []Violation {
	t.Helper()
	c := newImplementsReferenceChecker()
	violations, err := c.check(specRoot)
	if err != nil {
		t.Fatalf("check returned error: %v", err)
	}
	return violations
}
