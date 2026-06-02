// Package projectdef provides the specscore.yaml schema and read/write
// operations defined by the SpecScore Repo Config feature
// (https://specscore.md/repo-config).
package projectdef

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// yamlMarshal is the marshal function used by WriteSpecConfig.
// Tests can replace it to simulate marshal failures.
var yamlMarshal = yaml.Marshal

// SpecConfigFile is the canonical filename for the repo-level config,
// per the repo-config feature.
const SpecConfigFile = "specscore.yaml"

// SchemaHeader is the exact comment that MUST appear on line 1 of every
// specscore.yaml file. See repo-config#req:schema-header-comment.
const SchemaHeader = "# SpecScore Repo Config Schema: https://specscore.md/repo-config"

// Default values supplied when fields are omitted.
const (
	DefaultSpecsDirName = "specs"
	DefaultDocsDirName  = "docs"
	DefaultStudioName   = "SpecScore.Studio"
	DefaultStudioURL    = "https://specscore.studio/"
	DefaultModuleName   = "default"
)

// SpecConfig is the deserialized form of specscore.yaml.
type SpecConfig struct {
	Project      *ProjectConfig `yaml:"project,omitempty"`
	Projects     []string       `yaml:"projects,omitempty"`
	SpecsDirName string         `yaml:"specs_dir_name,omitempty"`
	DocsDirName  string         `yaml:"docs_dir_name,omitempty"`
	Studio       *StudioConfig  `yaml:"studio,omitempty"`
	Modules      []ModuleConfig `yaml:"modules,omitempty"`
	Grade        *GradeConfig   `yaml:"grade,omitempty"`
	Extras       map[string]any `yaml:",inline"`

	// studioExplicitNull is set to true when YAML contains
	// `studio: null` (or `~`, or an empty value) — the opt-out form
	// per repo-config#req:studio-null-opts-out. It is NOT serialized;
	// callers should reconstruct via WithStudioSuppressed when writing.
	studioExplicitNull bool
}

// ProjectConfig holds project identity. All fields are optional; when
// omitted, callers infer values from the working directory and git
// remote.
type ProjectConfig struct {
	Title        string             `yaml:"title,omitempty"`
	Host         string             `yaml:"host,omitempty"`
	Org          string             `yaml:"org,omitempty"`
	Repo         string             `yaml:"repo,omitempty"`
	Repositories []RepositoryConfig `yaml:"repositories,omitempty"`
	Extras       map[string]any     `yaml:",inline"`
}

// RepositoryConfig is one entry in `project.repositories`. Per
// repo-config#req:repositories-entry-shape every entry MUST be a YAML
// mapping (object) with a non-empty `roles` list — flat URL strings are
// rejected.
type RepositoryConfig struct {
	URL     string         `yaml:"url"`
	Title   string         `yaml:"title,omitempty"`
	Comment string         `yaml:"comment,omitempty"`
	Roles   []Role         `yaml:"roles"`
	Extras  map[string]any `yaml:",inline"`
}

// Role is a value from the closed enum defined in
// repo-config#req:repositories-roles-enum.
type Role string

// Canonical role values per repo-config#req:repositories-roles-enum.
// The enum is closed in v1; new values require a Feature revision.
const (
	RoleCode          Role = "code"
	RoleSpecification Role = "specification"
	RoleState         Role = "state"
	RoleDocs          Role = "docs"
	RoleRunner        Role = "runner"
)

// validRoles is the canonical set used for membership checks.
var validRoles = map[Role]struct{}{
	RoleCode:          {},
	RoleSpecification: {},
	RoleState:         {},
	RoleDocs:          {},
	RoleRunner:        {},
}

// IsValidRole reports whether r is a member of the canonical role enum.
func IsValidRole(r Role) bool {
	_, ok := validRoles[r]
	return ok
}

// UnmarshalYAML enforces repo-config#req:repositories-entry-shape — the
// entry MUST be a mapping (not a scalar / not a sequence). Flat URL
// strings are rejected with a hard error citing the violated REQ.
func (r *RepositoryConfig) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf(
			"project.repositories entry must be a mapping with `url` and `roles` fields; flat-string entries are not accepted (repo-config#req:repositories-entry-shape)",
		)
	}
	// Use a type alias to avoid recursing back into UnmarshalYAML and to
	// pick up the inline `Extras` plumbing that gopkg.in/yaml.v3 wires
	// only on the unaliased type.
	type rawRepo RepositoryConfig
	var raw rawRepo
	if err := node.Decode(&raw); err != nil {
		return fmt.Errorf("decoding project.repositories entry: %w", err)
	}
	*r = RepositoryConfig(raw)
	return nil
}

