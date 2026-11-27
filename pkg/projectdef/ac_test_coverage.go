package projectdef

import (
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"
)

// DefaultACTestFileGlobs are the test-file globs the feature-ac-test-coverage
// rule scans when `ac_test_coverage.test_files` is not configured
// (cli/spec/lint/ac-test-coverage#req:test-files). Globs are doublestar
// patterns matched against slash-separated paths relative to the project
// root.
var DefaultACTestFileGlobs = []string{
	"**/*_test.go",
	"**/*.test.{ts,tsx,js,mjs}",
	"**/*.spec.{ts,tsx,js,mjs}",
	"**/test_*.py",
	"**/*_test.py",
}

// ACTestCoverageConfig is the optional `ac_test_coverage:` block. It opts a
// repository into the feature-ac-test-coverage lint rule, which requires
// every acceptance criterion of an Implementing or Stable Feature to be named
// by a test-file annotation or explicitly deferred. Absent, or
// `required: false`, leaves the rule off. See
// spec/features/cli/spec/lint/ac-test-coverage/README.md#req:opt-in.
//
// Shape problems are recorded in shapeErr rather than returned as an
// unmarshal error (the StorageLayoutConfig pattern), so a malformed block
// never breaks every config read and surfaces as one visible lint finding
// (cli/spec/lint/ac-test-coverage#req:config-shape).
type ACTestCoverageConfig struct {
	// Required turns the rule on for this repository.
	Required bool `yaml:"required,omitempty"`
	// TestFiles, when set, replaces DefaultACTestFileGlobs.
	TestFiles []string       `yaml:"test_files,omitempty"`
	Extras    map[string]any `yaml:",inline"`

	shapeErr string
	// explicitlyOff is true when `required` is the YAML boolean false.
	explicitlyOff bool
}

// UnmarshalYAML parses the `ac_test_coverage:` block, recording shape
// problems in shapeErr instead of failing. Unknown keys are preserved in
// Extras and also reported, because a misspelled key would otherwise leave
// the rule off or the defaults in place.
func (a *ACTestCoverageConfig) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		a.shapeErr = "`ac_test_coverage` must be a mapping such as `required: true`"
		return nil
	}
	var problems []string
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, val := node.Content[i].Value, node.Content[i+1]
		switch key {
		case "required":
			var b bool
			if val.Kind != yaml.ScalarNode || val.Tag != "!!bool" || val.Decode(&b) != nil {
				problems = append(problems, "`ac_test_coverage.required` must be true or false")
				continue
			}
			a.Required, a.explicitlyOff = b, !b
		case "test_files":
			globs, problem := parseACTestFileGlobs(val)
			if problem != "" {
				problems = append(problems, problem)
				continue
			}
			a.TestFiles = globs
		default:
			var v any
			_ = val.Decode(&v)
			if a.Extras == nil {
				a.Extras = map[string]any{}
			}
			a.Extras[key] = v
			problems = append(problems, "unknown key `ac_test_coverage."+key+"` (known keys: required, test_files)")
		}
	}
	a.shapeErr = strings.Join(problems, "; ")
	return nil
}

// parseACTestFileGlobs reads `test_files`: a non-empty sequence of non-empty
// string scalars, each a valid doublestar pattern.
func parseACTestFileGlobs(val *yaml.Node) ([]string, string) {
	const shape = "`ac_test_coverage.test_files` must be a non-empty list of glob strings"
	if val.Kind != yaml.SequenceNode || len(val.Content) == 0 {
		return nil, shape
	}
	globs := make([]string, 0, len(val.Content))
	for _, item := range val.Content {
		g := strings.TrimSpace(item.Value)
		if item.Kind != yaml.ScalarNode || item.Tag != "!!str" || g == "" {
			return nil, shape
		}
		if !doublestar.ValidatePattern(g) {
			return nil, "`ac_test_coverage.test_files` entry `" + g + "` is not a valid glob"
		}
		globs = append(globs, g)
	}
	return globs, ""
}

// ACTestCoverageRequired reports whether the repository opted into the
// feature-ac-test-coverage lint rule (`ac_test_coverage.required: true`) with
// a well-formed block.
func (c SpecConfig) ACTestCoverageRequired() bool {
	return c.ACTestCoverage != nil && c.ACTestCoverage.Required && c.ACTestCoverage.shapeErr == ""
}

// ACTestCoverageTestFiles returns the configured `ac_test_coverage.test_files`
// globs, or a copy of DefaultACTestFileGlobs when none are configured.
func (c SpecConfig) ACTestCoverageTestFiles() []string {
	if c.ACTestCoverage == nil || len(c.ACTestCoverage.TestFiles) == 0 {
		return append([]string(nil), DefaultACTestFileGlobs...)
	}
	return c.ACTestCoverage.TestFiles
}

// ACTestCoverageExplicitlyOff reports whether the block sets `required: false`.
func (c SpecConfig) ACTestCoverageExplicitlyOff() bool {
	return c.ACTestCoverage != nil && c.ACTestCoverage.explicitlyOff
}

// ACTestCoverageShapeError returns the recorded `ac_test_coverage` shape
// problems, or "" when the block is absent or well-formed.
func (c SpecConfig) ACTestCoverageShapeError() string {
	if c.ACTestCoverage == nil {
		return ""
	}
	return c.ACTestCoverage.shapeErr
}

// ProjectIdentity returns the configured `host/org/repo` project identity and
// true, or "" and false when any of project.host, project.org or
// project.repo is missing.
func (c SpecConfig) ProjectIdentity() (string, bool) {
	p := c.Project
	if p == nil || p.Host == "" || p.Org == "" || p.Repo == "" {
		return "", false
	}
	return p.Host + "/" + p.Org + "/" + p.Repo, true
}
