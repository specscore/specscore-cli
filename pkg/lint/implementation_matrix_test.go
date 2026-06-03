package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validMatrix = "# Feature: Dashboards\n\n**Status:** Approved\n\n" +
	"## Implementation Matrix\n\n" +
	"| Platform | Status | Brief | Link |\n" +
	"| --- | --- | --- | --- |\n" +
	"| CLI | Partial | Data-only, no graph | specscore:feature/dashboards-cli |\n" +
	"| Web | Full | Rich interactive graph | specscore:feature/dashboards-web |\n"

// Task 4 — a Capability whose matrix is missing the Status column is a
// shape error (capability-and-platform-implementations#ac:matrix-shape).
func TestImplementationMatrix_MissingStatusColumn_Errors(t *testing.T) {
	tmp := t.TempDir()
	content := "# Feature: Dashboards\n\n## Implementation Matrix\n\n" +
		"| Platform | Brief | Link |\n" +
		"| --- | --- | --- |\n" +
		"| CLI | Data-only | specscore:feature/dashboards-cli |\n"
	writeFeatureReadme(t, tmp, "dashboards", content)

	violations := runImplementationMatrixCheck(t, tmp)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	v := violations[0]
	if v.Rule != "implementation-matrix" {
		t.Errorf("Rule = %q, want implementation-matrix", v.Rule)
	}
	if !strings.Contains(v.Message, "missing required column") || !strings.Contains(v.Message, "Status") {
		t.Errorf("Message = %q, want missing-required-column naming Status", v.Message)
	}
}

// Task 4 — a Status cell outside the vocabulary is an error naming the
// allowed set (capability-and-platform-implementations#ac:matrix-bad-status).
func TestImplementationMatrix_BadStatus_Errors(t *testing.T) {
	tmp := t.TempDir()
	content := "# Feature: Dashboards\n\n## Implementation Matrix\n\n" +
		"| Platform | Status | Brief | Link |\n" +
		"| --- | --- | --- | --- |\n" +
		"| CLI | Done | Data-only | specscore:feature/dashboards-cli |\n"
	writeFeatureReadme(t, tmp, "dashboards", content)

	violations := runImplementationMatrixCheck(t, tmp)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	m := violations[0].Message
	if !strings.Contains(m, "Done") {
		t.Errorf("Message = %q, want it to name the offending value Done", m)
	}
	for _, want := range []string{"Full", "Partial", "Planned", "Absent"} {
		if !strings.Contains(m, want) {
			t.Errorf("Message = %q, want it to list allowed value %q", m, want)
		}
	}
}

// Task 4 — a Brief cell carrying multiple lines (via an HTML break) is an
// error (capability-and-platform-implementations#ac:matrix-brief-single-line).
func TestImplementationMatrix_BriefMultiLine_Errors(t *testing.T) {
	tmp := t.TempDir()
	content := "# Feature: Dashboards\n\n## Implementation Matrix\n\n" +
		"| Platform | Status | Brief | Link |\n" +
		"| --- | --- | --- | --- |\n" +
		"| CLI | Partial | Data-only<br>no graph rendering | specscore:feature/dashboards-cli |\n"
	writeFeatureReadme(t, tmp, "dashboards", content)

	violations := runImplementationMatrixCheck(t, tmp)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	if !strings.Contains(violations[0].Message, "single line") {
		t.Errorf("Message = %q, want single-line Brief error", violations[0].Message)
	}
}

// A well-formed matrix produces no violations.
func TestImplementationMatrix_Valid_NoViolation(t *testing.T) {
	tmp := t.TempDir()
	writeFeatureReadme(t, tmp, "dashboards", validMatrix)

	violations := runImplementationMatrixCheck(t, tmp)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %d: %+v", len(violations), violations)
	}
}

// A non-Capability Feature (no matrix heading) is ignored by the rule.
func TestImplementationMatrix_NonCapabilityIgnored(t *testing.T) {
	tmp := t.TempDir()
	writeFeatureReadme(t, tmp, "plain", "# Feature: Plain\n\n**Status:** Approved\n\n## Summary\n")

	violations := runImplementationMatrixCheck(t, tmp)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %d: %+v", len(violations), violations)
	}
}

// A Capability whose matrix heading has no table at all is reported as
// missing every required column.
func TestImplementationMatrix_NoTable_Errors(t *testing.T) {
	tmp := t.TempDir()
	writeFeatureReadme(t, tmp, "dashboards", "# Feature: Dashboards\n\n## Implementation Matrix\n\nNo table yet.\n")

	violations := runImplementationMatrixCheck(t, tmp)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	for _, want := range []string{"Platform", "Status", "Brief", "Link"} {
		if !strings.Contains(violations[0].Message, want) {
			t.Errorf("Message = %q, want it to list required column %q", violations[0].Message, want)
		}
	}
}