// StudioConfig names the upstream studio that renders SpecScore
// artifacts. Both fields are required when the block is present.
// Replaces the pre-2026-05-19 `viewer:` block (repo-config#req:studio-explicit-values).
type StudioConfig struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// DefaultGradeValues is the built-in grade value set applied when no
// `grade.values` is configured (canonical-grade-metadata-field#req:grade-values-default).
var DefaultGradeValues = []string{"A", "B", "C", "D", "F"}

// GradeConfig is the optional `grade:` block. It carries the repository's
// allowed `**Grade:**` value set (canonical-grade-metadata-field#req:grade-values-config).
//
// Shape problems (a scalar `values`, an empty list, a non-scalar or empty
// entry, or a non-mapping `grade`) are recorded in shapeErr rather than
// returned as an unmarshal error, so a malformed block surfaces as a single
// `grade-values-shape` lint violation instead of breaking every config read.
type GradeConfig struct {
	Values        []string
	valuesPresent bool
	shapeErr      string
}

// UnmarshalYAML parses the `grade:` mapping, validating the shape of
// `values` per canonical-grade-metadata-field#req:grade-values-shape.
// It never returns an error for shape problems; it records shapeErr.
func (g *GradeConfig) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		g.shapeErr = "`grade` must be a mapping with a `values` list (canonical-grade-metadata-field#req:grade-values-shape)"
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i]
		val := node.Content[i+1]
		if key.Value != "values" {
			continue
		}
		g.valuesPresent = true
		if val.Kind != yaml.SequenceNode {
			g.shapeErr = "`grade.values` must be a non-empty list of scalar tokens, not a scalar (canonical-grade-metadata-field#req:grade-values-shape)"
			return nil
		}
		if len(val.Content) == 0 {
			g.shapeErr = "`grade.values` must be a non-empty list (canonical-grade-metadata-field#req:grade-values-shape)"
			return nil
		}
		for _, item := range val.Content {
			if item.Kind != yaml.ScalarNode {
				g.shapeErr = "`grade.values` entries must be scalar tokens (canonical-grade-metadata-field#req:grade-values-shape)"
				return nil
			}
			tok := strings.TrimSpace(item.Value)
			if tok == "" {
				g.shapeErr = "`grade.values` entries must be non-empty (canonical-grade-metadata-field#req:grade-values-shape)"
				return nil
			}
			g.Values = append(g.Values, tok)
		}
	}
	return nil
}

// EffectiveGradeValues returns the repository's allowed grade value set:
// the configured `grade.values` (de-duplicated) when present and non-empty,
// otherwise DefaultGradeValues (canonical-grade-metadata-field#req:grade-values-default).
func (c SpecConfig) EffectiveGradeValues() []string {
	if c.Grade != nil && c.Grade.valuesPresent && len(c.Grade.Values) > 0 {
		return dedupeStrings(c.Grade.Values)
	}
	return append([]string(nil), DefaultGradeValues...)
}

// GradeShapeError returns the recorded `grade.values` shape error, or "" when
// the block is absent or well-formed.
func (c SpecConfig) GradeShapeError() string {
	if c.Grade != nil {
		return c.Grade.shapeErr
	}
	return ""
}

// GradeValuesHasDuplicates reports whether the configured `grade.values`
// list contains duplicate tokens (a lint advisory, not a hard error).
func (c SpecConfig) GradeValuesHasDuplicates() bool {
	if c.Grade == nil || !c.Grade.valuesPresent {
		return false
	}
	seen := make(map[string]bool, len(c.Grade.Values))
	for _, v := range c.Grade.Values {
		if seen[v] {
			return true
		}
		seen[v] = true
	}
	return false
}

