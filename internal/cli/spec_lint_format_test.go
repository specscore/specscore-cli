package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/specscore/specscore-cli/pkg/projectdef"
	"gopkg.in/yaml.v3"
)

// --- Unit tests for output formatting functions ---

func TestOutputLintJSON_Empty(t *testing.T) {
	var buf bytes.Buffer
	err := outputLintJSON(&buf, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got []lint.Violation
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, buf.String())
	}
	if len(got) != 0 {
		t.Fatalf("expected empty array, got %d elements", len(got))
	}
}

func TestOutputLintJSON_WithViolations(t *testing.T) {
	violations := []lint.Violation{
		{File: "ideas/foo.md", Line: 5, Severity: "error", Rule: "idea-status", Message: "missing status"},
		{File: "features/README.md", Line: 1, Severity: "warning", Rule: "oq-section", Message: "missing OQ"},
	}
	var buf bytes.Buffer
	err := outputLintJSON(&buf, violations)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got []lint.Violation
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, buf.String())
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 violations, got %d", len(got))
	}
	if got[0].File != "ideas/foo.md" {
		t.Errorf("got[0].File = %q, want %q", got[0].File, "ideas/foo.md")
	}
	if got[0].Line != 5 {
		t.Errorf("got[0].Line = %d, want 5", got[0].Line)
	}
	if got[0].Severity != "error" {
		t.Errorf("got[0].Severity = %q, want %q", got[0].Severity, "error")
	}
	if got[0].Rule != "idea-status" {
		t.Errorf("got[0].Rule = %q, want %q", got[0].Rule, "idea-status")
	}
	if got[0].Message != "missing status" {
		t.Errorf("got[0].Message = %q, want %q", got[0].Message, "missing status")
	}
	if got[1].File != "features/README.md" {
		t.Errorf("got[1].File = %q, want %q", got[1].File, "features/README.md")
	}
}

func TestOutputLintYAML_Empty(t *testing.T) {
	var buf bytes.Buffer
	err := outputLintYAML(&buf, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if out != "[]" {
		t.Fatalf("expected %q, got %q", "[]", out)
	}
}

func TestOutputLintYAML_WithViolations(t *testing.T) {
	violations := []lint.Violation{
		{File: "ideas/foo.md", Line: 5, Severity: "error", Rule: "idea-status", Message: "missing status"},
		{File: "features/README.md", Line: 1, Severity: "warning", Rule: "oq-section", Message: "missing OQ"},
	}
	var buf bytes.Buffer
	err := outputLintYAML(&buf, violations)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify the output is valid YAML that decodes to the correct structure.
	var got []lint.Violation
	if err := yaml.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid YAML: %v\noutput: %s", err, buf.String())
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 violations, got %d", len(got))
	}
	if got[0].File != "ideas/foo.md" {
		t.Errorf("got[0].File = %q, want %q", got[0].File, "ideas/foo.md")
	}
	if got[1].Severity != "warning" {
		t.Errorf("got[1].Severity = %q, want %q", got[1].Severity, "warning")
	}
}

func TestOutputLintText_Empty(t *testing.T) {
	var buf bytes.Buffer
	err := outputLintText(&buf, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "0 violations found") {
		t.Errorf("expected '0 violations found' in output, got: %q", out)
	}
}

func TestOutputLintText_WithViolations(t *testing.T) {
	violations := []lint.Violation{
		{File: "ideas/foo.md", Line: 5, Severity: "error", Rule: "idea-status", Message: "missing status"},
		{File: "features/README.md", Line: 1, Severity: "warning", Rule: "oq-section", Message: "missing OQ"},
	}
	var buf bytes.Buffer
	err := outputLintText(&buf, violations)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	// Verify per-violation lines are present.
	if !strings.Contains(out, "ideas/foo.md:5 [error] idea-status: missing status") {
		t.Errorf("missing first violation line in output: %q", out)
	}
	if !strings.Contains(out, "features/README.md:1 [warning] oq-section: missing OQ") {
		t.Errorf("missing second violation line in output: %q", out)
	}
	// Verify summary line.
	if !strings.Contains(out, "2 violations found") {
		t.Errorf("expected '2 violations found' summary, got: %q", out)
	}
	if !strings.Contains(out, "1 error") {
		t.Errorf("expected '1 error' in summary, got: %q", out)
	}
	if !strings.Contains(out, "1 warning") {
		t.Errorf("expected '1 warning' in summary, got: %q", out)
	}
}

func TestOutputLintText_Plural(t *testing.T) {
	violations := []lint.Violation{
		{File: "a.md", Line: 1, Severity: "error", Rule: "r1", Message: "m1"},
		{File: "b.md", Line: 2, Severity: "error", Rule: "r2", Message: "m2"},
		{File: "c.md", Line: 3, Severity: "warning", Rule: "r3", Message: "m3"},
		{File: "d.md", Line: 4, Severity: "warning", Rule: "r4", Message: "m4"},
		{File: "e.md", Line: 5, Severity: "warning", Rule: "r5", Message: "m5"},
	}
	var buf bytes.Buffer
	err := outputLintText(&buf, violations)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	// 2 errors => "2 errors" (plural "s")
	if !strings.Contains(out, "2 errors") {
		t.Errorf("expected '2 errors' (plural), got: %q", out)
	}
	// 3 warnings => "3 warnings" (plural "s")
	if !strings.Contains(out, "3 warnings") {
		t.Errorf("expected '3 warnings' (plural), got: %q", out)
	}
}

