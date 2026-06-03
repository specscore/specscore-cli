package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
		{
			name:    "empty short notation is malformed",
			line:    "**Implements:** specscore:",
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

// Task 3 — a same-repo Implements reference that resolves to nothing is an
// error (capability-and-platform-implementations#ac:implements-unresolved-same-repo).
func TestImplementsReference_UnresolvedSameRepo_Errors(t *testing.T) {
	tmp := t.TempDir()
	writeFeatureReadme(t, tmp, "dashboards-cli",
		"# Feature: Dashboards (CLI)\n\n**Status:** Approved\n**Implements:** specscore:feature/no-such-capability\n\n## Summary\n")

	violations := runImplementsReferenceCheck(t, tmp)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	v := violations[0]
	if v.Rule != "implements-reference" {
		t.Errorf("Rule = %q, want implements-reference", v.Rule)
	}
	if !strings.Contains(v.Message, "does not resolve") {
		t.Errorf("Message = %q, want it to mention the reference does not resolve", v.Message)
	}
}

// Task 3 — a same-repo reference that resolves to an existing Feature lacking
// an "## Implementation Matrix" section is an error
// (capability-and-platform-implementations#ac:implements-non-capability).
func TestImplementsReference_TargetNotCapability_Errors(t *testing.T) {
	tmp := t.TempDir()
	writeFeatureReadme(t, tmp, "source-references",
		"# Feature: Source References\n\n**Status:** Approved\n\n## Summary\nNo Implementation Matrix here.\n")
	writeFeatureReadme(t, tmp, "dashboards-cli",
		"# Feature: Dashboards (CLI)\n\n**Status:** Approved\n**Implements:** specscore:feature/source-references\n\n## Summary\n")

	violations := runImplementsReferenceCheck(t, tmp)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	if !strings.Contains(violations[0].Message, "not a Capability") {
		t.Errorf("Message = %q, want it to mention the target is not a Capability", violations[0].Message)
	}
}

// A same-repo reference whose resolved path is not a feature at all (e.g. a
// plan reference) cannot point at a Capability — reported as unresolved.
func TestImplementsReference_NonFeatureTarget_Unresolved(t *testing.T) {
	tmp := t.TempDir()
	writeFeatureReadme(t, tmp, "dashboards-cli",
		"# Feature: Dashboards (CLI)\n\n**Status:** Approved\n**Implements:** specscore:plan/some-plan\n\n## Summary\n")

	violations := runImplementsReferenceCheck(t, tmp)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	if !strings.Contains(violations[0].Message, "does not resolve") {
		t.Errorf("Message = %q, want unresolved", violations[0].Message)
	}
}

// An Implementation whose "**Implements:**" line carries no recognizable
// specscore: reference is a malformed-notation error.
func TestImplementsReference_MalformedNotation_Errors(t *testing.T) {
	tmp := t.TempDir()
	writeFeatureReadme(t, tmp, "dashboards-cli",
		"# Feature: Dashboards (CLI)\n\n**Status:** Approved\n**Implements:** dashboards\n\n## Summary\n")

	violations := runImplementsReferenceCheck(t, tmp)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	if !strings.Contains(violations[0].Message, "well-formed") {
		t.Errorf("Message = %q, want malformed-notation message", violations[0].Message)
	}
}

// Metadata: the checker reports under the implements-reference rule at error severity.
func TestImplementsReference_Metadata(t *testing.T) {
	c := newImplementsReferenceChecker()
	if c.name() != "implements-reference" {
		t.Errorf("name = %q", c.name())
	}
	if c.severity() != "error" {
		t.Errorf("severity = %q", c.severity())
	}
}

// The walk error from an unreadable features subtree propagates out of check.
func TestImplementsReference_WalkError(t *testing.T) {
	tmp := t.TempDir()
	writeFeatureReadme(t, tmp, "dashboards-cli",
		"# Feature: Dashboards (CLI)\n\n**Implements:** specscore:feature/dashboards\n")
	featDir := filepath.Join(tmp, "features", "dashboards-cli")
	if err := os.Chmod(featDir, 0o111); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(featDir, 0o755) }()

	c := newImplementsReferenceChecker()
	if _, err := c.check(tmp); err == nil {
		t.Fatal("expected a walk error from the unreadable features subtree")
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