// dedupeStrings returns s with duplicate values removed, preserving first-seen order.
func dedupeStrings(s []string) []string {
	seen := make(map[string]bool, len(s))
	out := make([]string, 0, len(s))
	for _, v := range s {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// ModuleConfig is one entry in the `modules:` list — a code area inside
// the repo, optionally with its own specs/docs subdirectories.
type ModuleConfig struct {
	Name   string         `yaml:"name,omitempty"`
	Path   string         `yaml:"path,omitempty"`
	Code   []string       `yaml:"code,omitempty"`
	Extras map[string]any `yaml:",inline"`
}

// EffectiveSpecsDirName returns the configured specs dir name or the
// default.
func (c SpecConfig) EffectiveSpecsDirName() string {
	if c.SpecsDirName != "" {
		return c.SpecsDirName
	}
	return DefaultSpecsDirName
}

// EffectiveDocsDirName returns the configured docs dir name or the
// default.
func (c SpecConfig) EffectiveDocsDirName() string {
	if c.DocsDirName != "" {
		return c.DocsDirName
	}
	return DefaultDocsDirName
}

// EffectiveStudio reports the studio to use for artifact links.
// suppressed is true when `studio: null` was set explicitly — callers
// MUST omit any toolbar in that case (repo-config#req:studio-null-opts-out).
// When the block is omitted entirely, name and url default to SpecScore.Studio
// (repo-config#req:studio-default-when-omitted).
func (c SpecConfig) EffectiveStudio() (name, url string, suppressed bool) {
	if c.studioExplicitNull {
		return "", "", true
	}
	if c.Studio != nil {
		return c.Studio.Name, c.Studio.URL, false
	}
	return DefaultStudioName, DefaultStudioURL, false
}

// EffectiveName returns the module's effective name per
// repo-config#req:module-name-deduction.
func (m ModuleConfig) EffectiveName() string {
	if m.Name != "" {
		return m.Name
	}
	if m.Path != "" {
		return filepath.Base(filepath.Clean(m.Path))
	}
	return DefaultModuleName
}

// EffectivePath returns the module's effective filesystem path,
// defaulting to "." (repo root) when no path is set.
func (m ModuleConfig) EffectivePath() string {
	if m.Path == "" {
		return "."
	}
	return filepath.Clean(m.Path)
}

// EffectiveModules returns the module list with the implicit-root
// default applied per repo-config#req:modules-default.
func (c SpecConfig) EffectiveModules() []ModuleConfig {
	if len(c.Modules) == 0 {
		return []ModuleConfig{{}}
	}
	return c.Modules
}

// Validate checks the structural invariants that don't require I/O:
// studio mapping completeness, repositories entry shape and role-enum
// membership, and module path uniqueness/non-nesting. File-system
// checks (e.g. projects local-path resolution) belong to the linter.
func (c SpecConfig) Validate() error {
	if !c.studioExplicitNull && c.Studio != nil {
		if c.Studio.Name == "" {
			return errors.New("studio.name is required when the studio block is a mapping (repo-config#req:studio-explicit-values)")
		}
		if c.Studio.URL == "" {
			return errors.New("studio.url is required when the studio block is a mapping (repo-config#req:studio-explicit-values)")
		}
		// repo-config#req:studio-url-trailing-slash — URL MUST end with
		// exactly one `/`. Reject empty trailing slash and double-slash.
		if !strings.HasSuffix(c.Studio.URL, "/") || strings.HasSuffix(c.Studio.URL, "//") {
			return errors.New("studio.url must end with exactly one '/' (repo-config#req:studio-url-trailing-slash)")
		}
	}
	if c.Project != nil {
		if err := validateRepositories(c.Project.Repositories); err != nil {
			return err
		}
	}
	return validateModules(c.Modules)
}

// validateRepositories enforces repo-config#req:repositories-entry-shape,
// repo-config#req:repositories-roles-list, and repo-config#req:repositories-roles-enum.
// Errors include the offending entry index and the violated REQ name.
func validateRepositories(repos []RepositoryConfig) error {
	for i, r := range repos {
		if r.URL == "" {
			return fmt.Errorf(
				"project.repositories[%d]: missing `url` (repo-config#req:repositories-entry-shape)",
				i,
			)
		}
		if len(r.Roles) == 0 {
			return fmt.Errorf(
				"project.repositories[%d]: `roles` is required and must be a non-empty list (repo-config#req:repositories-roles-list)",
				i,
			)
		}
		for _, role := range r.Roles {
			if !IsValidRole(role) {
				return fmt.Errorf(
					"project.repositories[%d]: unknown role %q; expected one of code, specification, state, docs, runner (repo-config#req:repositories-roles-enum)",
					i, string(role),
				)
			}
		}
	}
	return nil
}

// ValidateSchemaHeader verifies the first line of `data` is exactly the
// schema-header comment. Returns nil on success.
func ValidateSchemaHeader(data []byte) error {
	idx := bytes.IndexByte(data, '\n')
	var line []byte
	if idx < 0 {
		line = data
	} else {
		line = data[:idx]
	}
	line = bytes.TrimRight(line, "\r")
	if string(line) != SchemaHeader {
		return fmt.Errorf("schema header missing or malformed on line 1; expected %q (repo-config#req:schema-header-comment)", SchemaHeader)
	}
	return nil
}

// ReadSpecConfig reads dir/specscore.yaml and decodes it. The file must
// begin with the schema-header comment on line 1; otherwise an error
// is returned without further parsing.
//
// The pre-2026-05-19 `viewer:` block is rejected with a migration error
// (repo-config#req:viewer-block-rejected). It MUST be hand-renamed to
// `studio:` — there is no auto-migration in this pre-v1 break.
func ReadSpecConfig(dir string) (SpecConfig, error) {
	var cfg SpecConfig
	path := filepath.Join(dir, SpecConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("reading spec config: %w", err)
	}
	if err := ValidateSchemaHeader(data); err != nil {
		return cfg, err
	}
	if detectViewerKey(data) {
		return cfg, errors.New("viewer: block is no longer supported; rename to studio: in specscore.yaml (see https://specscore.md/repo-config-specification)")
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing spec config: %w", err)
	}
	// Detect `studio: null` vs studio-omitted. yaml.v3 unmarshals both
	// to a nil *StudioConfig, so we re-read at the node level to tell
	// them apart.
	cfg.studioExplicitNull = detectStudioExplicitNull(data)
	return cfg, nil
}

// WriteSpecConfig serializes cfg to dir/specscore.yaml, prepending the
// schema-header comment as line 1.
func WriteSpecConfig(dir string, cfg SpecConfig) error {
	body, err := yamlMarshal(cfg)
	if err != nil {
		return fmt.Errorf("marshalling spec config: %w", err)
	}
	out := append([]byte(SchemaHeader+"\n\n"), body...)
	if err := os.WriteFile(filepath.Join(dir, SpecConfigFile), out, 0o644); err != nil {
		return fmt.Errorf("writing spec config: %w", err)
	}
	return nil
}

// IsStudioSuppressed reports whether studio: null was set in the source.
func (c SpecConfig) IsStudioSuppressed() bool {
	return c.studioExplicitNull
}

// validateModules enforces module-paths-unique and module-paths-non-nested
// from the repo-config feature.
func validateModules(modules []ModuleConfig) error {
	if len(modules) == 0 {
		return nil
	}
	// 1. Effective-path uniqueness — implicit root counts as ".".
	seen := make(map[string]int, len(modules))
	for i, m := range modules {
		p := m.EffectivePath()
		if prev, ok := seen[p]; ok {
			return fmt.Errorf(
				"duplicate module path %q at modules[%d] and modules[%d] (repo-config#req:module-paths-unique)",
				p, prev, i,
			)
		}
		seen[p] = i
	}
	// 2. Non-nesting — applies only to explicit paths. The implicit-root
	// module (no path:) is exempt per repo-config#req:module-paths-non-nested.
	type idxPath struct {
		i int
		p string
	}
	var explicit []idxPath
	for i, m := range modules {
		if m.Path == "" {
			continue
		}
		explicit = append(explicit, idxPath{i, filepath.Clean(m.Path)})
	}
	for i, a := range explicit {
		for j, b := range explicit {
			if i == j {
				continue
			}
			if isAncestorPath(a.p, b.p) {
				return fmt.Errorf(
					"module path %q (modules[%d]) is an ancestor of %q (modules[%d]) (repo-config#req:module-paths-non-nested)",
					a.p, a.i, b.p, b.i,
				)
			}
		}
	}
	return nil
}

// isAncestorPath reports whether `a` is a strict ancestor directory of `b`
// when both are interpreted as forward-slash paths.
func isAncestorPath(a, b string) bool {
	a = filepath.ToSlash(a)
	b = filepath.ToSlash(b)
	if a == b {
		return false
	}
	return strings.HasPrefix(b, a+"/")
}

// detectStudioExplicitNull parses the YAML at the node level to detect
// whether `studio:` was explicitly null (vs omitted). Returns true only
// when the mapping has a "studio" key and its value is a Null node or
// an empty alias to Null.
func detectStudioExplicitNull(data []byte) bool {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return false
	}
	// Document → MappingNode → key/value pairs
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return false
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		key := root.Content[i]
		val := root.Content[i+1]
		if key.Value != "studio" {
			continue
		}
		// Scalar with empty value or YAML null tag = explicit null.
		if val.Kind == yaml.ScalarNode {
			if val.Tag == "!!null" || val.Value == "" || val.Value == "~" || val.Value == "null" || val.Value == "Null" || val.Value == "NULL" {
				return true
			}
		}
		return false
	}
	return false
}

// detectViewerKey returns true if the top-level YAML mapping contains
// a `viewer` key in any form (mapping, scalar, null, bare). Used by
// ReadSpecConfig to reject the legacy block per
// repo-config#req:viewer-block-rejected.
func detectViewerKey(data []byte) bool {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return false
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return false
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "viewer" {
			return true
		}
	}
	return false
}