func TestImplementationMatrix_Metadata(t *testing.T) {
	c := newImplementationMatrixChecker()
	if c.name() != "implementation-matrix" {
		t.Errorf("name = %q", c.name())
	}
	if c.severity() != "error" {
		t.Errorf("severity = %q", c.severity())
	}
}

func TestImplementationMatrix_WalkError(t *testing.T) {
	tmp := t.TempDir()
	writeFeatureReadme(t, tmp, "dashboards", validMatrix)
	featDir := filepath.Join(tmp, "features", "dashboards")
	if err := os.Chmod(featDir, 0o111); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(featDir, 0o755) }()

	c := newImplementationMatrixChecker()
	if _, err := c.check(tmp); err == nil {
		t.Fatal("expected a walk error from the unreadable features subtree")
	}
}

// Unit coverage for the matrix-row extractor's structural edge cases.
func TestExtractMatrixRows(t *testing.T) {
	t.Run("no heading", func(t *testing.T) {
		if rows := extractMatrixRows("# Feature: X\n\n## Summary\n"); rows != nil {
			t.Errorf("expected nil rows, got %+v", rows)
		}
	})
	t.Run("table ends at next heading without blank line", func(t *testing.T) {
		content := "## Implementation Matrix\n| Platform | Status | Brief | Link |\n| --- | --- | --- | --- |\n## Next\n"
		rows := extractMatrixRows(content)
		if len(rows) != 2 {
			t.Fatalf("expected 2 rows (header + separator), got %d: %+v", len(rows), rows)
		}
	})
	t.Run("non-table content before any row stops extraction", func(t *testing.T) {
		if rows := extractMatrixRows("## Implementation Matrix\nprose line\n| Platform |\n"); rows != nil {
			t.Errorf("expected nil rows, got %+v", rows)
		}
	})
}

// A header-only matrix (all required columns, no separator/data rows) has no
// data rows to validate and so reports no violations.
func TestCheckImplementationMatrix_HeaderOnly_NoViolation(t *testing.T) {
	content := "## Implementation Matrix\n| Platform | Status | Brief | Link |\n"
	if msgs := checkImplementationMatrix(content); len(msgs) != 0 {
		t.Fatalf("expected no messages for a header-only matrix, got %+v", msgs)
	}
}

// Task 5 — the Implementation Matrix is author-declared: `lint --fix` validates
// its shape but never mutates Status cells, even when a same-repo
// Implementation's lifecycle differs from the declared parity
// (capability-and-platform-implementations#ac:matrix-no-rollup).
func TestImplementationMatrix_NoRollupUnderFix(t *testing.T) {
	// The matrix checker must not implement the fixer interface at all — that
	// structurally guarantees `--fix` can never rewrite a Status cell.
	if _, ok := newImplementationMatrixChecker().(fixer); ok {
		t.Fatal("implementation-matrix checker must not implement fixer (no rollup)")
	}

	tmp := t.TempDir()
	capability := "# Feature: Dashboards\n\n**Status:** Approved\n\n## Implementation Matrix\n\n" +
		"| Platform | Status | Brief | Link |\n" +
		"| --- | --- | --- | --- |\n" +
		"| CLI | Planned | Data-only | specscore:feature/dashboards-cli |\n"
	writeFeatureReadme(t, tmp, "dashboards", capability)
	// A same-repo Implementation whose lifecycle (Stable) differs from the
	// declared parity status (Planned) — a rollup would "correct" the cell.
	writeFeatureReadme(t, tmp, "dashboards-cli",
		"# Feature: Dashboards (CLI)\n\n**Status:** Stable\n**Implements:** specscore:feature/dashboards\n\n## Summary\n")

	capPath := filepath.Join(tmp, "features", "dashboards", "README.md")
	before, err := os.ReadFile(capPath)
	if err != nil {
		t.Fatal(err)
	}

	res, err := LintWithResult(Options{SpecRoot: tmp, Fix: true, Rules: []string{"implementation-matrix"}})
	if err != nil {
		t.Fatalf("LintWithResult: %v", err)
	}
	if len(res.Fixed) != 0 {
		t.Errorf("expected no files fixed, got %v", res.Fixed)
	}

	after, err := os.ReadFile(capPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("Capability matrix mutated by --fix:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if !strings.Contains(string(after), "| CLI | Planned |") {
		t.Errorf("declared Status cell Planned was altered:\n%s", after)
	}
}

func runImplementationMatrixCheck(t *testing.T, specRoot string) []Violation {
	t.Helper()
	c := newImplementationMatrixChecker()
	violations, err := c.check(specRoot)
	if err != nil {
		t.Fatalf("check returned error: %v", err)
	}
	return violations
}
