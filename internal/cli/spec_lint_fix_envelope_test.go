package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/specscore/specscore-cli/pkg/projectdef"
	"gopkg.in/yaml.v3"
)

// setupAutofixProject builds a spec tree anchored by specscore.yaml with
// exactly one autofixable violation: a feature README missing its adherence
// footer. Returns the repo root.
func setupAutofixProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := projectdef.WriteSpecConfig(root, projectdef.SpecConfig{}); err != nil {
		t.Fatalf("WriteSpecConfig: %v", err)
	}
	dir := filepath.Join(root, "spec", "features", "needsfix")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := "# Feature: Needs Fix\n\n**Status:** Draft\n\n## Summary\nNo footer here.\n"
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	return root
}

// AC: fix-json-envelope-only-under-fix (the --fix half) and
// fix-report-needs-no-flag (json key) and fix-reports-only-changed-files.
// Under --fix --format json, stdout is a single object with both `fixed` and
// `violations` keys, and `fixed` names the changed file.
func TestSpecLint_FixJSONEnvelope(t *testing.T) {
	root := setupAutofixProject(t)
	out, _, _ := runSpec(t, "lint", "--project", root, "--rules=adherence-footer", "--fix", "--format=json")

	var env struct {
		Fixed      *[]string        `json:"fixed"`
		Violations *[]lint.Violation `json:"violations"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("output is not a valid JSON object: %v\nraw: %s", err, out)
	}
	if env.Fixed == nil {
		t.Fatalf("expected `fixed` key in envelope, got: %s", out)
	}
	if env.Violations == nil {
		t.Fatalf("expected `violations` key in envelope, got: %s", out)
	}
	want := filepath.Join("features", "needsfix", "README.md")
	found := false
	for _, f := range *env.Fixed {
		if f == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("`fixed` = %v, want it to contain %q", *env.Fixed, want)
	}
}

// AC: fix-json-envelope-only-under-fix (the no-fix half) — without --fix the
// json output is a bare array of violations with no `fixed` key.
func TestSpecLint_NoFixJSONBareArray(t *testing.T) {
	root := setupAutofixProject(t)
	out, _, _ := runSpec(t, "lint", "--project", root, "--rules=adherence-footer", "--format=json")

	// Must parse as a bare array.
	var arr []lint.Violation
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		t.Fatalf("no-fix output is not a bare JSON array: %v\nraw: %s", err, out)
	}
	// Must NOT parse as an object carrying a `fixed` key.
	var asObj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &asObj); err == nil {
		if _, ok := asObj["fixed"]; ok {
			t.Fatalf("no-fix output must not contain a `fixed` key, got: %s", out)
		}
	}
}

// AC: fix-json-envelope-only-under-fix — yaml variant of the envelope.
func TestSpecLint_FixYAMLEnvelope(t *testing.T) {
	root := setupAutofixProject(t)
	out, _, _ := runSpec(t, "lint", "--project", root, "--rules=adherence-footer", "--fix", "--format=yaml")

	var env struct {
		Fixed      *[]string         `yaml:"fixed"`
		Violations *[]lint.Violation `yaml:"violations"`
	}
	if err := yaml.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("output is not valid YAML: %v\nraw: %s", err, out)
	}
	if env.Fixed == nil {
		t.Fatalf("expected `fixed` key in YAML envelope, got: %s", out)
	}
	if env.Violations == nil {
		t.Fatalf("expected `violations` key in YAML envelope, got: %s", out)
	}
}