func TestLintPlural(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "s"},
		{1, ""},
		{2, "s"},
		{5, "s"},
		{100, "s"},
	}
	for _, tt := range tests {
		got := lintPlural(tt.n)
		if got != tt.want {
			t.Errorf("lintPlural(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

// --- CLI integration tests for spec lint ---

func setupLintCleanProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := projectdef.WriteSpecConfig(root, projectdef.SpecConfig{}); err != nil {
		t.Fatalf("WriteSpecConfig: %v", err)
	}
	specDir := filepath.Join(root, "spec")
	if err := os.MkdirAll(filepath.Join(specDir, "features"), 0o755); err != nil {
		t.Fatalf("mkdir features: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(specDir, "ideas"), 0o755); err != nil {
		t.Fatalf("mkdir ideas: %v", err)
	}
	specReadme := "# Specifications\n\n## Contents\n\n- [features](features/README.md)\n- [ideas](ideas/README.md)\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(specDir, "README.md"), []byte(specReadme), 0o644); err != nil {
		t.Fatalf("write spec/README.md: %v", err)
	}
	featIdx := "# Features\n\n## Index\n\n| Feature | Status | Kind | Description |\n|---------|--------|------|-------------|\n\n_No features yet._\n\n## Open Questions\n\nNone at this time.\n\n---\n*This document follows the https://specscore.md/features-index-specification*\n"
	if err := os.WriteFile(filepath.Join(specDir, "features", "README.md"), []byte(featIdx), 0o644); err != nil {
		t.Fatalf("write features/README.md: %v", err)
	}
	ideasIdx := "# Ideas\n\n## Index\n\n| Idea | Status | Date | Owner | Promotes To |\n|------|--------|------|-------|-------------|\n\n_No active ideas yet._\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(specDir, "ideas", "README.md"), []byte(ideasIdx), 0o644); err != nil {
		t.Fatalf("write ideas/README.md: %v", err)
	}
	migrateTree(t, root)
	return root
}

func runSpec(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := specCommand()
	cmd.SilenceUsage = true
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestSpecLint_InvalidFormat(t *testing.T) {
	root := setupLintCleanProject(t)
	_, _, err := runSpec(t, "lint", "--project", root, "--format=csv")
	if err == nil {
		t.Fatal("expected error for invalid format, got nil")
	}
	if got := exitCodeOf(err); got != exitcode.InvalidArgs {
		t.Fatalf("exit code = %d, want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
	if !strings.Contains(err.Error(), "invalid format") {
		t.Errorf("error should mention 'invalid format', got: %q", err.Error())
	}
}

func TestSpecLint_InvalidSeverity(t *testing.T) {
	root := setupLintCleanProject(t)
	_, _, err := runSpec(t, "lint", "--project", root, "--severity=banana")
	if err == nil {
		t.Fatal("expected error for invalid severity, got nil")
	}
	if got := exitCodeOf(err); got != exitcode.InvalidArgs {
		t.Fatalf("exit code = %d, want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
	if !strings.Contains(err.Error(), "invalid severity") {
		t.Errorf("error should mention 'invalid severity', got: %q", err.Error())
	}
}

func TestSpecLint_MutualExclusion(t *testing.T) {
	root := setupLintCleanProject(t)
	_, _, err := runSpec(t, "lint", "--project", root, "--rules=oq-section", "--ignore=readme-exists")
	if err == nil {
		t.Fatal("expected error for mutual exclusion, got nil")
	}
	if got := exitCodeOf(err); got != exitcode.InvalidArgs {
		t.Fatalf("exit code = %d, want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error should mention 'mutually exclusive', got: %q", err.Error())
	}
}

func TestSpecLint_NoConfigRoot(t *testing.T) {
	root := t.TempDir() // no specscore.yaml
	_, _, err := runSpec(t, "lint", "--project", root)
	if err == nil {
		t.Fatal("expected error for missing config, got nil")
	}
	if got := exitCodeOf(err); got != exitcode.NotFound {
		t.Fatalf("exit code = %d, want %d (NotFound)", got, exitcode.NotFound)
	}
}

func TestSpecLint_FormatJSON(t *testing.T) {
	root := setupLintCleanProject(t)
	out, _, err := runSpec(t, "lint", "--project", root, "--format=json")
	// The clean project may still produce violations from some rules;
	// we only assert that the output is valid JSON.
	_ = err
	var parsed []lint.Violation
	if jsonErr := json.Unmarshal([]byte(out), &parsed); jsonErr != nil {
		t.Fatalf("output is not valid JSON: %v\nraw output: %s", jsonErr, out)
	}
}

func TestSpecLint_FormatYAML(t *testing.T) {
	root := setupLintCleanProject(t)
	out, _, err := runSpec(t, "lint", "--project", root, "--format=yaml")
	_ = err
	var parsed []lint.Violation
	if yamlErr := yaml.Unmarshal([]byte(out), &parsed); yamlErr != nil {
		t.Fatalf("output is not valid YAML: %v\nraw output: %s", yamlErr, out)
	}
}
