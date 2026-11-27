package projectdef

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Tests for the opt-in `ac_test_coverage:` block
// (cli/spec/lint/ac-test-coverage#req:opt-in, #req:config-shape).

func readACTestCoverageConfig(t *testing.T, block string) SpecConfig {
	t.Helper()
	dir := t.TempDir()
	body := SchemaHeader + "\n\nproject:\n  title: T\n" + block
	if err := os.WriteFile(filepath.Join(dir, SpecConfigFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := ReadSpecConfig(dir)
	if err != nil {
		t.Fatalf("an ac_test_coverage block must never fail the whole config read: %v", err)
	}
	return cfg
}

// specscore:verifies feature/cli/spec/lint/ac-test-coverage#ac:off-without-opt-in
// specscore:verifies feature/cli/spec/lint/ac-test-coverage#ac:off-without-opt-in
func TestACTestCoverage_AbsentIsOffWithDefaults(t *testing.T) {
	var cfg SpecConfig
	if cfg.ACTestCoverageRequired() || cfg.ACTestCoverageExplicitlyOff() || cfg.ACTestCoverageShapeError() != "" {
		t.Fatal("absent block must be off, not explicitly off, and well-formed")
	}
	got := cfg.ACTestCoverageTestFiles()
	if !slices.Equal(got, DefaultACTestFileGlobs) {
		t.Fatalf("default globs = %v", got)
	}
	got[0] = "mutated"
	if DefaultACTestFileGlobs[0] == "mutated" {
		t.Fatal("ACTestCoverageTestFiles must return a copy of the defaults")
	}
}

func TestACTestCoverage_ParsedFromYAML(t *testing.T) {
	cfg := readACTestCoverageConfig(t, "ac_test_coverage:\n  required: true\n  test_files:\n    - \" e2e/**/*.feature \"\n    - \"**/*_test.go\"\n")
	if !cfg.ACTestCoverageRequired() {
		t.Fatalf("required: true must turn the rule on (shape: %q)", cfg.ACTestCoverageShapeError())
	}
	if got := cfg.ACTestCoverageTestFiles(); !slices.Equal(got, []string{"e2e/**/*.feature", "**/*_test.go"}) {
		t.Fatalf("test_files = %v", got)
	}
	if _, leaked := cfg.Extras["ac_test_coverage"]; leaked {
		t.Fatal("ac_test_coverage must parse into the typed field, not Extras")
	}
}

func TestACTestCoverage_RequiredFalseIsExplicitlyOff(t *testing.T) {
	cfg := readACTestCoverageConfig(t, "ac_test_coverage:\n  required: false\n")
	if cfg.ACTestCoverageRequired() || !cfg.ACTestCoverageExplicitlyOff() {
		t.Fatal("required: false must be off and explicitly off")
	}
}

// specscore:verifies feature/cli/spec/lint/ac-test-coverage#ac:malformed-config-visible
// specscore:verifies feature/cli/spec/lint/ac-test-coverage#ac:malformed-config-visible
func TestACTestCoverage_ShapeErrors(t *testing.T) {
	cases := map[string]struct{ block, want string }{
		"scalar":           {"ac_test_coverage: true\n", "must be a mapping"},
		"required string":  {"ac_test_coverage:\n  required: \"yes\"\n", "`ac_test_coverage.required` must be true or false"},
		"required list":    {"ac_test_coverage:\n  required: [true]\n", "`ac_test_coverage.required` must be true or false"},
		"test_files empty": {"ac_test_coverage:\n  required: true\n  test_files: []\n", "non-empty list of glob strings"},
		"test_files str":   {"ac_test_coverage:\n  required: true\n  test_files: \"**/*_test.go\"\n", "non-empty list of glob strings"},
		"entry number":     {"ac_test_coverage:\n  required: true\n  test_files: [1]\n", "non-empty list of glob strings"},
		"entry blank":      {"ac_test_coverage:\n  required: true\n  test_files: [\" \"]\n", "non-empty list of glob strings"},
		"bad glob":         {"ac_test_coverage:\n  required: true\n  test_files: [\"a/[b\"]\n", "entry `a/[b` is not a valid glob"},
		"unknown keys":     {"ac_test_coverage:\n  required: true\n  requird: true\n  note: x\n", "unknown key `ac_test_coverage.requird`"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := readACTestCoverageConfig(t, tc.block)
			if !strings.Contains(cfg.ACTestCoverageShapeError(), tc.want) {
				t.Fatalf("shape error %q does not contain %q", cfg.ACTestCoverageShapeError(), tc.want)
			}
			if cfg.ACTestCoverageRequired() {
				t.Fatal("a malformed block must not turn the rule on")
			}
		})
	}
	cfg := readACTestCoverageConfig(t, "ac_test_coverage:\n  required: false\n  note: x\n  other: y\n")
	if !cfg.ACTestCoverageExplicitlyOff() || !strings.Contains(cfg.ACTestCoverageShapeError(), "`ac_test_coverage.other`") {
		t.Fatalf("required: false with unknown keys: off=%v shape=%q", cfg.ACTestCoverageExplicitlyOff(), cfg.ACTestCoverageShapeError())
	}
	if cfg.ACTestCoverage.Extras["note"] != "x" || cfg.ACTestCoverage.Extras["other"] != "y" {
		t.Fatalf("unknown keys must be preserved, got %v", cfg.ACTestCoverage.Extras)
	}
}

func TestACTestCoverage_ProjectIdentity(t *testing.T) {
	if _, ok := (SpecConfig{}).ProjectIdentity(); ok {
		t.Fatal("no project block has no identity")
	}
	if _, ok := (SpecConfig{Project: &ProjectConfig{Host: "github.com", Org: "acme"}}).ProjectIdentity(); ok {
		t.Fatal("a partial identity is no identity")
	}
	got, ok := (SpecConfig{Project: &ProjectConfig{Host: "github.com", Org: "acme", Repo: "orders"}}).ProjectIdentity()
	if !ok || got != "github.com/acme/orders" {
		t.Fatalf("identity = %q, %v", got, ok)
	}
}
