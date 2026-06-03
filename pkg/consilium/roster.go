package consilium

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// rosterConfigFileName is the per-project SpecScore config filename. It is
// mirrored verbatim (rather than imported) so this package stays free of a
// dependency on pkg/event, matching the loader style there.
const rosterConfigFileName = "specscore.yaml"

// Group is one of the three fixed roster groups. The values double as the
// accepted enum for a custom role's `group:` key.
type Group string

const (
	GroupBuilders    Group = "builders"
	GroupCustomers   Group = "customers"
	GroupAdversaries Group = "adversaries"
)

// groupEnum is the closed set of accepted group values, in stable order so
// error messages list them deterministically.
var groupEnum = []Group{GroupBuilders, GroupCustomers, GroupAdversaries}

// RosterEntry is one resolved roster role: its slug name, its group, and (for
// custom roles only) the repo-root-relative path to its markdown file. Default
// roles carry an empty Path.
type RosterEntry struct {
	Name  string
	Group Group
	Path  string
}

// defaultRoster is the embedded 9-role default roster per
// REQ:default-roster-9-roles, in declared order: builders, then customers, then
// adversaries. This is the slug→group mapping the CLI owns; the role prompt
// files ship with the cross-repo consilium skill.
var defaultRoster = []RosterEntry{
	{Name: "engineer", Group: GroupBuilders},
	{Name: "architect", Group: GroupBuilders},
	{Name: "qa", Group: GroupBuilders},
	{Name: "pm", Group: GroupCustomers},
	{Name: "ux", Group: GroupCustomers},
	{Name: "marketing", Group: GroupCustomers},
	{Name: "yagni-cop", Group: GroupAdversaries},
	{Name: "skeptic", Group: GroupAdversaries},
	{Name: "security-ops", Group: GroupAdversaries},
}

// rosterConfig is the deserialization shape of the `consilium.roster` block.
// Both sub-keys are optional; their absence means "use defaults".
type rosterConfig struct {
	Exclude []string     `yaml:"exclude"`
	Custom  []customRole `yaml:"custom"`
}

// customRole is one `consilium.roster.custom` entry. All three fields are
// required; group must be one of the three-value enum. Field presence and the
// markdown-file contract are checked by a later validation task — here we only
// parse the block and validate the group enum so resolution can place the role.
type customRole struct {
	Name  string `yaml:"name"`
	Group string `yaml:"group"`
	Path  string `yaml:"path"`
}

// consiliumBlock / configRoot mirror only the slice of specscore.yaml this
// package consumes: consilium.roster.
type consiliumBlock struct {
	Roster rosterConfig `yaml:"roster"`
}

type configRoot struct {
	Consilium consiliumBlock `yaml:"consilium"`
}

// ResolveRoster loads <projectRoot>/specscore.yaml, parses the
// `consilium.roster` block, and resolves the active roster as
// (defaults − exclude) ∪ custom, preserving group membership. The result is an
// ordered list of entries: the surviving defaults in declared order, followed
// by custom roles in declared order. When the config file or the roster block
// is absent, the full 9-role default set is returned.
//
// Resolution intentionally does NOT perform roster validation (group floors,
// ≤12 cap, name collisions, custom-file parsing); that is a separate task that
// consumes this surface.
func ResolveRoster(projectRoot string) ([]RosterEntry, error) {
	cfg, err := loadRosterConfig(projectRoot)
	if err != nil {
		return nil, err
	}

	excluded := make(map[string]bool, len(cfg.Exclude))
	for _, slug := range cfg.Exclude {
		excluded[slug] = true
	}

	resolved := make([]RosterEntry, 0, len(defaultRoster)+len(cfg.Custom))
	for _, d := range defaultRoster {
		if excluded[d.Name] {
			continue
		}
		resolved = append(resolved, d)
	}

	for i, c := range cfg.Custom {
		if !validGroup(c.Group) {
			return nil, fmt.Errorf(
				"%s: consilium.roster.custom[%d].group: out-of-enum value %q; must be one of %s",
				rosterConfigFileName, i, c.Group, groupEnumList(),
			)
		}
		resolved = append(resolved, RosterEntry{
			Name:  c.Name,
			Group: Group(c.Group),
			Path:  c.Path,
		})
	}

	return resolved, nil
}

// loadRosterConfig reads specscore.yaml and returns the consilium.roster block.
// A missing config file yields a zero-value config (use defaults), matching the
// pkg/event loader's treatment of an absent file.
func loadRosterConfig(projectRoot string) (rosterConfig, error) {
	path := filepath.Join(projectRoot, rosterConfigFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return rosterConfig{}, nil
		}
		return rosterConfig{}, fmt.Errorf("read %s: %w", rosterConfigFileName, err)
	}

	var root configRoot
	if err := yaml.Unmarshal(data, &root); err != nil {
		return rosterConfig{}, fmt.Errorf("%s: parse: %w", rosterConfigFileName, err)
	}
	return root.Consilium.Roster, nil
}

// validGroup reports whether g is one of the three accepted group values.
func validGroup(g string) bool {
	for _, e := range groupEnum {
		if string(e) == g {
			return true
		}
	}
	return false
}

// groupEnumList renders the accepted group enum as a comma-separated string in
// declared order.
func groupEnumList() string {
	s := ""
	for i, g := range groupEnum {
		if i > 0 {
			s += ", "
		}
		s += string(g)
	}
	return s
}
